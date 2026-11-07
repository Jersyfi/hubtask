// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A live document's words the server has not had yet are kept on the device under the document,
// and leaving the page while any wait asks first.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MemoryStorage } from '@hubtask/sync-engine';

import { UnsentDocuments, keeperIn } from './unsentdocs.ts';

test('a document keeps its unsent updates apart from every other document', async () => {
  const storage = new MemoryStorage();
  const page = keeperIn(storage, 'page', 'p1');
  const board = keeperIn(storage, 'whiteboard', 'p1');
  await page.save([new Uint8Array([1, 2])]);
  assert.deepEqual(await page.load(), [new Uint8Array([1, 2])]);
  assert.deepEqual(await board.load(), []);
  await page.save([]);
  assert.deepEqual(await page.load(), []);
  assert.deepEqual(await storage.collections(), [], 'nothing is left once sent');
});

test('leaving asks while any open document holds unsent changes', () => {
  const unsent = new UnsentDocuments();
  assert.equal(unsent.waiting, 0);
  unsent.set('page:p1', 3);
  unsent.set('whiteboard:b1', 1);
  assert.equal(unsent.waiting, 4);
  unsent.set('page:p1', 0);
  unsent.forget('whiteboard:b1');
  assert.equal(unsent.waiting, 0);
});

