// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// ChoiceCards (design-system.md §4, wave 1), checked without a browser: the radio-group contract
// it promises, as a reading of its source - the bargain `conventions.test.js` makes.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const sourceDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
const source = fs.readFileSync(path.join(sourceDir, 'ChoiceCards.svelte'), 'utf8');
const markup = source.slice(source.lastIndexOf('</script>'), source.indexOf('<style>'));
const style = source.slice(source.indexOf('<style>'));

test('the cards are one named radio group of native radios', () => {
  assert.match(markup, /<fieldset[^>]*role="radiogroup"/, 'a reader hears a radio group');
  assert.match(markup, /<legend class="legend">\{label\}<\/legend>/, 'the question names the group');
  assert.match(markup, /type="radio"/);
  assert.match(markup, /\{name\}/, 'every radio carries the one name, so the arrow keys walk the group');
  assert.match(source, /name = `choice-\$\{instanceId\}`/, 'a generated name, so two groups never merge');
  assert.match(markup, /bind:group=\{value\}/);
});

test('the whole card is pressed: the radio sits inside the card’s label', () => {
  const card = markup.slice(markup.indexOf('<label class="card"'), markup.indexOf('</label>'));
  assert.ok(card.length > 0, 'a card is a label');
  assert.match(card, /type="radio"/, 'the radio is inside the card it chooses');
  assert.match(card, /class="title">\{option\.label\}/);
});

test('the line and a reason describe their radio', () => {
  assert.match(markup, /aria-describedby=\{described \|\| undefined\}/);
  assert.match(markup, /id=\{`\$\{optionId\}-line`\}/);
  assert.match(markup, /id=\{`\$\{optionId\}-reason`\}/);
  assert.match(markup, /disabled=\{off\}/, 'a card that cannot be chosen is skipped by the arrow keys, as in Radio');
});

test('the chosen card is marked by its edge, its surface and the dot - not by colour alone', () => {
  assert.match(style, /\.card:has\(\.native:checked\) \{\s*border-color: var\(--accent-primary\);\s*background: var\(--accent-primary-subtle\);/);
  assert.match(style, /\.native:checked \+ \.dot \{[^}]*box-shadow: inset/, 'the dot fills: a shape, not a hue');
  assert.match(style, /\.card:has\(\.native:focus-visible\) \{\s*outline: var\(--bw-ring\) solid var\(--focus-ring\)/, 'the focus ring is drawn on the card');
});

test('nothing is chosen unless the caller chose it', () => {
  assert.match(source, /value = \$bindable\(''\)/);
  assert.doesNotMatch(markup, /\bchecked\b/, 'no card is checked by the component');
});

// --- the tiles of a gallery that is a choice -------------------------------------------------------

test('a tick is an empty ring until chosen, then filled: a shape, not a hue', () => {
  assert.match(markup, /\{#if mark === 'tick'\}\s*<span class="tick" aria-hidden="true"><Icon name="check"/);
  const ring = style.match(/\n {2}\.tick \{[^}]*\}/)?.[0] ?? '';
  assert.match(ring, /border: var\(--bw-hairline\) solid var\(--border-default\)/, 'not chosen, the tick is a ring');
  assert.match(ring, /color: transparent/, 'not chosen, the ring is empty');
  assert.match(style, /\.native:checked \+ \.tick \{[^}]*background: var\(--accent-primary\);[^}]*color: var\(--text-inverse\)/, 'chosen, it fills with the tick');
  assert.match(style, /\[data-mark='tick'\] \.control \{[^}]*position: absolute;[^}]*inset-inline-end:/, 'the tick sits in the top end corner');
});

test('the families are fieldsets with legends inside the one radio group, and every radio shares one name', () => {
  assert.equal(markup.match(/role="radiogroup"/g)?.length, 1, 'one radio group, however many families');
  const group = markup.slice(markup.indexOf('{#each groups'), markup.indexOf('{/each}', markup.indexOf('{#each groups')));
  assert.match(group, /<fieldset class="group">\s*<legend class="heading">\{group\.label\}<\/legend>/);
  assert.match(group, /\{@render cards\(group\.options\)\}/, 'a family draws the same cards');
  assert.equal(markup.match(/type="radio"/g)?.length, 1, 'one radio in the markup, drawn for every card of every family');
  assert.match(markup, /type="radio"[^>]*\{name\}/, 'and it carries the one name, so the arrows walk across the families');
});

test('the grid sits on an element inside the fieldset, and a tile is never narrower than a gallery card', () => {
  assert.match(style, /\[data-layout='grid'\] \.cards \{[^}]*display: grid;[^}]*minmax\(min\(100%, var\(--layout-gallery-card\)\), 1fr\)/);
  assert.doesNotMatch(style, /\.choices\[data-layout='grid'\] \{[^}]*display: grid/, 'a grid on the fieldset itself has differed between engines');
});

