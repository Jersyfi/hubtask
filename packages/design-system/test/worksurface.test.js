// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The work surface's blocks (design-system.md §4, wave 7), checked without a browser: the arithmetic
// behind them as functions over data, and the keyboard and ARIA contract each block promises as a
// reading of its source - the same bargain `conventions.test.js` makes.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { LOZENGE_VOCABULARIES, lozengeLook } from '../src/lozenge.ts';
import { defaultSteps, primaryStep, splitSteps } from '../src/steps.ts';
import { foldWindow } from '../src/fold.ts';
import { SWIPE_MIN, swipeIntent, swipeOffset, swipeOutcome } from '../src/swipe.ts';
import { readDensity, writeDensity } from '../src/density.ts';
import { lineText, suggest } from '../src/query.ts';

const sourceDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
const source = (name) => fs.readFileSync(path.join(sourceDir, name), 'utf8');

// --- StateLozenge ---------------------------------------------------------------------------

test('every state whose tone says something also carries a glyph (UX-7)', () => {
  for (const [vocabulary, states] of Object.entries(LOZENGE_VOCABULARIES)) {
    for (const [state, look] of Object.entries(states)) {
      if (look.tone === 'neutral' || look.tone === 'info') continue;
      assert.ok(look.icon, `${vocabulary}.${state} is ${look.tone} without a glyph: colour alone`);
    }
  }
});

test('one state looks the same in every vocabulary that has it', () => {
  assert.deepEqual(lozengeLook('goal', 'atRisk'), lozengeLook('sla', 'atRisk'));
  assert.equal(lozengeLook('sla', 'breached').tone, 'danger');
});

test('an unknown state still has a look, so its word still stands', () => {
  assert.deepEqual(lozengeLook('category', 'archived-by-a-newer-server'), { tone: 'neutral' });
});

// --- StatusButton ---------------------------------------------------------------------------

const todo = { id: 'ready', label: 'Ready', category: 'todo' };
const progress = { id: 'progress', label: 'In progress', category: 'progress' };
const review = { id: 'review', label: 'In review', category: 'progress' };
const done = { id: 'done', label: 'Done', category: 'done' };

test('allowed and blocked steps are split, each keeping the server’s order', () => {
  const steps = [
    { id: 'a', label: 'A', to: review },
    { id: 'b', label: 'B', to: done, blockedReason: 'only the owner' },
    { id: 'c', label: 'C', to: todo },
  ];
  const { allowed, blocked } = splitSteps(steps);
  assert.deepEqual(allowed.map((step) => step.id), ['a', 'c']);
  assert.deepEqual(blocked.map((step) => step.id), ['b']);
});

test('the primary is the workflow’s next step, but never one the reader may not take', () => {
  const marked = { id: 'review', label: 'Request review', to: review, isNext: true };
  assert.equal(primaryStep(progress, [{ id: 'back', label: 'Put back', to: todo }, marked])?.id, 'review');
  const refused = { ...marked, blockedReason: 'needs story points' };
  assert.equal(primaryStep(progress, [{ id: 'back', label: 'Put back', to: todo }, refused, { id: 'finish', label: 'Finish', to: done }])?.id, 'finish');
});

test('a step back is never the primary', () => {
  assert.equal(primaryStep(progress, [{ id: 'back', label: 'Put back', to: todo }]), undefined);
});

test('without a workflow every other status is reachable and done is the next', () => {
  const steps = defaultSteps(progress, [todo, progress, done]);
  assert.deepEqual(steps.map((step) => step.to.id), ['ready', 'done']);
  assert.equal(steps.every((step) => step.blockedReason === undefined), true);
  assert.equal(primaryStep(progress, steps)?.to.id, 'done');
});

