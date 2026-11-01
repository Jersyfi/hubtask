// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A screen's chunk that cannot be loaded after an upgrade reloads the page once, onto the new
// release, instead of sending the person to check a connection that is fine.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { entryOf, whenAChunkFails } from './chunkfailure.ts';

const served = `<!DOCTYPE html><html><head>
<script type="module" crossorigin src="/assets/index-B2new.js"></script>
<link rel="stylesheet" crossorigin href="/assets/index-Cx.css"></head></html>`;

test('the entry is read from the served document', () => {
  assert.equal(entryOf(served), '/assets/index-B2new.js');
  assert.equal(entryOf('<html></html>'), null);
});

test('an upgraded release reloads the page once', () => {
  const base = { online: true, current: '/assets/index-A1old.js', served: '/assets/index-B2new.js' };
  assert.equal(whenAChunkFails({ ...base, reloadedFor: null }), 'reload');
  // The reload did not help (the new release is still missing the chunk): no loop.
  assert.equal(whenAChunkFails({ ...base, reloadedFor: '/assets/index-B2new.js' }), 'updated');
});

test('an unchanged release offers a retry, offline says so', () => {
  const same = { current: '/assets/index-B2new.js', served: '/assets/index-B2new.js', reloadedFor: null };
  assert.equal(whenAChunkFails({ ...same, online: true }), 'retry');
  assert.equal(whenAChunkFails({ ...same, online: false }), 'offline');
  // The document could not be read either: a retry, never a reload in the dark.
  assert.equal(whenAChunkFails({ ...same, online: true, served: null }), 'retry');
});
