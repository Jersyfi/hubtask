// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The mark's geometry and its form rule (design-system.md §9, UC-WRK-33 check 4): the numbers are
// the owner's, and below the threshold the use case names the reduced form is drawn.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { formFor, FULL_MARK, LOGO_SPACE_STEP, knockedOutPlane, REDUCED_MARK, squarePath } from '../src/mark.ts';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const tokens = JSON.parse(fs.readFileSync(path.join(packageRoot, 'tokens', 'tokens.json'), 'utf8'));

/** UC-WRK-33 check 4: below this many pixels the reduced mark is used. */
const REDUCED_BELOW = 24;

test('every size step below the threshold draws the reduced form, every other the full one', () => {
  for (const [size, step] of Object.entries(LOGO_SPACE_STEP)) {
    const value = tokens.primitive.space[step]?.$value;
    assert.match(value ?? '', /^\d+px$/, `space step ${step} of ${size} is not a pixel length`);
    const pixels = Number.parseFloat(value);
    const expected = pixels < REDUCED_BELOW ? REDUCED_MARK : FULL_MARK;
    assert.equal(formFor(size), expected, `${size} (${value}) draws the wrong form`);
  }
  assert.ok(Object.values(LOGO_SPACE_STEP).some((step) => Number.parseFloat(tokens.primitive.space[step].$value) < REDUCED_BELOW));
});

test('the full form is the owner’s drawing', () => {
  assert.equal(FULL_MARK.outline.d, 'M42 5.5H22A16.5 16.5 0 0 0 5.5 22v20A16.5 16.5 0 0 0 22 58.5h20A16.5 16.5 0 0 0 58.5 42V30');
  assert.equal(FULL_MARK.outline.stroke, 7);
  assert.deepEqual(FULL_MARK.plane, { x: 15, y: 23, size: 26, r: 8.5 });
  assert.deepEqual(FULL_MARK.core, { x: 21, y: 31, size: 12, r: 4 });
  assert.deepEqual(FULL_MARK.spark, { cx: 54, cy: 12, r: 5.5 });
});

test('the reduced form has no core and stays inside the owner’s ranges', () => {
  assert.equal(REDUCED_MARK.outline.d, 'M42 6H22A16 16 0 0 0 6 22v20a16 16 0 0 0 16 16h20a16 16 0 0 0 16-16V30');
  assert.equal(REDUCED_MARK.core, undefined);
  assert.ok(REDUCED_MARK.outline.stroke >= 9 && REDUCED_MARK.outline.stroke <= 10);
  assert.ok(REDUCED_MARK.plane.size >= 24 && REDUCED_MARK.plane.size <= 26);
  assert.ok(REDUCED_MARK.spark.r >= 7 && REDUCED_MARK.spark.r <= 8);
});

test('the reduced form keeps a gap between the outline’s open end and the spark', () => {
  // The open end is the path's first point; its round cap reaches half the stroke beyond it.
  const [x, y] = REDUCED_MARK.outline.d.match(/^M([\d.]+) ([\d.]+)/).slice(1).map(Number);
  const { cx, cy, r } = REDUCED_MARK.spark;
  const gap = Math.hypot(cx - x, cy - y) - r - REDUCED_MARK.outline.stroke / 2;
  assert.ok(gap > 1, `the cap and the spark are ${gap.toFixed(2)} units apart`);
});

test('the mono plane is one path of two closed subpaths, the core knocked out', () => {
  const d = knockedOutPlane(FULL_MARK);
  assert.equal(d, `${squarePath(FULL_MARK.plane)}${squarePath(FULL_MARK.core)}`);
  assert.equal((d.match(/M/g) ?? []).length, 2);
  assert.equal((d.match(/Z/g) ?? []).length, 2);
  assert.equal(knockedOutPlane(REDUCED_MARK), squarePath(REDUCED_MARK.plane));
});

test('the mono variant names no colour but the text’s', () => {
  const source = fs.readFileSync(path.join(packageRoot, 'src', 'Logo.svelte'), 'utf8');
  const mono = [...source.matchAll(/\[data-variant='mono'\][^{]*\{([^}]*)\}/g)].map((m) => m[1]).join(' ');
  assert.ok(mono.length > 0, 'no mono rule found in Logo.svelte');
  for (const value of mono.matchAll(/(?:fill|stroke):\s*([^;]+);/g)) assert.equal(value[1].trim(), 'currentColor');
});

// The generated files (build/mark.js), rendered here from the same functions the build calls.

const { appIcon, favicon } = await import('../build/mark.js');
const { values } = await import('../dist/tokens.ts');

const colours = (svg) => [...svg.matchAll(/#[0-9a-fA-F]{3,8}\b/g)].map((m) => m[0].toLowerCase());
const brandValues = (mode) => new Set(Object.values(values[mode].brand).map((v) => String(v).toLowerCase()));

test('the tab icon is the reduced mark, light by attributes and dark by one media rule', () => {
  const svg = favicon(values.light.brand, values.dark.brand);
  assert.ok(svg.includes(`d="${REDUCED_MARK.outline.d}"`), 'the tab icon does not draw the reduced outline');
  assert.ok(!svg.includes(squarePath(FULL_MARK.core)), 'the tab icon draws a core');
  const [rule] = svg.match(/<style>[\s\S]*<\/style>/) ?? [''];
  assert.match(rule, /^<style>@media \(prefers-color-scheme: dark\)\{[^@]*\}<\/style>$/, 'the style holds more than the dark rule');
  for (const colour of colours(rule)) assert.ok(brandValues('dark').has(colour), `${colour} is not a dark brand value`);
  for (const colour of colours(svg.replace(rule, ''))) assert.ok(brandValues('light').has(colour), `${colour} is not a light brand value`);
  assert.equal(svg.match(/stroke="([^"]+)"/)[1], values.light.brand.outline);
});

test('the app icon source is the full mark on the tile, in brand colours only', () => {
  const svg = appIcon(values.light.brand);
  assert.ok(svg.includes(`d="${FULL_MARK.outline.d}"`));
  assert.ok(svg.includes(squarePath(FULL_MARK.core)));
  assert.ok(svg.includes(`fill="${values.light.brand.tile}"`));
  assert.ok(svg.includes(`stroke="${values.light.brand.tileOutline}"`));
  for (const colour of colours(svg)) assert.ok(brandValues('light').has(colour), `${colour} is not a brand value`);
  assert.ok(!svg.includes('<style'), 'the app icon has one form and needs no style');
});