test('the start from nothing is dashed until it is chosen', () => {
  assert.match(markup, /data-blank=\{option\.isBlank \? '' : undefined\}/);
  assert.match(style, /\.card\[data-blank\]:not\(:has\(\.native:checked\)\) \{ border-style: dashed; \}/);
});

test('a summary is cut after two lines; it and the chips describe the radio', () => {
  assert.match(style, /\.summary \{[^}]*line-clamp: 2;/);
  assert.match(markup, /option\.summary \? `\$\{optionId\}-summary`/);
  assert.match(markup, /hasChips \? `\$\{optionId\}-chips`/);
});

test('a caller can type its families: ChoiceCardGroup is exported with ChoiceCards', () => {
  const index = fs.readFileSync(path.join(sourceDir, 'index.ts'), 'utf8');
  assert.match(index, /export \{ default as ChoiceCards, [^}]*\btype ChoiceCardGroup\b[^}]*\} from '\.\/ChoiceCards\.svelte';/);
});

test('the tile keeps the create-hub draft’s measures, from the tokens', () => {
  assert.match(style, /min-block-size: calc\(var\(--sp-1000\) \+ var\(--sp-150\)\);/, 'a tile is as high as the draft’s from 600px');
  assert.match(style, /\.chip \{\s*padding: var\(--bw-hairline\) var\(--sp-100\);/, 'the chip has the draft’s block padding');
  assert.match(style, /\.tick :global\(\.icon\) \{ inline-size: var\(--fs-075\); block-size: var\(--fs-075\); \}/, 'the tick’s glyph is the caption’s size');
  assert.match(style, /\.mark :global\(\.icon\) \{ inline-size: var\(--fs-200\); block-size: var\(--fs-200\); \}/, 'the mark’s glyph is the body’s size');
});

test('a hidden legend still names the group: drawn nowhere, never display none', () => {
  assert.match(source, /isLabelHidden\?: boolean/);
  assert.match(markup, /data-label-hidden=\{isLabelHidden \? '' : undefined\}/);
  const rule = style.slice(style.indexOf('.choices[data-label-hidden] > .legend'));
  assert.match(rule.slice(0, rule.indexOf('}')), /clip-path: inset\(50%\)/, 'the legend is clipped, not removed');
  assert.doesNotMatch(rule.slice(0, rule.indexOf('}')), /display: none/);
});

test('a tile measures by its border, so its minimum height is the draft’s in any application', () => {
  const card = style.slice(style.indexOf('  .card {'));
  assert.match(card.slice(0, card.indexOf('}')), /box-sizing: border-box/);
});

test('the tick measures by its edge too, so its ring is the draft’s in any application', () => {
  const tick = style.slice(style.indexOf('  .tick {'));
  assert.match(tick.slice(0, tick.indexOf('}')), /box-sizing: border-box/);
});

test('a chosen tile under the pointer keeps its brand edge: the hover darkens only an unchosen one', () => {
  assert.match(style, /\.choices\[data-mark='tick'\] \.card:not\(:has\(\.native:checked\)\):hover \{ border-color: var\(--border-strong\); \}/);
  assert.doesNotMatch(style, /\.choices\[data-mark='tick'\] \.card:hover \{/);
});
