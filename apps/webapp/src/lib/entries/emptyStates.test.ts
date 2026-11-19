// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A collection's list when it shows nothing (user-experience §4, WB-6): empty because of what is
// seen says so and leads back to everything; truly empty offers a template as the main way, the
// first entry beside it. Read from the source, as the ladder test does.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const source = readFileSync(new URL('./EntryList.svelte', import.meta.url), 'utf8');

test('a list empty by its filter says so and offers everything again, not the empty collection\'s invitation', () => {
  assert.match(source, /<EmptyState kind="filtered" title=\{t\('app\.entries\.none_matching'\)\}/u);
  assert.match(source, /onclick=\{onshowall\}>\{t\('app\.entries\.show_all'\)\}/u);
  assert.ok(source.indexOf('kind="filtered"') < source.indexOf('kind="unused" title={t(root'), 'the filtered case is not decided first');
});

// WB-6: what is truly empty shows a sentence, a template as the main way and a side way.
test('an empty collection offers a template as the main way and its first entry beside it (WB-6)', () => {
  assert.match(source, /\{#if isTemplateFirst\}<Button tone="primary" icon="copy" onclick=\{ontemplate\}>\{t\('app\.templates\.from'\)\}<\/Button>\{\/if\}/u);
  assert.match(source, /<Button tone=\{isTemplateFirst \? 'secondary' : 'primary'\} icon="plus" data-opener="add-entry"/u);
});

// E-4: the side way stands where no template does - the import beside the first entry - and the
// templates along the path stand as cards under the ways, as the scene empty draws them.
test('an empty collection without a template offers the import as its side way and the templates as cards (E-4)', () => {
  assert.match(source, /\{#if !isTemplateFirst && onimport\}<Button tone="secondary" icon="cloud-upload" onclick=\{onimport\}>\{t\('app\.import\.open'\)\}<\/Button>\{\/if\}/u);
  assert.match(source, /\{#if isTemplateFirst && starts && starts\.length > 0\}<TemplateCards templates=\{starts\} \/>\{\/if\}/u);
});
