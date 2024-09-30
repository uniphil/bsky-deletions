#!/usr/bin/env node
import { readFileSync } from 'fs';
import { createServer } from 'http';
import WebSocket, { WebSocketServer } from 'ws';
import { Counter, Gauge, Histogram, register, collectDefaultMetrics, exponentialBuckets } from 'prom-client';
import PostCache from './post-cache.js';
import LangTracker from './lang-tracker.js';
import filterJetstreamMessage from './filter-jetstream-message.js';

const postCache = new PostCache(1000000);
const langTracker = new LangTracker();
let deletedPostHit = 0;
let deletedPostMiss = 0;
let replaying = true;
const REPLAY_MINUTES = (process.env.NODE_ENV === 'production') ? 30 : 5;
const REPLAY_COMPLETE_IF_WITHIN_S = 30; // consider replay complete if the last post is within this many seconds ofnow
let lastPostMs = +new Date() - REPLAY_MINUTES * 60 * 1000; // begin startup replay from

let preloadedIndexHtmlContent;
if (process.env.NODE_ENV === 'production') {
  console.log('preloading html template...');
  preloadedIndexHtmlContent = readFileSync('./index.html', 'utf-8');
}

collectDefaultMetrics();
const postCacheDepth = new Gauge({
  name: 'post_cache_depth',
  help: 'Seconds since the oldest item was created',
});
const postCacheSize = new Gauge({
  name: 'post_cache_size',
  help: 'Number of items in the post cache',
});
const postCounter = new Counter({
  name: 'posts',
  help: 'Count of new posts',
  labelNames: ['lang', 'target'],
});
const postDeleteCounter = new Counter({
  name: 'post_deletes',
  help: 'Count of deleted posts, lang and target only available for cach hits',
  labelNames: ['lang', 'target', 'cache'],
});
const postAge = new Histogram({
  name: 'post_deleted_age',
  help: 'Histogram of ages of deleted posts, cache misses excluded',
  labelNames: ['target'],
  buckets: exponentialBuckets(4, 4, 8),
});

let jws;
const jetstreamConnect = (n = 0) => {
  console.log('jetstream connecting...');
  const cursor = lastPostMs * 1000; // us timestamp
  jws = new WebSocket(`wss://jetstream.atproto.tools/subscribe?wantedCollections=app.bsky.feed.post&cursor=${cursor}`);
  jws.on('message', handleJetstreamMessage);
  jws.on('open', () => {
    n = 0;
    console.log('jetstream connected.');
  });
  jws.on('error', e => {
    console.error(e);
    jws.close();
  });
  jws.on('close', e => {
    const t = 1000 * Math.pow(1.3, n) * (1 + Math.random() / 10);
    console.warn(`jetstream connection closed, retrying in ${t}ms`, e);
    setTimeout(() => jetstreamConnect(n + 1), t);
  });
};
jetstreamConnect();

function handleJetstreamMessage(m) {
  let contents;
  try {
    contents = JSON.parse(m);
  } catch (e) {
    console.error('failed to parse json for jetstream message', e, m.data, m);
    return;
  }
  const message = filterJetstreamMessage(contents);
  if (message === undefined) return;
  const { type, t, rkey, langs, text, target } = message;
  if (type === 'post') {
    postCache.set(t, rkey, { text, langs, target });
    langs?.forEach(l => langTracker.addSighting(l));
    postCounter.inc({ lang: langs && langs[0], target });
  } else if (type === 'update') {
    postCache.update(t, rkey, { text, langs, target });
    langs?.forEach(l => langTracker.addSighting(l));
  } else if (type === 'delete') {
    const found = postCache.take(t, rkey);
    if (found !== undefined) {
      if (!replaying) {
        handleDeletedPost(found);
      }
      const { value: { langs, target }, age } = found;
      postAge.observe({ target }, age / 1000);
      postDeleteCounter.inc({ cache: 'hit', lang: langs && langs[0], target: target });
      deletedPostHit += 1;
    } else {
      postDeleteCounter.inc({ cache: 'miss' });
      deletedPostMiss += 1;
    }
  }
  if (t) {
    if (replaying && t > (+new Date() - REPLAY_COMPLETE_IF_WITHIN_S * 1000)) {
      replaying = false;
    }
    if (t > lastPostMs) {
      lastPostMs = t;
    }
  }
}

const server = createServer(handleRequest);
const wss = new WebSocketServer({ noServer: true });

