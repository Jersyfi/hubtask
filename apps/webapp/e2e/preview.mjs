// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The built web app over the walks' fixture, for a person to look at in their own browser:
// `pnpm --filter @hubtask/webapp build`, then `node apps/webapp/e2e/preview.mjs`.
//
// It answers "does this look right" without Go, PostgreSQL or a password: any email and password
// sign in, and every read is the fixture the engine walks use. It proves nothing about behaviour -
// a change that writes, syncs or authorises is walked against a real server
// (docs/evidence/README.md, "How to walk").
import { createReadStream, existsSync, statSync } from 'node:fs';
import { createServer } from 'node:http';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';
import { stub } from './fixture.mjs';

const dist = fileURLToPath(new URL('../dist/', import.meta.url));
const port = Number(process.env.PORT ?? 5180);
const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.png': 'image/png', '.ico': 'image/x-icon', '.webmanifest': 'application/manifest+json' };

if (!existsSync(join(dist, 'index.html'))) {
  console.error('apps/webapp/dist is missing - run `pnpm --filter @hubtask/webapp build` first');
  process.exit(1);
}

/** The fixture speaks Playwright's route; this is the least of one a plain request can be. */
function route(request, response, url, body) {
  const send = ({ status = 200, contentType, body: text, json }) => {
    response.writeHead(status, { 'content-type': json === undefined ? contentType : 'application/json' });
    response.end(json === undefined ? text : JSON.stringify(json));
  };
  return {
    request: () => ({ url: () => url.href, method: () => request.method, postDataJSON: () => (body ? JSON.parse(body) : null) }),
    fulfill: send,
  };
}

createServer((request, response) => {
  const url = new URL(request.url, `http://${request.headers.host}`);
  if (url.pathname.startsWith('/api/v1/')) {
    // The stream stays open, so the bar reads the connection as live rather than reconnecting.
    if (url.pathname === '/api/v1/stream') {
      response.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-store' });
      response.write('retry: 3600000\n\n');
      return;
    }
    let body = '';
    request.on('data', (chunk) => { body += chunk; });
    request.on('end', () => {
      if (url.pathname.startsWith('/api/v1/auth/sessions') && request.method === 'POST') {
        response.writeHead(201, { 'content-type': 'application/json' });
        response.end(JSON.stringify({ access_token: 'preview-bearer', refresh_token: 'preview-refresh', token_type: 'Bearer', expires_in: 3600 }));
        return;
      }
      stub(route(request, response, url, body));
    });
    return;
  }
  const file = normalize(join(dist, decodeURIComponent(url.pathname)));
  const target = file.startsWith(dist) && existsSync(file) && statSync(file).isFile() ? file : join(dist, 'index.html');
  response.writeHead(200, { 'content-type': types[extname(target)] ?? 'application/octet-stream' });
  createReadStream(target).pipe(response);
}).listen(port, '127.0.0.1', () => {
  console.log(`web app preview on http://localhost:${port} - any email and password sign in`);
});
