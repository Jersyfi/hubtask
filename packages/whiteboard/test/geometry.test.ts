// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import { connectorPath, culled, ends, fit, hit, toSvg, withinFrame, zoomAt, type Element } from '../src/index.ts';

const a: Element = { id: 'a', kind: 'STICKY', x: 0, y: 0, width: 100, height: 100 };
const b: Element = { id: 'b', kind: 'SHAPE', x: 300, y: 0, width: 100, height: 100 };
const link: Element = { id: 'l', kind: 'CONNECTOR', from: 'a', to: 'b', x: 0, y: 0, width: 0, height: 0 };

test('a connector runs between the outlines of its objects and follows one that moves (UC-CNV-01 check 2)', () => {
  const byId = new Map([a, b].map((e) => [e.id, e]));
  assert.deepEqual(ends(link, byId), [{ x: 100, y: 50 }, { x: 300, y: 50 }]);
  assert.equal(connectorPath(link, byId), 'M 100 50 L 300 50');
  const moved = { ...b, y: 200 };
  const [, end] = ends(link, new Map([a, moved].map((e) => [e.id, e])));
  assert.ok(end.y > 200 && end.y < 300 && end.x >= 300, `the end follows the shape: ${JSON.stringify(end)}`);
  assert.match(connectorPath({ ...link, route: 'ELBOW' }, byId), /^M 100 50 H 200 V 50 H 300$/);
  assert.match(connectorPath({ ...link, route: 'CURVED' }, byId), /^M 100 50 C /);
  const free: Element = { id: 'f', kind: 'CONNECTOR', x: 10, y: 10, width: -5, height: 20 };
  assert.deepEqual(ends(free, byId), [{ x: 10, y: 10 }, { x: 5, y: 30 }]);
});

test('only what the viewport shows is drawn, a board of thousands included', () => {
  const many: Element[] = Array.from({ length: 5000 }, (_, i) => ({ id: `e${i}`, kind: 'STICKY', x: (i % 100) * 200, y: Math.floor(i / 100) * 200, width: 160, height: 160 }));
  const shown = culled(many, { x: 0, y: 0, zoom: 1, width: 1000, height: 600 });
  assert.ok(shown.length > 0 && shown.length < 50, `${shown.length} drawn`);
});

test('a point hits the top-most object, a frame only where nothing else is', () => {
  const frame: Element = { id: 'f', kind: 'FRAME', x: -50, y: -50, width: 600, height: 300 };
  const all = [frame, a, b];
  assert.equal(hit(all, { x: 50, y: 50 })?.id, 'a');
  assert.equal(hit(all, { x: 200, y: 200 })?.id, 'f');
  assert.equal(hit(all, { x: 900, y: 900 }), undefined);
  assert.deepEqual(withinFrame([frame, a, b, link, { ...a, id: 'out', x: 900 }], frame).map((e) => e.id), ['f', 'a', 'b', 'l']);
});

test('zooming keeps the point under the pointer, and fitting shows a box whole', () => {
  const view = { x: 0, y: 0, zoom: 1, width: 800, height: 600 };
  const zoomed = zoomAt(view, { x: 400, y: 300 }, 2);
  assert.equal(zoomed.zoom, 2);
  assert.equal(zoomed.x + 400 / zoomed.zoom, 400);
  const fitted = fit(view, { left: 0, top: 0, right: 1600, bottom: 100 });
  assert.ok(fitted.zoom < 1);
});

test('a board, a frame or a selection is drawn as an SVG document with its words escaped (UC-CNV-03 check 2)', () => {
  const svg = toSvg([{ ...a, text: 'Fish & <chips>', color_token: 'amber' }, b, link, { id: 'c', kind: 'CARD', x: 0, y: 300, width: 260, height: 112, target: { kind: 'ENTRY', id: 'e1' } }], {
    title: 'Retro',
    colours: { paper: 'paper', ink: 'ink', line: 'line', surface: 'surface', label: (token) => (token === 'amber' ? 'amber-colour' : undefined) },
    card: (id) => (id === 'e1' ? { title: 'Book the train', key: 'trv-12', completed: true } : undefined),
    unavailable: 'Not available',
  });
  assert.match(svg, /^<svg xmlns="http:\/\/www.w3.org\/2000\/svg"/);
  assert.match(svg, />Fish &amp;<\/text>.*>&lt;chips&gt;</);
  assert.match(svg, /fill="amber-colour"/);
  assert.match(svg, /Book the train/);
  assert.match(svg, /trv-12 ✓/);
  assert.match(svg, /marker-end="url\(#arrow\)"/);
  assert.doesNotMatch(svg, /<chips>/);
});

test('a picture is drawn from its upload, a file as its name, and one out of reach as a placeholder (UC-CNV-08 check 1)', () => {
  const picture: Element = { id: 'i', kind: 'IMAGE', x: 0, y: 0, width: 320, height: 200, media_id: 'm1', text: 'The site' };
  const file: Element = { id: 'f', kind: 'FILE', x: 0, y: 300, width: 280, height: 72, media_id: 'm2' };
  const lost: Element = { id: 'l', kind: 'IMAGE', x: 400, y: 0, width: 320, height: 200, media_id: 'm3' };
  const svg = toSvg([picture, file, lost], {
    title: 'Moodboard',
    colours: { paper: 'paper', ink: 'ink', line: 'line', surface: 'surface', label: () => undefined },
    unavailable: 'Not available',
    media: (id) => (id === 'm1' ? { href: 'data:image/png;base64,AAAA', name: 'site.png' } : id === 'm2' ? { name: 'plan.pdf' } : undefined),
  });
  assert.match(svg, /<image href="data:image\/png;base64,AAAA" x="0" y="0" width="320" height="200"/);
  assert.match(svg, /The site/);
  assert.match(svg, /plan\.pdf/);
  assert.match(svg, /Not available/);
});
