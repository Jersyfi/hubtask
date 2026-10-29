// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import * as Y from 'yjs';

import {
  Board, Facilitation, MAX_STROKE_POINTS, TEMPLATES, TEMPLATE_WORDS, align, copy, fromMinimap, hit, minimap, paste, pdfOfJpeg, readCopied,
  strokePath, strokesOf, template, toSvg, touches, withGroups, type Element,
} from '../src/index.ts';

const sticky = (id: string, x = 0, y = 0, group?: string): Element => ({ id, kind: 'STICKY', x, y, width: 100, height: 100, text: id, ...(group ? { group } : {}) });
let n = 0;
const newId = () => `n${(n += 1)}`;

test('a drawn line becomes strokes in their own boxes, and the eraser touches them (UC-CNV-04 check 1)', () => {
  const line = Array.from({ length: 2500 }, (_, i) => ({ x: 10 + i, y: 20 + (i % 2) * 5 }));
  const strokes = strokesOf(line, 'PEN', newId, 'blue');
  assert.ok(strokes.length >= 2, 'a long line is cut into several strokes');
  for (const s of strokes) {
    assert.equal(s.kind, 'STROKE');
    assert.equal(s.color_token, 'blue');
    const points = s.points ?? [];
    assert.ok(points.length / 2 <= MAX_STROKE_POINTS);
    for (let i = 0; i < points.length; i += 2) {
      assert.ok((points[i] as number) >= 0 && (points[i] as number) <= s.width && (points[i + 1] as number) >= 0 && (points[i + 1] as number) <= s.height);
    }
  }
  const straight = strokesOf([{ x: 0, y: 0 }, { x: 50, y: 0.2 }, { x: 100, y: 0 }], 'HIGHLIGHTER', newId)[0] as Element;
  assert.equal((straight.points ?? []).length, 4, 'a nearly straight line keeps its ends');
  assert.match(strokePath(straight), /^M 0 0/);
  assert.ok(touches(straight, { x: 50, y: 6 }, 2), 'the highlighter is wide');
  assert.ok(!touches(straight, { x: 50, y: 40 }, 2));
  assert.equal(hit([straight], { x: 50, y: 40 }), undefined, 'a stroke is hit on its line, not its box');
});

test('a moving object snaps to the edges and centres near it, and the guide is drawn (UC-CNV-04 check 2)', () => {
  const others = [sticky('a', 0, 0)];
  const aligned = align({ left: 104, top: 3, right: 204, bottom: 103 }, others, 6);
  assert.equal(aligned.dx, -4, 'its left snaps to the other right edge');
  assert.equal(aligned.dy, -3, 'its top snaps to the other top edge');
  assert.deepEqual(aligned.guides.map((g) => g.axis), ['x', 'y']);
  assert.equal(align({ left: 300, top: 300, right: 400, bottom: 400 }, others, 6).guides.length, 0, 'nothing near, nothing drawn');
});

test('the minimap shows the whole board with the viewport, and takes the viewport where it is clicked (UC-CNV-04 check 3)', () => {
  const view = { x: 0, y: 0, zoom: 1, width: 400, height: 300 };
  const map = minimap([sticky('a', 0, 0), sticky('b', 3900, 2900)], view, { width: 200, height: 150 });
  assert.equal(map.boxes.length, 2);
  assert.ok(map.view.width <= 200 && map.view.left === 0);
  const moved = fromMinimap(map, view, { x: 200, y: 150 });
  assert.ok(moved.x > 3000 && moved.y > 2000, 'the far corner is reached');
});

test('a selection with its group and connectors is copied and pasted with new identifiers (UC-CNV-04 check 4)', () => {
  const elements: Element[] = [
    sticky('a', 0, 0, 'g'), sticky('b', 200, 0, 'g'), sticky('c', 400, 0),
    { id: 'l', kind: 'CONNECTOR', from: 'a', to: 'b', x: 0, y: 0, width: 0, height: 0 },
    { id: 'm', kind: 'CONNECTOR', from: 'b', to: 'c', x: 0, y: 0, width: 0, height: 0 },
  ];
  assert.deepEqual(withGroups(elements, ['a']), ['a', 'b'], 'choosing one of a group chooses the group');
  const copied = copy(elements, ['a']);
  assert.deepEqual(copied.elements.map((e) => e.id), ['a', 'b', 'l']);
  const again = readCopied(JSON.stringify(copied));
  assert.ok(again, 'the clipboard words read back');
  assert.equal(readCopied('{"format":"other"}'), undefined);
  const pasted = paste(again, { x: 1000, y: 1000 }, newId);
  const ids = new Set(pasted.map((e) => e.id));
  assert.equal(pasted.length, 3);
  assert.ok(!ids.has('a') && !ids.has('b'));
  const link = pasted.find((e) => e.kind === 'CONNECTOR') as Element;
  assert.ok(ids.has(link.from ?? '') && ids.has(link.to ?? ''), 'the connector is attached to the copies');
  const groups = new Set(pasted.filter((e) => e.kind === 'STICKY').map((e) => e.group));
  assert.equal(groups.size, 1);
  assert.ok(!groups.has('g'), 'the copy is a group of its own');
  const board = new Board(new Y.Doc());
  assert.ok(board.addAll(pasted));
  board.group([pasted[0]?.id ?? ''], null);
  assert.equal(board.get(pasted[0]?.id ?? '')?.group, undefined);
});

