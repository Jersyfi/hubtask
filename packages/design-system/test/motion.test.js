// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic behind the product's movements (user-experience.md §6): the easing a token names,
// the durations as the CSS spells them, and the stagger that keeps a long list from being waited for.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { STAGGERED, cubicBezier, parseDuration, parseEasing, parseLength, roleOf, staggerDelay } from '../src/motion.ts';

const here = path.dirname(fileURLToPath(import.meta.url));
const source = JSON.parse(fs.readFileSync(path.join(here, '..', 'tokens', 'tokens.json'), 'utf8'));
const css = fs.readFileSync(path.join(here, '..', 'dist', 'tokens.css'), 'utf8');

test('a cubic Bézier starts at nought, ends at one and follows its curve between', () => {
  const standard = cubicBezier(0.2, 0, 0, 1);
  assert.equal(standard(0), 0);
  assert.equal(standard(1), 1);
  assert.ok(standard(0.5) > 0.5, 'the standard curve has done more than half its way at half its time');
  const straight = cubicBezier(0, 0, 1, 1);
  for (const t of [0.1, 0.25, 0.5, 0.9]) assert.ok(Math.abs(straight(t) - t) < 1e-4, `the diagonal curve is linear at ${t}`);
  const exit = cubicBezier(0.3, 0, 0.8, 0.15);
  assert.ok(exit(0.5) < 0.5, 'an exit starts slowly and leaves fast');
  let previous = 0;
  for (let t = 0; t <= 1; t += 0.05) {
    const value = exit(t);
    assert.ok(value >= previous - 1e-9, `the curve never runs backwards (at ${t.toFixed(2)})`);
    previous = value;
  }
});

test('durations, lengths and easings are read as the computed style spells them', () => {
  assert.equal(parseDuration('180ms'), 180);
  assert.equal(parseDuration(' 0.26s'), 260);
  assert.equal(parseDuration(''), 0, 'a token sheet that is not loaded moves nothing');
  assert.equal(parseDuration('var(--dur-base)'), 0);
  assert.equal(parseLength('8px'), 8);
  assert.equal(parseLength(''), 0);
  const easing = parseEasing('cubic-bezier(0.05, 0.7, 0.1, 1)');
  assert.ok(easing(0.2) > 0.5, 'the entrance curve arrives early');
  assert.equal(parseEasing('linear')(0.3), 0.3);
  assert.equal(parseEasing('cubic-bezier(a, b)')(0.3), 0.3, 'what cannot be read is linear, never a throw');
});

test('one arrival staggers its first eight and lets the rest arrive with the eighth', () => {
  assert.equal(STAGGERED, 8);
  assert.equal(staggerDelay(0, 30), 0, 'the first arrives at once');
  assert.equal(staggerDelay(1, 30), 30);
  assert.equal(staggerDelay(7, 30), 210);
  assert.equal(staggerDelay(40, 30), 210, 'a long list is never waited for');
  assert.equal(staggerDelay(-1, 30), 0);
});

test('every role this module moves by is a token pair the CSS declares', () => {
  for (const role of ['entrance', 'exit', 'reorder', 'reveal']) {
    assert.ok(source.motion[role], `motion.${role} is in tokens.json`);
    assert.match(css, new RegExp(`--motion-${role}-duration:`), `the CSS declares --motion-${role}-duration`);
    assert.match(css, new RegExp(`--motion-${role}-easing:`), `the CSS declares --motion-${role}-easing`);
  }
  assert.match(css, /--dur-stagger: 30ms;/, 'the stagger step is a primitive duration');
  const style = { getPropertyValue: (name) => ({ '--motion-reorder-duration': '260ms', '--motion-reorder-easing': ' cubic-bezier(0.2, 0, 0, 1)' })[name] ?? '' };
  assert.deepEqual(roleOf(style, 'reorder'), { duration: 260, easing: 'cubic-bezier(0.2, 0, 0, 1)' });
});

test('the components whose lists move draw them through this module', () => {
  const read = (name) => fs.readFileSync(path.join(here, '..', 'src', name), 'utf8');
  assert.match(read('Table.svelte'), /<tbody \{@attach arrivals\}>/, 'a table row arrives');
  assert.match(read('BucketColumn.svelte'), /class="cards" \{@attach arrivals\}/, 'a board card arrives');
  assert.match(read('ListRow.svelte'), /\{@attach arrive\}/, 'a list row arrives');
  assert.match(read('TaskRow.svelte'), /<ListRow /, 'a task row arrives as the list row it draws');
});
