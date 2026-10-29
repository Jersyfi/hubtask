// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { boardFileName, cardEntries, clockOf, entryOfNote, liveBoardPath, notesInFrame, peerColour, pictureColours, readNative, voterKey } from './whiteboards.ts';

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

test('a frame made entries turns the sticky notes with words inside it (UC-CNV-02 check 7)', () => {
  const frame = { id: 'f', kind: 'FRAME' as const, x: 0, y: 0, width: 500, height: 500 };
  const note = (id: string, x: number, text: string) => ({ id, kind: 'STICKY' as const, x, y: 10, width: 100, height: 100, text });
  const shape = { id: 's', kind: 'SHAPE' as const, x: 10, y: 10, width: 50, height: 50, text: 'not a note' };
  assert.deepEqual(notesInFrame([frame, note('a', 10, 'Book'), note('b', 450, 'Half out'), note('c', 200, '  '), shape], frame).map((e) => e.id), ['a']);
});

test('an anonymous vote is kept under a minted key, an open one under the account (UC-CNV-05 check 1)', () => {
  assert.equal(voterKey(false, 'anna', () => 'k'), 'anna');
  assert.equal(voterKey(true, 'anna', () => 'k'), 'k');
  assert.equal(voterKey(true, 'anna', () => 'k', 'kept'), 'kept');
});

test('a timer reads as minutes and seconds, and a cursor colour is the same on every device', () => {
  assert.equal(clockOf(5 * 60 * 1000), '5:00');
  assert.equal(clockOf(61_001), '1:02');
  assert.equal(clockOf(-1), '0:00');
  assert.equal(peerColour(12), peerColour(12));
});

test('a sticky note becomes an entry of the type chosen, its words the title (UC-CNV-02 check 6)', () => {
  assert.deepEqual(entryOfNote('c1', '  Book the room ', { key: 'bug', system: false }), { type: 'TASK', collection_id: 'c1', title: 'Book the room', entry_type: 'bug' });
  assert.deepEqual(entryOfNote('c1', 'Book', { key: 'task', system: true }), { type: 'TASK', collection_id: 'c1', title: 'Book' });
  assert.equal(entryOfNote('c1', '   '), undefined, 'a note without words is not turned');
});