test('the status menu is a menu the keyboard can walk and leave', () => {
  const button = source('StatusButton.svelte');
  assert.match(button, /aria-haspopup="menu"/);
  assert.match(button, /aria-expanded=\{isOpen \|\| isSheetOpen\}/);
  assert.match(button, /role="menu"/);
  assert.match(button, /'menuitemradio' : 'menuitem'/, 'without a workflow the statuses are radio items');
  assert.match(button, /rovingIndex\(event\.key/, 'the arrows walk the steps');
  assert.match(button, /event\.key === 'ArrowDown'/, 'the arrow opens it from the button');
  assert.match(button, /aria-disabled=\{item\.blockedReason !== undefined/, 'a refused step stays focusable and says so');
  assert.match(button, /aria-describedby=\{item\.blockedReason/, 'a refused step points at its reason');
  assert.match(button, /focusReturn\(/, 'closing gives focus back to the button');
  assert.match(button, /isCompactWidth\(\)/, 'a phone gets the sheet');
});

// --- FoldingSection -------------------------------------------------------------------------

test('a list folds to its first rows, or to its newest', () => {
  assert.deepEqual(foldWindow(7, 5), { start: 0, end: 5, hidden: 2 });
  assert.deepEqual(foldWindow(13, 3, 'end'), { start: 10, end: 13, hidden: 10 });
});

test('a fold that would hide one row hides nothing, and an open fold shows all', () => {
  assert.deepEqual(foldWindow(6, 5), { start: 0, end: 6, hidden: 0 });
  assert.deepEqual(foldWindow(20, 5, 'start', true), { start: 0, end: 20, hidden: 0 });
  assert.deepEqual(foldWindow(0, 5), { start: 0, end: 0, hidden: 0 });
});

test('a folded section says what it hides and opens when focus arrives inside it', () => {
  const section = source('FoldingSection.svelte');
  assert.match(section, /aria-expanded=\{isOpen\}/);
  assert.match(section, /aria-controls=\{bodyId\}/);
  assert.match(section, /onfocusin=/, 'focus inside the clamp opens it (2.4.11)');
  assert.match(section, /tabindex="-1"/, 'a jump mark can move focus to it');
});

// --- the swipe ------------------------------------------------------------------------------

test('a gesture more upright than level is the page’s scroll, not the row’s swipe', () => {
  assert.equal(swipeIntent(3, 2), undefined);
  assert.equal(swipeIntent(4, 30), 'scroll');
  assert.equal(swipeIntent(30, 4), 'swipe');
});

test('letting go acts past the threshold, on either side', () => {
  assert.equal(swipeOutcome(SWIPE_MIN - 1, 100), 'none');
  assert.equal(swipeOutcome(SWIPE_MIN, 100), 'right');
  assert.equal(swipeOutcome(-130, 400), 'left');
  assert.equal(swipeOutcome(110, 400), 'none', 'a wide row asks for its share, not the minimum');
});

test('the row follows the thumb, slower past the threshold, and not at all towards a side with no act', () => {
  assert.equal(swipeOffset(40, 300, { right: true, left: true }), 40);
  assert.ok(swipeOffset(200, 300, { right: true, left: true }) < 200);
  assert.equal(swipeOffset(-80, 300, { right: true, left: false }), 0);
});

test('every swipe has a way without the gesture (2.5.7)', () => {
  const table = source('DataTable.svelte');
  assert.match(table, /const menuOf = /);
  assert.match(table, /__swipe-end/, 'the swipe’s act heads the row’s menu');
  assert.match(table, /event\.pointerType === 'mouse'/, 'a mouse is never taken for a swipe');
});

// --- density --------------------------------------------------------------------------------

test('the density is kept on the device and survives a store that throws', () => {
  const kept = new Map();
  const store = { getItem: (key) => kept.get(key) ?? null, setItem: (key, value) => kept.set(key, value) };
  assert.equal(readDensity(store, 'queue'), 'comfortable');
  writeDensity(store, 'queue', 'compact');
  assert.equal(readDensity(store, 'queue'), 'compact');
  assert.equal(readDensity(store, 'board'), 'comfortable', 'one list’s choice is not another’s');
  const broken = { getItem: () => { throw new Error('quota'); }, setItem: () => { throw new Error('quota'); } };
  assert.equal(readDensity(broken, 'queue'), 'comfortable');
  assert.doesNotThrow(() => writeDensity(broken, 'queue', 'compact'));
  assert.equal(readDensity(undefined, 'queue'), 'comfortable');
});

// --- QueryLine ------------------------------------------------------------------------------

const catalogue = {
  fields: [
    { key: 'priority', label: 'Priority', detail: 'choice', example: 'priority: P1, P2' },
    { key: 'cf.prioritised', label: 'Prioritised by' },
    { key: 'type', label: 'Type' },
  ],
  values: (field) => (field === 'priority' ? [{ value: 'P1', label: 'P1 Critical', countLabel: '2 open' }, { value: 'P2', label: 'P2 High' }] : []),
};

test('fields are suggested by key or name, those that start with the word first', () => {
  const found = suggest(catalogue, 'prio');
  assert.deepEqual(found.map((item) => item.label), ['Priority', 'Prioritised by']);
  assert.equal(found[0].insert, 'priority: ');
  assert.deepEqual(suggest(catalogue, ''), [], 'nothing typed, nothing suggested');
  assert.deepEqual(suggest(catalogue, 'ype').map((item) => item.label), ['Type'], 'inside a word too');
});

test('after a field and its colon, its values with their counts; a comma keeps the values before it', () => {
  const values = suggest(catalogue, 'priority: P');
  assert.deepEqual(values.map((item) => [item.label, item.hint]), [['P1 Critical', '2 open'], ['P2 High', undefined]]);
  assert.equal(suggest(catalogue, 'priority: P1, P2')[0].insert, 'priority: P1, P2 ');
  assert.equal(suggest(catalogue, '-priority: P1')[0].insert, '-priority: P1 ', 'a negation is kept');
  assert.deepEqual(suggest(catalogue, 'nofield: x'), []);
});

test('a pasted run of spaces after a colon costs its length, not its square', () => {
  const pasted = `priority:${' '.repeat(200_000)}\nP\nx`;
  const start = performance.now();
  assert.deepEqual(suggest(catalogue, pasted), []);
  assert.ok(performance.now() - start < 2000, 'the pattern retried every split of the spaces');
  assert.equal(suggest(catalogue, 'priority:   ').length, 2, 'spaces after the colon still offer the values');
  assert.equal(suggest(catalogue, 'priority:\n P1')[0].insert, 'priority: P1 ', 'a line break before the value is space');
});

test('+ Condition offers every field, and inside an open group the part after OR is completed', () => {
  assert.equal(suggest(catalogue, '', { isBrowsing: true }).length, catalogue.fields.length);
  assert.equal(suggest(catalogue, '(').length, catalogue.fields.length, 'a group just opened offers every field');
  assert.equal(suggest(catalogue, '(type: x OR prio')[0].insert, '(type: x OR priority: ', 'what stands before is kept');
  assert.equal(suggest(catalogue, '(priority: P1')[0].insert, '(priority: P1 ');
});

test('the line as text is every token’s spelling, then what is being typed', () => {
  const language = { parse: () => ({ tokens: [], rest: '' }), format: (token) => (token.kind === 'text' ? token.text : `${token.field}: x`) };
  const tokens = [{ kind: 'chip', id: '1', field: 'type', fieldLabel: 'Type', values: [] }, { kind: 'text', id: '2', text: '(a OR b)' }];
  assert.equal(lineText(language, tokens, ' passkey '), 'type: x (a OR b) passkey');
});

test('the query line is a combobox whose chips can be edited and removed by keyboard', () => {
  const line = source('QueryLine.svelte');
  assert.match(line, /role="combobox"/);
  assert.match(line, /aria-activedescendant=/);
  assert.match(line, /role="listbox"/);
  assert.match(line, /role="option"/);
  assert.match(line, /event\.key === 'Backspace' && text === ''/, 'Backspace takes the last chip back');
  assert.match(line, /aria-label=\{editLabel\(words\)\}/);
  assert.match(line, /label=\{removeLabel\(words\)\}/);
  assert.match(line, /aria-describedby=\{token\.problem/, 'an error is said at its chip');
});

// --- the rest of the contract ---------------------------------------------------------------

test('the composer keeps reply and note apart as two tabs and says who sees it', () => {
  const composer = source('Composer.svelte');
  assert.match(composer, /role="tablist"/);
  assert.match(composer, /aria-selected=\{!isNote\}/);
  assert.match(composer, /aria-selected=\{isNote\}/);
  assert.match(composer, /role=\{modes \? 'tabpanel'/);
  assert.match(composer, /aria-describedby=\{who/, 'the field points at who will see it');
  assert.match(composer, /event\.ctrlKey \|\| event\.metaKey/, 'Ctrl+Enter sends');
  assert.match(composer, /noteSendLabel/, 'the button says what it does');
});

test('the rail’s sections fold as twists and an empty one is one line that adds', () => {
  const rail = source('PropertyRail.svelte');
  assert.match(rail, /<aside class="rail" aria-label=\{label\}>/);
  assert.match(rail, /aria-expanded=\{isOpen\(item\)\}/);
  assert.match(rail, /onclick=\{\(\) => onadd\?\.\(item\.id\)\}/);
  assert.match(rail, /<Sheet /, 'a phone gets the sheet');
});

test('jump marks say which section is in view', () => {
  const marks = source('JumpMarks.svelte');
  assert.match(marks, /<nav class="marks" aria-label=\{label\}>/);
  assert.match(marks, /aria-current=\{mark\.id === current \? 'location'/);
});

test('a step bar names the step the object stands on, and says a done step in words and glyph', () => {
  const bar = source('StepBar.svelte');
  assert.match(bar, /<ol class="steps" aria-label=\{label\}/);
  assert.match(bar, /aria-current=\{state === 'current' \? 'step'/);
  assert.match(bar, /\{#if state === 'done'\}<Icon name="check"/, 'done carries a glyph, not colour alone');
  assert.match(bar, /<VisuallyHidden>\{doneLabel\}<\/VisuallyHidden>/);
});

test('the head’s title is edited in place with Enter and Escape, and the edit has a name', () => {
  const head = source('ObjectHead.svelte');
  assert.match(head, /aria-label=\{titleLabel\}/);
  assert.match(head, /event\.key === 'Enter'\) finish\(true\)/);
  assert.match(head, /event\.key === 'Escape'/);
  assert.match(head, /label=\{editTitleLabel\}/, 'the keyboard’s way in is a named button');
});

// --- BucketColumn (board, build-plan §5) ------------------------------------------------------

test('a folded column unfolds by a button, and a refused one says why in words (rule 3)', () => {
  const column = source('BucketColumn.svelte');
  assert.match(column, /<button type="button" class="unfold" aria-expanded="false"/, 'the folded head is a button');
  assert.match(column, /\{#if refusal\}<p class="refusal"/, 'the reason is written, not only dimmed');
  assert.match(column, /data-refused=\{refusal \? '' : undefined\}/, 'the dimming follows the reason');
  assert.match(column, /\{#if foot\}<div class="foot">/, 'the foot is capture in place (C-4)');
});

// --- TaskRow in a calm table (WB-4) -------------------------------------------------------------

// N12, N14: from a tablet's width a ruled row's title is one line, also in a list narrowed by a
// panel beside it - the viewport decides, not the list's own width, so a row stays compact.
test('a ruled row keeps its title to one line from a tablet\'s width (N12, N14)', () => {
  const row = source('TaskRow.svelte');
  assert.match(row, /@media \(width >= 600px\) \{\s*\.task-row\[data-ruled\] \.title \{ display: block; white-space: nowrap; text-overflow: ellipsis; min-inline-size: 0; \}/);
});

test('a ruled task row is a line of one region, not a card, and keeps its whole title as a tooltip', () => {
  const row = source('TaskRow.svelte');
  assert.match(row, /data-ruled=\{isRuled \? '' : undefined\}/);
  assert.match(row, /\.task-row\[data-ruled\] > :global\(\.row\) \{\s*border-radius: 0;\s*border-color: transparent transparent var\(--border-subtle\);/, 'ruled by a hairline, square');
  assert.match(row, /title=\{isOneLine \|\| isRuled \? title : undefined\}/, 'a title cut to one line keeps its words on hover');
});

test('a view switcher\'s targets reach layout.target on a phone or a coarse pointer (§7)', () => {
  const css = source('ViewSwitcher.svelte');
  assert.match(css, /@media \(pointer: coarse\), \(width < 600px\) \{\s*\.view \{ min-block-size: var\(--layout-target\); min-inline-size: var\(--layout-target\); \}/u);
});

// --- InsertList ------------------------------------------------------------------------------

import { listKey, markedParts, matchingGroups } from '../src/insert.ts';

test('the insert list moves with the arrows, wraps, jumps by letter, takes with Enter and closes with Escape', () => {
  const labels = ['Embed a view', 'Mention an entry', 'Callout', 'Collapsible'];
  assert.equal(listKey('ArrowDown', 0, labels), 1);
  assert.equal(listKey('ArrowUp', 0, labels), 3);
  assert.equal(listKey('End', 0, labels), 3);
  assert.equal(listKey('c', 0, labels), 2);
  assert.equal(listKey('c', 2, labels), 3);
  assert.equal(listKey('Enter', 1, labels), 'take');
  assert.equal(listKey('Enter', 0, []), null);
  assert.equal(listKey('Escape', 1, labels), 'close');
  assert.equal(listKey('Tab', 1, labels), null);
});

test('the words typed after "/" narrow the insert list and are marked where they stand', () => {
  const groups = [
    { label: 'Hubtask', choices: [{ id: 'view', label: 'Ansicht einbetten', detail: 'Eine gespeicherte Ansicht' }, { id: 'emoji', label: 'Emoji' }] },
    { label: 'Blöcke', choices: [{ id: 'toc', label: 'Inhalt' }, { id: 'details', label: 'Aufklappbereich', detail: 'Mit einer Zusammenfassung' }] },
  ];
  assert.deepEqual(matchingGroups(groups, '').map((group) => group.choices.length), [2, 2]);
  assert.deepEqual(matchingGroups(groups, 'EIN').flatMap((group) => group.choices.map((choice) => choice.id)), ['view', 'details']);
  assert.deepEqual(matchingGroups(groups, 'emo').map((group) => group.label), ['Hubtask'], 'a group without a match is left out');
  assert.deepEqual(matchingGroups(groups, 'xyz'), []);
  assert.deepEqual(markedParts('Ansicht einbetten', 'ein'), [
    { text: 'Ansicht ', isMatch: false }, { text: 'ein', isMatch: true }, { text: 'betten', isMatch: false },
  ]);
  assert.deepEqual(markedParts('Emoji', ''), [{ text: 'Emoji', isMatch: false }]);
});

test('the insert list is a listbox of options read through aria-activedescendant, with its keys line', () => {
  const list = source('InsertList.svelte');
  assert.match(list, /role="listbox"/);
  assert.match(list, /role="option"/);
  assert.match(list, /aria-activedescendant=/);
  assert.match(list, /<Kbd>↑↓<\/Kbd>/);
});
