#!/usr/bin/env node
import { readFileSync } from 'fs';
import { createServer } from 'http';
import WebSocket, { WebSocketServer } from 'ws';
import PostCache from './post-cache.js';
import LangTracker from './lang-tracker.js';
import filterJetstreamMessage from './filter-jetstream-message.js';

const postCache = new PostCache(1500000);
const langTracker = new LangTracker();
let deletedPostHit = 0;
let deletedPostMiss = 0;

const indexHtmlContent = readFileSync('./index.html', 'utf-8');

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
  if (req.url !== '/' || req.method !== 'GET') {
    res.writeHead(404);
    return res.end('not found ' + req.method);
  }

  // remove in prod
  const indexHtmlContent = readFileSync('./index.html', 'utf-8');

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
  res.setHeader('cache-control', 'public, max-age=300');
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
  // console.log(found.value);
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

setInterval(() => {
  console.log(
    'cache size:', postCache.size(),
    'hit rate:', (deletedPostHit / (deletedPostHit + deletedPostMiss)).toFixed(3),
    'languages:', langTracker.getActive(),
    'connected clients:', wss.clients.size,
  );
  wss.clients.forEach(function each(client) {
    if (client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify({
        'type': 'observers',
        'observers': wss.clients.size,
      }));
    }
  });
}, 6000);

const PORT = 3000;
server.listen(PORT, '0.0.0.0', () => console.log(`listening on ${PORT}`));
