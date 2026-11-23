// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// MultiSelect (design-system.md §4, wave 1), checked without a browser: the arithmetic of the
// selection in multiselect.ts, and the roles the component promises, as a reading of its source.
// The keyboard, the focus and the sheet are walked in apps/webapp/e2e/workbench-multiselect.test.mjs.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import {
  FILTER_FROM,
  INLINE_MOST,
  closedOf,
  cutChips,
  filterSections,
  hasFilter,
  isFixed,
  isLocked,
  presentationOf,
  sectionsOf,
  toggled,
  withAll,
  withNone,
} from '../src/multiselect.ts';

const option = (value, extra = {}) => ({ value, label: value.toUpperCase(), ...extra });
const many = (count) => Array.from({ length: count }, (_, index) => option(`o${index}`));

// --- which presentation ------------------------------------------------------------------------

test('auto draws up to five options as ticks and six or more as a field', () => {
  assert.equal(INLINE_MOST, 5);
  for (const count of [0, 1, 2, 5]) assert.equal(presentationOf('auto', count), 'inline', `${count} options`);
  for (const count of [6, 15, 45]) assert.equal(presentationOf('auto', count), 'field', `${count} options`);
});

test('a presentation the caller forces is kept, whatever the count', () => {
  assert.equal(presentationOf('inline', 15), 'inline');
  assert.equal(presentationOf('field', 2), 'field');
});

// --- toggling ------------------------------------------------------------------------------------

test('a toggle keeps the order the caller’s selection has, and adds at its end', () => {
  const options = many(4);
  assert.deepEqual(toggled(['o2', 'o0'], 'o1', options), ['o2', 'o0', 'o1']);
  assert.deepEqual(toggled(['o2', 'o0', 'o1'], 'o0', options), ['o2', 'o1']);
});

test('a value the options do not name stays chosen', () => {
  // The caller's list may know a value this list does not draw - a capability added later. A
  // toggle of another option must not drop it.
  assert.deepEqual(toggled(['later', 'o0'], 'o1', many(2)), ['later', 'o0', 'o1']);
  assert.deepEqual(withNone(['later', 'o0'], many(2)), ['later']);
});

test('an option with a reason does not toggle, ticked or not', () => {
  const options = [option('locked', { disabledReason: 'Every entry can be done.' }), option('refused', { disabledReason: 'Needs a level above.' }), option('free')];
  assert.deepEqual(toggled(['locked'], 'locked', options), ['locked'], 'a locked option is cleared');
  assert.deepEqual(toggled(['locked'], 'refused', options), ['locked'], 'a refused option is ticked');
  assert.equal(isLocked(options[0], ['locked']), true);
  assert.equal(isLocked(options[1], ['locked']), false);
  assert.equal(isFixed(options[1]), true);
  assert.equal(isFixed(options[2]), false);
});

test('All and None skip an option with a reason', () => {
  const options = [option('locked', { disabledReason: 'Always on.' }), option('a'), option('refused', { disabledReason: 'Not here.' }), option('b')];
  assert.deepEqual(withAll(['locked'], options), ['locked', 'a', 'b']);
  assert.deepEqual(withNone(['locked', 'a', 'b'], options), ['locked'], 'None clears a locked option');
  assert.deepEqual(withAll(['b'], options), ['b', 'a'], 'All keeps the order and adds the rest in the options’ order');
});

test('All and None reach only the options they are given - the ones the filter shows', () => {
  const options = many(4);
  const shown = options.slice(0, 2);
  assert.deepEqual(withAll(['o3'], shown), ['o3', 'o0', 'o1']);
  assert.deepEqual(withNone(['o0', 'o3'], shown), ['o3']);
});

// --- the closed field ---------------------------------------------------------------------------

test('nothing chosen says the caller’s empty text', () => {
  assert.deepEqual(closedOf(many(6), [], { emptyLabel: 'Nothing chosen' }), { kind: 'empty', text: 'Nothing chosen' });
});

test('the caller’s sentence replaces the chips, and undefined from it keeps them', () => {
  const options = many(6);
  const summary = (chosen, total) => (chosen.length === total ? `All ${total}` : undefined);
  assert.deepEqual(closedOf(options, options.map((each) => each.value), { emptyLabel: '-', summary }), { kind: 'summary', text: 'All 6' });
  const chips = closedOf(options, ['o1'], { emptyLabel: '-', summary });
  assert.equal(chips.kind, 'chips');
  assert.deepEqual(chips.chips.map((each) => each.value), ['o1']);
});

test('the chips are the chosen in the options’ order, a locked one first', () => {
  const options = [option('a'), option('b'), option('done', { disabledReason: 'Always on.' }), option('c')];
  const closed = closedOf(options, ['c', 'done', 'a'], { emptyLabel: '-' });
  assert.deepEqual(closed.chips.map((each) => each.value), ['done', 'a', 'c']);
  assert.deepEqual(closed.chips.map((each) => each.isLocked), [true, false, false]);
});

