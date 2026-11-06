// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { LOCAL_SERVER, apiBaseOf } from './server.ts';

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
