// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The built bundle, served the way the binary serves it: a file where one exists, the document
// everywhere else (ADR-0028's fallback). A few lines over node:http rather than a dependency,
// because what the browser job loads has to be what ships - `dist/` - and nothing more (ADR-0048,
// decision 3). It answers on a free port and hands the address back.

import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize } from 'node:path';

const TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.ico': 'image/x-icon',
  '.woff2': 'font/woff2',
  '.webmanifest': 'application/manifest+json',
};

/** Serves `root` on a free loopback port; resolves to the origin and the function that stops it. */
export function serve(root) {
  const server = createServer(async (request, response) => {
    const path = normalize(decodeURIComponent(new URL(request.url, 'http://localhost').pathname));
    const file = join(root, path);
    try {
      const body = await readFile(file.startsWith(root) && extname(file) ? file : join(root, 'index.html'));
      response.writeHead(200, { 'content-type': TYPES[extname(file)] ?? TYPES['.html'] });
      response.end(body);
    } catch {
      response.writeHead(404);
      response.end();
    }
  });
  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      resolve({ origin: `http://127.0.0.1:${port}`, close: () => new Promise((done) => server.close(done)) });
    });
  });
}
