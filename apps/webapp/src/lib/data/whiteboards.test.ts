// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { boardFileName, cardEntries, liveBoardPath, pictureColours, readNative } from './whiteboards.ts';

test('a board lives at its own live document', () => {
  assert.equal(liveBoardPath('b1'), '/realtime/documents/whiteboard/b1');
});

test('an exported picture is drawn in the design system values, a label by its name (rule 15)', () => {
  const colours = pictureColours();
  assert.ok(colours.label('amber'));
  assert.equal(colours.label('crimson'), undefined);
  assert.notEqual(colours.paper, colours.ink);
});

test('an export is saved under the board title, and a native document is read back (UC-CNV-03 check 1)', () => {
  assert.equal(boardFileName('Retro: Q4 / Team', 'svg'), 'Retro-Q4-Team.svg');
  assert.equal(boardFileName('***', 'json'), 'whiteboard.json');
  const doc = JSON.stringify({ format: 'hubtask.whiteboard', version: 1, title: 'Retro', elements: [{ id: 'a', kind: 'STICKY', x: 0, y: 0, width: 1, height: 1 }] });
  assert.equal(readNative(doc)?.elements.length, 1);
  assert.equal(readNative('{"format":"other"}'), undefined);
  assert.equal(readNative('not json'), undefined);
});

test('the entries a board draws are asked for once each', () => {
  const card = (id: string, entry: string) => ({ id, kind: 'CARD' as const, x: 0, y: 0, width: 1, height: 1, target: { kind: 'ENTRY' as const, id: entry } });
  assert.deepEqual(cardEntries([card('a', 'e1'), card('b', 'e1'), card('c', 'e2')]), ['e1', 'e2']);
});
