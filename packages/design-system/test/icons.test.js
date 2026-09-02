// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the icon set has to keep being true (ADR-0041).
//
// The committed `src/icons/base.ts` is generated, which makes it editable and therefore worth
// checking: these are the properties that would otherwise be true only on the day `make icons`
// last ran. The contract that matters most is the smallest - no mark names a colour - because an
// icon that did would be the one place in the product where rule 15 could be broken invisibly,
// `lint-no-literals` not reading generated output.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { DECLARED, OUTPUT, render, sourceAvailable } from '../build/icons.js';
import { BASE_ICONS, CUSTOM_ICONS, ICONS, ICON_NAMES } from '../src/icons/index.ts';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = (...p) => fs.readFileSync(path.join(packageRoot, ...p), 'utf8');

const COLOUR = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?)\(/;

test('the generated set is exactly what build/icons.js declares', () => {
  const declared = Object.values(DECLARED).flat().sort();
  assert.deepEqual(
    Object.keys(BASE_ICONS).sort(),
    declared,
    'src/icons/base.ts is out of step with the declared list - run `make icons` and commit it',
  );
});

test('the generated file is marked as generated', () => {
  assert.match(read('src', 'icons', 'base.ts'), /DO NOT EDIT/);
});

test('the committed file is what the generator produces today', (t) => {
  // The counterpart of `make generate` producing no diff. lucide-static is a devDependency, so
  // this can only run where it is installed - which is every CI run and every contributor who has
  // installed the workspace, and nowhere that would fail for the wrong reason.
  if (!sourceAvailable()) return t.skip('lucide-static is not installed');
  assert.equal(
    fs.readFileSync(OUTPUT, 'utf8'),
    render().file,
    'src/icons/base.ts is stale - run `make icons` and commit the result',
  );
});

test('no icon names a colour', () => {
  for (const [name, nodes] of Object.entries(ICONS)) {
    for (const [tag, attributes] of nodes) {
      for (const [key, value] of Object.entries(attributes)) {
        assert.ok(
          !COLOUR.test(String(value)),
          `${name} carries a colour in <${tag} ${key}="${value}">. An icon takes the colour of the ` +
            'text it sits in (currentColor); a value lives in tokens.json (rule 15).',
        );
      }
    }
  }
});

test('an icon paints only in the colour of the text it sits in', () => {
  // `fill="currentColor"` is legitimate and several icons need it - the dot in `tag` is a filled
  // shape, not a stroked one. What is forbidden is any *other* value: a mark that set one would
  // look right in the theme it was drawn in and wrong in the other, which is the whole reason the
  // set inherits its colour. `stroke-width` is the wrapper's alone, because it is what makes 16 px
  // and 24 px read as the same weight.
  const allowed = new Set(['currentColor', 'none']);
  for (const [name, nodes] of Object.entries(ICONS)) {
    for (const [, attributes] of nodes) {
      for (const key of ['stroke', 'fill']) {
        if (!(key in attributes)) continue;
        assert.ok(
          allowed.has(attributes[key]),
          `${name} sets ${key}="${attributes[key]}"; only currentColor and none are colours an icon may name`,
        );
      }
      assert.ok(!('stroke-width' in attributes), `${name} sets its own stroke-width; Icon.svelte owns it`);
    }
  }
});

test('the two sets do not overlap, and the union is what the product may ask for', () => {
  const overlap = Object.keys(CUSTOM_ICONS).filter((name) => name in BASE_ICONS);
  assert.deepEqual(overlap, [], 'a name in both sets makes one of the two unreachable');
  assert.equal(ICON_NAMES.length, Object.keys(BASE_ICONS).length + Object.keys(CUSTOM_ICONS).length);
});

test('every mark is drawn on the 24 grid the wrapper declares', () => {
  // Coordinates outside 0..24 are not wrong in themselves - a curve may overshoot - but a mark
  // drawn on a 256 grid by accident would be invisible, and that is worth catching once.
  const numbers = /-?\d+(?:\.\d+)?/g;
  for (const [name, nodes] of Object.entries(CUSTOM_ICONS)) {
    for (const [, attributes] of nodes) {
      for (const value of Object.values(attributes)) {
        for (const found of String(value).match(numbers) ?? []) {
          assert.ok(
            Math.abs(Number(found)) <= 26,
            `${name} has the coordinate ${found}, which is off a 24x24 grid`,
          );
        }
      }
    }
  }
});

test('the domain nouns the specification names all have a mark', () => {
  // design-system.md §9 named these. A mark quietly dropped in a refactor is a domain noun the
  // product can no longer point at.
  for (const noun of ['hub', 'collection', 'task', 'work-package', 'activity', 'jumble', 'bucket', 'capability']) {
    assert.ok(noun in CUSTOM_ICONS, `there is no mark for '${noun}'`);
  }
});