test('every template puts its objects on the board in the reader\'s words (UC-CNV-04 check 5)', () => {
  for (const name of TEMPLATES) {
    const elements = template(name, { at: { x: 0, y: 0 }, words: (key) => `«${key}»`, newId, entries: ['0190a000-0000-7000-8000-000000000001'] });
    assert.ok(elements.length > 0, name);
    const board = new Board(new Y.Doc());
    assert.ok(board.addAll(elements), `${name} fits on a board`);
    const texts = elements.map((e) => e.text ?? '').join(' ');
    for (const key of TEMPLATE_WORDS[name]) {
      assert.ok(texts.includes(`«${key}»`), `${name} names ${key}`);
    }
    const ids = new Set(elements.map((e) => e.id));
    for (const e of elements.filter((each) => each.kind === 'CONNECTOR')) assert.ok(ids.has(e.from ?? '') && ids.has(e.to ?? ''));
  }
  const kanban = template('KANBAN', { at: { x: 0, y: 0 }, words: (k) => k, newId, entries: ['0190a000-0000-7000-8000-000000000001'] });
  assert.equal(kanban.filter((e) => e.kind === 'CARD').length, 1, 'a Kanban board holds live cards');
});

test('a voting session counts each person\'s votes up to their number and hides who voted when anonymous (UC-CNV-05 check 1)', () => {
  const doc = new Y.Doc();
  const other = new Y.Doc();
  doc.on('update', (u: Uint8Array) => Y.applyUpdate(other, u));
  const session = new Facilitation(doc);
  const seen = new Facilitation(other);
  session.startVoting({ id: 'v1', perPerson: 2, anonymous: true, startedBy: 'anna' });
  assert.ok(session.vote('k1', 'a'));
  assert.ok(session.vote('k1', 'a'));
  assert.ok(!session.vote('k1', 'b'), 'no third vote');
  assert.ok(session.vote('k2', 'b'));
  assert.ok(session.unvote('k1', 'a'));
  const { counts, voters } = seen.results();
  assert.equal(voters, 2);
  assert.deepEqual([...counts], [['a', 1], ['b', 1]]);
  assert.equal(seen.voting?.anonymous, true);
  session.endVoting();
  assert.ok(!session.vote('k2', 'a'), 'a closed session takes no vote');
  assert.equal(seen.results().voters, 2, 'the results stay after the end');
  session.startVoting({ id: 'v2', perPerson: 3, anonymous: false, startedBy: 'anna' });
  assert.equal(seen.results().voters, 0, 'a new session starts empty');
});

test('the timer everyone sees runs, pauses and resumes by each device\'s clock (UC-CNV-05 check 2)', () => {
  const doc = new Y.Doc();
  const session = new Facilitation(doc);
  session.startTimer(5 * 60 * 1000, 1_000_000);
  assert.deepEqual(session.remaining(1_060_000), { left: 240_000, running: true });
  session.pauseTimer(1_060_000);
  assert.deepEqual(session.remaining(9_000_000), { left: 240_000, running: false });
  session.resumeTimer(2_000_000);
  assert.deepEqual(session.remaining(2_240_000), { left: 0, running: false });
  session.stopTimer();
  assert.equal(session.remaining(0), undefined);
});

test('the board exports as a PDF of one page holding its picture (UC-CNV-03 check 4)', () => {
  const jpeg = new Uint8Array([0xff, 0xd8, 0xff, 0xd9]);
  const pdf = pdfOfJpeg(jpeg, 800, 600, 'Retro – März');
  const text = new TextDecoder('latin1').decode(pdf);
  assert.ok(text.startsWith('%PDF-1.4'));
  assert.match(text, /\/MediaBox \[0 0 600 450\]/);
  assert.match(text, /\/Filter \/DCTDecode \/Length 4/);
  const xref = Number(/startxref\n(\d+)/.exec(text)?.[1]);
  assert.equal(text.slice(xref, xref + 4), 'xref', 'the cross-reference table is where the trailer says');
  const offsets = [...text.matchAll(/(\d{10}) 00000 n/g)].map((m) => Number(m[1]));
  offsets.forEach((offset, i) => assert.equal(text.slice(offset, offset + `${i + 1} 0 obj`.length), `${i + 1} 0 obj`));
  const sharp = new TextDecoder('latin1').decode(pdfOfJpeg(jpeg, 1600, 1200, 'x', { width: 800, height: 600 }));
  assert.match(sharp, /\/MediaBox \[0 0 600 450\]/, 'a picture drawn at twice the scale keeps its page');
  assert.match(sharp, /\/Width 1600 \/Height 1200/);
});

test('a stroke and a web link are drawn in the picture', () => {
  const svg = toSvg(
    [
      strokesOf([{ x: 0, y: 0 }, { x: 40, y: 40 }], 'HIGHLIGHTER', newId)[0] as Element,
      { id: 'w', kind: 'LINK', x: 0, y: 80, width: 280, height: 72, url: 'https://example.org/plan?a=1&b=2' },
    ],
    { title: 'x', unavailable: '-', colours: { paper: 'p', ink: 'i', line: 'l', surface: 's', label: () => undefined } },
  );
  assert.match(svg, /stroke-opacity="0.4"/);
  assert.match(svg, />example.org</);
  assert.match(svg, /a=1&amp;b=2/);
});

test('an older version brought back stands in place of the scene, for everyone at once (UC-CNV-06 check 3)', () => {
  const a = new Y.Doc();
  const b = new Y.Doc();
  a.on('update', (u: Uint8Array) => Y.applyUpdate(b, u));
  const board = new Board(a);
  board.addAll([sticky('x'), sticky('y', 200)]);
  const older: Element[] = [sticky('a'), { id: 'l', kind: 'CONNECTOR', from: 'a', to: 'a', x: 0, y: 0, width: 0, height: 0 }];
  assert.ok(board.replace(older));
  assert.deepEqual(new Board(b).elements().map((e) => e.id), ['a', 'l']);
});