function handleRequest(req, res) {
  req.on('error', console.error); // avoid throwing, possibly?
  res.on('error', console.error);
  if (req.method === 'GET' && req.url === '/ready') {
    if (replaying) {
      res.writeHead(503);
      return res.end('waiting for replay to catch up');
    } else if (jws.readyState !== WebSocket.OPEN) {
      res.writeHead(503);
      return res.end('jetstream disconnected');
    } else {
      res.writeHead(200);
      return res.end('ready');
    }
  }
  if (req.method === 'GET' && req.url === '/stats') {
    res.setHeader('content-type', 'application/json');
    res.setHeader('cache-control', 'public, max-age=300, immutable');
    res.writeHead(200);
    return res.end(JSON.stringify(getStats()));
  }
  if (req.method === 'GET' && req.url === '/metrics') {
    postCacheDepth.set(Math.round((+new Date - postCache.oldest()) / 1000));
    postCacheSize.set(postCache.size());
    return register.metrics().then(
      metrics => {
        res.setHeader('content-type', register.contentType);
        res.setHeader('cache-control', 'public, max-age=5'); // fly's grafana scrapes every 15
        res.end(metrics);
      },
      metricsErr => {
        res.writeHead(500);
        res.end('error collecting metrics');
      });
  }
  if (req.method === 'POST' && req.url === '/oops') {
    return handleClientErrorReport(req, res);
  }
  if (req.url !== '/') {
    res.writeHead(404);
    return res.end('not found');
  }
  if (req.method !== 'GET') {
    res.writeHead(405);
    return res.end('method not allowed');
  }

  // always reload in dev
  let indexHtmlContent = preloadedIndexHtmlContent ?? readFileSync('./index.html', 'utf-8');

  const knownLangs = langTracker.getActive();

  const preselectedLangs = Array.from(new Set(req.headers['accept-language']
    ?.split(',') // nottttt spec compliant but eh
      .map(l => l.trim())
      .map(l => l.split(';')[0])
      .map(l => l.split('-')[0])
      .filter(l => knownLangs.includes(l))
    ?? []));

  if (preselectedLangs.includes('en')) {
    preselectedLangs.push(null); // posts with no language tag have a high likelihood of being english
  }

  const userContent = indexHtmlContent
    .replaceAll('[[BROWSER_LANGS]]', JSON.stringify(preselectedLangs))
    .replaceAll('[[KNOWN_LANGS]]', JSON.stringify(knownLangs));

  res.setHeader('content-type', 'text/html');
  res.setHeader('cache-control', 'public, max-age=300, immutable');
  res.setHeader('vary', 'accept-language');
  res.writeHead(200);
  res.end(userContent);
};

function handleClientErrorReport(req, res) {
  const ua = req.headers['user-agent'];
  const body = [];
  req.on('data', chunk => { body.push(chunk); }).on('end', () => {
    try {
      const errInfo = JSON.parse(Buffer.concat(body).toString());
      console.warn('client error report', JSON.stringify({ ua, errInfo }));
    } catch (e) {
      console.warn('failed to receive client error report from', ua);
    };
    res.writeHead(201);
    return res.end('got it. and sorry :/');
  });
}

server.on('upgrade', function upgrade(req, socket, head) {
  wss.handleUpgrade(req, socket, head, function done(ws) {
    wss.emit('connection', ws, req);
  });
});

server.on('clientError', (err, socket) => {
  if (err.code === 'ECONNRESET' || !socket.writable) return;
  socket.end('HTTP/1.1 400 Bad Request\r\n\r\n');
});

wss.on('connection', (ws, req) => {
  ws.langs = new Set(new URL(`https://host${req.url}`).searchParams
    .getAll('lang')
    .map(l => l === 'null' ? null : l));
  console.log('new connection, langs:', ws.langs);
  ws.on('message', data => {
    let message;
    try {
      message = JSON.parse(data);
    } catch (e) {
      console.warn(`bad client message ${data}`);
      return;
    }
    if (message.type === 'setLangs') {
      ws.langs = new Set(message.langs);
      console.log('change langs:', ws.langs);
    }
  });
});

function handleDeletedPost(found) {
  const postLangs = found.value.langs ?? [null];
  wss.clients.forEach(function each(client) {
    if (client.readyState === WebSocket.OPEN) {
      if (
        (client.langs?.size ?? 0) > 0 &&
        !postLangs.some(lang => client.langs.has(lang))
      ) return;
      client.send(JSON.stringify({
        'type': 'post',
        'post': found,
      }));
    }
  });
}

const getStats = () => {
  const now_ms = +new Date();
  const now_s = Math.round(now_ms / 1000);
  const currentStats = {
    cached: postCache.size(),
    oldest: Math.round((now_ms - postCache.oldest()) / 1000),
    hit_rate: deletedPostHit / (deletedPostHit + deletedPostMiss),
    clients: wss.clients.size,
  };
  return currentStats;
}

setInterval(() => {
  wss.clients.forEach(function each(client) {
    if (client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify({
        'type': 'observers',
        'observers': wss.clients.size,
      }));
    }
  });
}, 12000);

console.log('getting jetstream replay...');
let lastReplayLog = +new Date();
(function waitForReplay() {
  if (replaying) {
    const now = +new Date;
    if ((now - lastReplayLog) > 2 * 1000) { // log replay progress every 2s
      lastReplayLog = now;
      const cacheSize = postCache.size();
      const minutesReplayed = Math.round((lastPostMs - postCache.oldest()) / 1000 / 60 * 10) / 10;
      console.log('replay', { cacheSize, minutesReplayed });
    }
    setTimeout(waitForReplay, 200);
  } else {
    console.log('replay complete, now forwarding deletions.');
  }
})();

const PORT = 3000;
server.listen(PORT, '0.0.0.0', () => console.log(`listening on ${PORT}`));
