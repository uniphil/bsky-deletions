#!/usr/bin/env node
import { readFileSync } from 'fs';
import { createServer } from 'http';
import WebSocket, { WebSocketServer } from 'ws';
import PostCache from './post-cache.js';
import LangTracker from './lang-tracker.js';
import filterJetstreamMessage from './filter-jetstream-message.js';

const postCache = new PostCache(1000000);
const langTracker = new LangTracker();
let deletedPostHit = 0;
let deletedPostMiss = 0;

let preloadedIndexHtmlContent;
if (process.env.NODE_ENV === 'production') {
  console.log('preloading html template...');
  preloadedIndexHtmlContent = readFileSync('./index.html', 'utf-8');
}

let jws;
const jetstreamConnect = (n = 0) => {
  console.log('jetstream connecting...');
  jws = new WebSocket('wss://jetstream.atproto.tools/subscribe?wantedCollections=app.bsky.feed.post');
  jws.onmessage = handleJetstreamMessage;
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
    contents = JSON.parse(m.data);
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
  } else if (type === 'update') {
    postCache.update(t, rkey, { text, langs, target });
    langs?.forEach(l => langTracker.addSighting(l));
  } else if (type === 'delete') {
    const found = postCache.take(t, rkey);
    if (found !== undefined) {
      handleDeletedPost(found);
      deletedPostHit += 1;
    } else {
      deletedPostMiss += 1;
    }
  }
}

const server = createServer(handleRequest);
const wss = new WebSocketServer({ noServer: true });

function handleRequest(req, res) {
  req.on('error', console.error); // avoid throwing, possibly?
  res.on('error', console.error);
  if (req.method === 'GET' && req.url === '/stats') {
    res.setHeader('content-type', 'application/json');
    res.setHeader('cache-control', 'public, max-age=300, immutable');
    res.writeHead(200);
    return res.end(JSON.stringify(statCache));
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


const STAT_CACHE_SIZE = 7200; // 24h at every 12s
let statCache = [];
const getStats = () => {
  const now_ms = +new Date();
  const now_s = Math.round(now_ms / 1000);
  const currentStats = {
    cached: postCache.size(),
    oldest: Math.round((now_ms - postCache.oldest()) / 1000),
    hit_rate: deletedPostHit / (deletedPostHit + deletedPostMiss),
    langs: langTracker._getStats(),
    clients: wss.clients.size,
  };
  if (statCache.length >= STAT_CACHE_SIZE) {
    statCache.unshift();
  }
  statCache.push({...currentStats, t: now_s});
  return currentStats;
}

setInterval(() => {
  const stats = {...getStats(), langs: langTracker.getActive()};
  wss.clients.forEach(function each(client) {
    if (client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify({
        'type': 'observers',
        'observers': stats.clients,
      }));
    }
  });
}, 12000);

const PORT = 3000;
server.listen(PORT, '0.0.0.0', () => console.log(`listening on ${PORT}`));
