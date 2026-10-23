// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-25 checks 1 and 2: lines become entries, a leading amount becomes the entry's amount; and
// the one rule both capture fields read a collection by (UC-WRK-26 check 3).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { capture, captureRulesOf, MAX_CAPTURE_LINES, parseLine, unitsOf, type CaptureRules } from './capture.ts';

const en = unitsOf('g kg mg l ml cl dl oz lb pcs');
const de = unitsOf('g kg mg l ml cl dl oz lb Stk Pkg Bund Dose EL TL');
const list: CaptureRules = { splitsLines: true, readsAmount: true, units: en };

test('a leading amount is read off the line, and what is not one stays in the title', () => {
  const cases: readonly (readonly [string, string, string | undefined, (readonly string[])?])[] = [
    ['2 milk', 'milk', '2'],
    ['1 kg apples', 'apples', '1 kg'],
    ['1kg apples', 'apples', '1 kg'],
    ['1,5 l milk', 'milk', '1,5 l'],
    ['0.25 l cream', 'cream', '0.25 l'],
    ['½ cabbage', 'cabbage', '½'],
    ['1½ l water', 'water', '1½ l'],
    ['1/2 cake', 'cake', '1/2'],
    ['2x eggs', 'eggs', '2×'],
    ['2 × eggs', 'eggs', '2×'],
    ['٢ milk', 'milk', '2'],
    ['२ केले', 'केले', '2'],
    ['3 Stk Eier', 'Eier', '3 Stk', de],
    ['3 stk Eier', 'Eier', '3 stk', de],
    ['3 Stk Eier', 'Stk Eier', '3'],
    ['2 milk, the good one', 'milk, the good one', '2'],
    ['2-3 apples', '2-3 apples', undefined],
    ['1.000 g flour', '1.000 g flour', undefined],
    ['7up', '7up', undefined],
    ['2', '2', undefined],
    ['2  ', '2', undefined],
    ['1 kg', '1 kg', undefined],
    ['kg apples', 'kg apples', undefined],
    ['milk', 'milk', undefined],
  ];
  for (const [line, title, amount, units] of cases) {
    const parsed = parseLine(line, { ...list, units: units ?? en });
    assert.deepEqual(parsed, amount === undefined ? { title } : { title, amount }, line);
  }
});

test('without AMOUNT the line is the title, number and all', () => {
  assert.deepEqual(parseLine('2 milk', { ...list, readsAmount: false }), { title: '2 milk' });
});

test('several lines are several entries, in order, blank lines and bullets dropped', () => {
  const result = capture('- 2 milk\r\n\n• bread\n  * 1 kg apples  \n[ ] eggs\n[x] tea\n   ', list);
  assert.deepEqual(result.lines, [
    { title: 'milk', amount: '2' },
    { title: 'bread' },
    { title: 'apples', amount: '1 kg' },
    { title: 'eggs' },
    { title: 'tea' },
  ]);
  assert.equal(result.rest, '');
});

test('past the limit the lines stay in the field, as typed', () => {
  const typed = Array.from({ length: MAX_CAPTURE_LINES + 2 }, (_, index) => `line ${index + 1}`).join('\n');
  const result = capture(typed, list);
  assert.equal(result.lines.length, MAX_CAPTURE_LINES);
  assert.equal(result.lines.at(-1)?.title, `line ${MAX_CAPTURE_LINES}`);
  assert.equal(result.rest, `line ${MAX_CAPTURE_LINES + 1}\nline ${MAX_CAPTURE_LINES + 2}`);
});

test('a collection that does not split lines takes one entry', () => {
  assert.deepEqual(capture('2 milk\nand bread', { ...list, splitsLines: false }).lines, [
    { title: 'milk and bread', amount: '2' },
  ]);
  assert.deepEqual(capture('  \n ', list).lines, []);
});

test('one rule for every capture field: lines split at one level or LINES, amounts where AMOUNT is offered (D7)', () => {
  const structure = (settings: Record<string, unknown> | undefined) =>
    (settings === undefined ? undefined : { settings }) as Parameters<typeof captureRulesOf>[0];
  const withAmount = { TASK: { capabilities: ['COMPLETION', 'AMOUNT'] } };
  const withoutAmount = { TASK: { capabilities: ['COMPLETION', 'DUE_DATE'] } };
  const one = [{ type: 'TASK' }];
  const two = [{ type: 'TASK' }, { type: 'ACTIVITY' }];
  const cases: readonly (readonly [string, Record<string, unknown>, boolean, boolean])[] = [
    ['a list: one level, LINES and AMOUNT', { levels: one, fields: withAmount, capture: ['LINES', 'AMOUNT'] }, true, true],
    ['one level without LINES still splits', { levels: one, fields: withAmount, capture: ['DATE'] }, true, false],
    ['two levels with LINES split', { levels: two, fields: withAmount, capture: ['LINES'] }, true, false],
    ['two levels without LINES do not', { levels: two, fields: withAmount, capture: ['DATE', 'AMOUNT'] }, false, true],
    ['AMOUNT recognised, the type without it', { levels: one, fields: withoutAmount, capture: ['AMOUNT'] }, true, false],
    ['the type with AMOUNT, AMOUNT not recognised', { levels: one, fields: withAmount, capture: ['LINES'] }, true, false],
    ['no levels answered is every type, not one level', { fields: withAmount, capture: ['DATE'] }, false, false],
    ['no capture answered recognises neither', { levels: two, fields: withAmount }, false, false],
  ];
  for (const [name, settings, splitsLines, readsAmount] of cases) {
    assert.deepEqual(captureRulesOf(structure(settings), 'TASK', en), { splitsLines, readsAmount, units: en }, name);
  }
  assert.deepEqual(captureRulesOf(undefined, 'TASK', de), { splitsLines: false, readsAmount: false, units: de }, 'no structure read yet');
  // The type the capture creates decides the amount: another type's AMOUNT is not this one's.
  const amounted = structure({ levels: one, fields: withAmount, capture: ['AMOUNT'] });
  assert.equal(captureRulesOf(amounted, 'ACTIVITY', en).readsAmount, false);
});
