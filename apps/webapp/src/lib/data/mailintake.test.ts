// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DEFAULT_MAIL_SETTINGS, linesOf, routesTo, suggestedName } from './mailintake.svelte.ts';

test('a typed list is one entry per line or comma', () => {
  assert.deepEqual(linesOf(' a@b.de\n\n@lieferant.de, c@d.de ;'), ['a@b.de', '@lieferant.de', 'c@d.de']);
});

test('the name on the receiving domain is suggested from the workspace', () => {
  assert.equal(suggestedName('Familie Müller & Söhne'), 'familie-mueller-soehne');
  assert.equal(suggestedName('  Café Été  '), 'cafe-ete');
});

test('a collection is reached by its subject word everywhere, by an address where the domain makes one', () => {
  assert.deepEqual(routesTo('einkauf', DEFAULT_MAIL_SETTINGS), { subject: '#einkauf' });
  assert.deepEqual(routesTo('einkauf', { ...DEFAULT_MAIL_SETTINGS, name: 'familie', domain: 'in.hubtask.app' }), {
    subject: '#einkauf',
    address: 'familie+einkauf@in.hubtask.app',
  });
});