test('the summary is told the chosen options and the count of all', () => {
  let told;
  closedOf(many(7), ['o3', 'o1'], { emptyLabel: '-', summary: (chosen, total) => { told = [chosen.map((each) => each.value), total]; return undefined; } });
  assert.deepEqual(told, [['o1', 'o3'], 7]);
});

test('the chips are cut where the two lines end, one place kept for the count of the rest', () => {
  const chips = many(9);
  assert.deepEqual(cutChips(chips, 9), { shown: chips, more: 0 }, 'all fit');
  assert.deepEqual(cutChips(chips, 20), { shown: chips, more: 0 });
  const cut = cutChips(chips, 5);
  assert.equal(cut.shown.length, 4, 'the fifth place goes to the count');
  assert.equal(cut.more, 5);
  assert.deepEqual(cutChips(chips, 1), { shown: chips.slice(0, 1), more: 8 }, 'at least one chip is shown');
  assert.deepEqual(cutChips(chips, 0), { shown: chips.slice(0, 1), more: 8 });
});

// --- the filter and the groups ------------------------------------------------------------------

test('the filter appears from eight options', () => {
  assert.equal(FILTER_FROM, 8);
  assert.equal(hasFilter(7), false);
  assert.equal(hasFilter(8), true);
});

test('options without a group come first, as a section with no heading', () => {
  const sections = sectionsOf([option('a')], [{ label: 'People', options: [option('b')] }, { label: 'Empty', options: [] }]);
  assert.deepEqual(sections.map((each) => [each.label, each.options.map((one) => one.value)]), [[undefined, ['a']], ['People', ['b']]], 'a group of none is not drawn');
});

test('the filter matches the label and the hint, folded in the reader’s locale', () => {
  const sections = sectionsOf([], [
    { label: 'Time', options: [option('due', { label: 'Dates', hint: 'When it is due' }), option('rem', { label: 'Reminders' })] },
    { label: 'People', options: [option('who', { label: 'Responsible person' })] },
  ]);
  assert.deepEqual(filterSections(sections, '  DUE ').map((each) => each.options.map((one) => one.value)), [['due']], 'by the hint, trimmed and folded');
  assert.deepEqual(filterSections(sections, 'person').map((each) => each.label), ['People'], 'a group with no match is left out');
  assert.deepEqual(filterSections(sections, 'zzz'), []);
  assert.equal(filterSections(sections, ''), sections, 'no term, nothing filtered');
  const turkish = sectionsOf([option('i', { label: 'İş' })], []);
  assert.equal(filterSections(turkish, 'iş', 'tr').length, 1, 'a capital İ folds to i under tr');
});

// --- the component, read ------------------------------------------------------------------------

const sourceDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
const source = fs.readFileSync(path.join(sourceDir, 'MultiSelect.svelte'), 'utf8');
const markup = source.slice(source.lastIndexOf('</script>'), source.indexOf('<style>'));

test('the closed field is one button that says it opens a list', () => {
  const field = markup.slice(markup.indexOf('class="field"') - 200, markup.indexOf('class="field"') + 600);
  assert.match(field, /<button/, 'the field is a button');
  assert.match(field, /aria-haspopup="listbox"/);
  assert.match(field, /aria-expanded=\{isOpen\}/);
  assert.match(markup, /class="multiselect"[^>]*data-invalid=\{error/, 'an invalid field marks its root: a button takes no aria-invalid');
  assert.doesNotMatch(field, /aria-invalid=/);
  assert.doesNotMatch(markup.slice(markup.indexOf('class="chips"'), markup.indexOf('class="chips"') + 400), /<button/, 'a chip is a summary, not a button');
});

test('the list is a multi-selectable listbox of options, grouped by named groups', () => {
  assert.match(markup, /role="listbox"[^>]*aria-multiselectable="true"|aria-multiselectable="true"[^>]*role="listbox"/);
  assert.match(markup, /role="option"/);
  assert.match(markup, /aria-selected=\{/);
  assert.match(markup, /role="group"[^>]*aria-labelledby=|aria-labelledby=[^>]*role="group"/, 'a group is named by its heading');
});

test('an option is named by its label alone; its hint and its reason describe it', () => {
  const opt = markup.slice(markup.indexOf('role="option"') - 400, markup.indexOf('role="option"') + 800);
  assert.match(opt, /aria-labelledby=\{`\$\{optionId\}-label`\}/);
  assert.match(opt, /aria-describedby=/);
  assert.match(opt, /aria-disabled=/, 'an option that cannot change says so and stays focusable');
  assert.doesNotMatch(opt, /\sdisabled[=\s>]/);
});

test('the inline ticks are the design system’s Checkbox, each its own stop', () => {
  const ticks = fs.readFileSync(path.join(sourceDir, '_Ticks.svelte'), 'utf8');
  assert.match(ticks, /import Checkbox from '\.\/Checkbox\.svelte'/);
  assert.match(ticks, /<Checkbox/);
  assert.match(source, /import Ticks from '\.\/_Ticks\.svelte'/);
  assert.match(markup, /<Ticks /);
  assert.match(markup, /role="group"/);
});
