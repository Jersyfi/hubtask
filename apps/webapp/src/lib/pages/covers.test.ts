// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { GALLERY, GRADIENTS, clampPosition, coverBackground, draggedPosition } from './covers.ts';

test('a cover draws from tokens alone: twelve gradients, a gallery, every colour a variable', () => {
  assert.equal(Object.keys(GRADIENTS).length, 12);
  assert.ok(Object.keys(GALLERY).length >= 6);
  for (const key of [...Object.keys(GRADIENTS), ...Object.keys(GALLERY)]) assert.match(key, /^[a-z0-9][a-z0-9-]{0,39}$/u, 'a key the server accepts');
  for (const drawn of [...Object.values(GRADIENTS).flat(), ...Object.values(GALLERY)]) assert.doesNotMatch(drawn, /#[0-9a-f]{3,8}\b|rgb|hsl/iu, 'a colour outside the tokens');
  // The design system names seven chart colours; a var() past them draws nothing.
  for (const drawn of [...Object.values(GRADIENTS).flat(), ...Object.values(GALLERY)]) {
    for (const [, n] of drawn.matchAll(/--chart-categorical-(\d+)/gu)) assert.ok(Number(n) >= 1 && Number(n) <= 7, `chart colour ${n}`);
  }
  assert.equal(coverBackground({ source: 'GRADIENT', key: 'indigo-teal', position: 50 }), 'linear-gradient(120deg, var(--chart-categorical-1), var(--chart-categorical-2))');
});

test('a key this client does not know, or an image not resolved, draws no cover', () => {
  assert.equal(coverBackground({ source: 'GRADIENT', key: 'unknown', position: 50 }), undefined);
  assert.equal(coverBackground({ source: 'GALLERY', key: 'unknown', position: 50 }), undefined);
  assert.equal(coverBackground({ source: 'IMAGE', key: 'm', position: 50 }), undefined);
  assert.equal(coverBackground({ source: 'IMAGE', key: 'm', position: 30 }, 'https://h/x.png'), 'url("https://h/x.png") center 30% / cover no-repeat');
});

test('a cover is moved by dragging, between its top and its foot', () => {
  assert.equal(draggedPosition(50, 50, 200), 25, 'dragged down shows more of the top');
  assert.equal(draggedPosition(50, -400, 200), 100);
  assert.equal(draggedPosition(10, 400, 200), 0);
  assert.equal(clampPosition(33.6), 34);
});
