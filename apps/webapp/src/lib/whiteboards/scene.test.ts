// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { sceneKey } from './scene.ts';

const shape = { id: 'a', kind: 'SHAPE', height: 70, locked: false, shape: 'ELLIPSE', text: 'Start', width: 160, x: 0, y: 0, color_token: null };
const line = { id: 'e', kind: 'CONNECTOR', from: 'a', to: 'b', text: '', x: 0, y: 0, width: 0, height: 0 };
const saved = [shape, line];

test('the scene the server holds and the same scene drawn again have one key, whatever the order of fields and the defaults', () => {
  const drawn = [
    { id: 'a', kind: 'SHAPE', x: 0, y: 0, width: 160, height: 70, text: 'Start', shape: 'ELLIPSE' },
    { id: 'e', kind: 'CONNECTOR', x: 0, y: 0, width: 0, height: 0, from: 'a', to: 'b' },
  ];
  assert.equal(sceneKey(drawn), sceneKey(saved));
});

test('a change of any kind changes the key: a moved, recoloured, locked, reordered or removed element', () => {
  const base = sceneKey(saved);
  for (const changed of [
    [{ ...shape, x: 10 }, line],
    [{ ...shape, color_token: 'red' }, line],
    [{ ...shape, locked: true }, line],
    [line, shape],
    [shape],
  ]) assert.notEqual(sceneKey(changed), base, JSON.stringify(changed));
});
