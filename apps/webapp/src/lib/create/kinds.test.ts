// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { AT_ONCE, kindOfLetter, kindsHere, lastHere, noteLast } from './kinds.ts';

const all = ['KNOWLEDGE', 'WHITEBOARD', 'GOALS', 'CMDB', 'TIME'];

test('in a hub one writes and shapes, every kind whose module is on (C-1)', () => {
  assert.deepEqual(kindsHere({ mayCapture: true, hubId: 'h', modulesOn: all, mayWrite: true, mayShape: true }), ['entry', 'page', 'whiteboard', 'goal', 'dashboard', 'asset', 'time', 'collection']);
});

test('a kind one may not create here is absent, not grey (C-2, P-05)', () => {
  assert.deepEqual(kindsHere({ mayCapture: true, hubId: 'h', modulesOn: ['KNOWLEDGE'], mayWrite: false, mayShape: false }), ['entry']);
  assert.deepEqual(kindsHere({ mayCapture: false, modulesOn: all, mayWrite: true, mayShape: true }), []);
  assert.deepEqual(kindsHere({ mayCapture: true, modulesOn: ['WHITEBOARD'], mayWrite: true, mayShape: true }), ['entry']);
});

test('large kinds open at once, small ones in their dialog (C-3)', () => {
  assert.deepEqual(['page', 'whiteboard', 'dashboard', 'entry', 'goal', 'asset', 'time'].filter((kind) => AT_ONCE.has(kind as never)), ['page', 'whiteboard', 'dashboard']);
});

test('the last created kinds come first, only where they may be created (C-1)', () => {
  const last = noteLast(noteLast(noteLast([], 'page'), 'entry'), 'page');
  assert.deepEqual(last, ['page', 'entry']);
  assert.deepEqual(lastHere(['asset', 'page', 'entry'], ['entry', 'page']), ['page', 'entry']);
});

test('the letter beside a kind jumps to it, where it is offered (C-5)', () => {
  assert.equal(kindOfLetter('s', ['entry', 'page']), 'page');
  assert.equal(kindOfLetter('w', ['entry', 'page']), undefined);
});
