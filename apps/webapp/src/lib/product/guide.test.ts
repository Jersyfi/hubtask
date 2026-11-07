// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from '../i18n/source.ts';
import { PRODUCTS } from '../navigation.ts';
import { guideOf, hasGuide, whoToAsk } from './guide.ts';

// user-experience.md §5.5: every product's first use links its guide where the workspace names its
// documentation, and none where it does not.
test('every product links the documentation address itself, no path below it', () => {
  for (const row of PRODUCTS) {
    if (row.module === 'WORKFLOWS') continue;
    assert.ok(hasGuide(row.module), `${row.module} has no guide path`);
  }
  assert.equal(guideOf('https://docs.example.org/', 'PLANNING'), 'https://docs.example.org/');
  assert.equal(guideOf(undefined, 'PLANNING'), undefined, 'no address, no link');
  assert.equal(guideOf('', 'KNOWLEDGE'), undefined);
});

// §5.5: a reader who may not create is told whom to ask, by name where the place names somebody.
test('whom to ask names the first administrator, else any', () => {
  const t = (code: string, params?: Record<string, string>) => `${code}${params ? JSON.stringify(params) : ''}`;
  assert.equal(whoToAsk([{ name: 'Anna Keller' }, { name: 'Ben Ott' }], t), 'app.product_empty.ask_named{"name":"Anna Keller"}');
  assert.equal(whoToAsk([], t), 'app.product_empty.ask_any');
  assert.equal(whoToAsk(undefined, t), 'app.product_empty.ask_any');
  for (const code of ['app.product_empty.ask_named', 'app.product_empty.ask_any', 'app.product_empty.guide']) {
    assert.ok(code in SOURCE, `${code} has no words`);
  }
});
