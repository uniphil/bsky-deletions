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

const jetstream = new WebSocket('wss://jetstream.atproto.tools/subscribe?wantedCollections=app.bsky.feed.post');

jetstream.onmessage = m => {
  const contents = JSON.parse(m.data);
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
};

const server = createServer(handleRequest);
const wss = new WebSocketServer({ noServer: true });

function handleRequest(req, res) {
  if (req.method !== 'GET') {
    res.writeHead(405);
    return res.end('method not allowed');
  }
  if (req.url === '/stats') {
    res.setHeader('content-type', 'application/json');
    res.setHeader('cache-control', 'public, max-age=300, immutable');
    res.writeHead(200);
    return res.end(JSON.stringify(statCache));
  }
  if (req.url !== '/') {
    res.writeHead(404);
    return res.end('not found');
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

  const userContent = indexHtmlContent
    .replaceAll('[[BROWSER_LANGS]]', JSON.stringify(preselectedLangs))
    .replaceAll('[[KNOWN_LANGS]]', JSON.stringify(knownLangs));

  res.setHeader('content-type', 'text/html');
  res.setHeader('cache-control', 'public, max-age=300, immutable');
  res.setHeader('vary', 'accept-language');
  res.writeHead(200);
  res.end(userContent);
};

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
  ws.langs = new URL(`https://host${req.url}`).searchParams.getAll('lang');
  ws.on('message', data => {
    let message;
    try {
      message = JSON.parse(data);
    } catch (e) {
      console.warn(`bad client message ${data}`);
      return;
    }
    if (message.type === 'setLangs') {
      ws.langs = message.langs;
    }
  });
});

function handleDeletedPost(found) {
  wss.clients.forEach(function each(client) {
    if (client.readyState === WebSocket.OPEN) {
      if ((client.langs?.length ?? 0) > 0) {
        if ((found.value.langs?.length ?? 0) === 0) return;
        if (!found.value.langs.some(l => client.langs.includes(l))) return;
      }
      client.send(JSON.stringify({
        'type': 'post',
        'post': found,
      }));
    }
  });
}


const STAT_CACHE_SIZE = 7200; // 12h at every 6s
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
}, 6000);

const PORT = 3000;
server.listen(PORT, '0.0.0.0', () => console.log(`listening on ${PORT}`));
