// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { API_PATH, LOCAL_SERVER, apiBaseOf, socketAddressOf } from './server.ts';

test('a shell\'s API is the chosen server\'s origin and the API\'s path', () => {
  const cases: Array<[string, string | undefined]> = [
    ['https://acme.hubtask.example', 'https://acme.hubtask.example/api/v1'],
    ['https://acme.hubtask.example/', 'https://acme.hubtask.example/api/v1'],
    ['https://acme.hubtask.example/some/page?x=1', 'https://acme.hubtask.example/api/v1'],
    ['https://tasks.example.org:8443', 'https://tasks.example.org:8443/api/v1'],
    ['  https://acme.hubtask.example  ', 'https://acme.hubtask.example/api/v1'],
    [LOCAL_SERVER, 'http://localhost:8080/api/v1'],
    ['http://127.0.0.1:8080', 'http://127.0.0.1:8080/api/v1'],
    // In the clear across a network, or not a server at all.
    ['http://acme.hubtask.example', undefined],
    ['https://person:secret@acme.hubtask.example', undefined],
    ['ftp://acme.hubtask.example', undefined],
    ['acme.hubtask.example', undefined],
    ['', undefined],
  ];
  for (const [server, want] of cases) assert.equal(apiBaseOf(server), want, server);
});

test('a live document\'s socket speaks to the API base, not to the page\'s own host', () => {
  const path = '/realtime/documents/page/0190d0c8-0000-7000-8000-000000000001';
  const cases: Array<[string, string, string]> = [
    // The browser: the API is the page's origin.
    [API_PATH, 'https://acme.hubtask.example/hubs/1', `wss://acme.hubtask.example/api/v1${path}`],
    [API_PATH, 'http://localhost:5173/', `ws://localhost:5173/api/v1${path}`],
    // An installed app: its page is the webview's, the API the server the person chose.
    ['https://acme.hubtask.example/api/v1', 'http://tauri.localhost/', `wss://acme.hubtask.example/api/v1${path}`],
    ['http://localhost:8080/api/v1', 'tauri://localhost/', `ws://localhost:8080/api/v1${path}`],
    // A server below a path prefix keeps it.
    ['https://example.org/tasks/api/v1', 'http://tauri.localhost/', `wss://example.org/tasks/api/v1${path}`],
  ];
  for (const [apiBase, page, want] of cases) assert.equal(socketAddressOf(apiBase, path, page), want, `${apiBase} on ${page}`);
});
