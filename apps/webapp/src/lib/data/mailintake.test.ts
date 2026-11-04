// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DEFAULT_MAIL_SETTINGS, linesOf, mailboxColumnFigure, mailboxFigure, mailboxStateOf, routesTo, suggestedName } from './mailintake.svelte.ts';

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

test('a plus address names the collection where the provider delivers one', async () => {
  const { mailboxRouteTo } = await import('./mailintake.svelte.ts');
  const box = { address: 'familie@gmail.com', plus_addresses: true } as never;
  assert.equal(mailboxRouteTo('einkauf', box), 'familie+einkauf@gmail.com');
  assert.equal(mailboxRouteTo('einkauf', { address: 'familie@gmx.de', plus_addresses: false } as never), 'familie@gmx.de');
});

test('a mailbox reads, has a failed read, fails for an hour, or is off - and only the hour is counted in the column', () => {
  const box = (enabled: boolean, failing: boolean, failure?: string) => ({ enabled, failing, ...(failure ? { failure } : {}) });
  assert.equal(mailboxStateOf(box(true, false)), 'reading');
  assert.equal(mailboxStateOf(box(true, false, 'mail.mailbox_unreachable')), 'failed_once');
  assert.equal(mailboxStateOf(box(true, true, 'mail.mailbox_refused')), 'failing');
  assert.equal(mailboxStateOf(box(false, true, 'mail.intake_off')), 'off', 'a mailbox switched off is not read, so it is not failing');
  const all = [box(true, false), box(true, false, 'mail.mailbox_unreachable'), box(true, true, 'mail.mailbox_refused'), box(false, false)];
  assert.deepEqual(mailboxFigure(all), { reading: 2, failing: 1, off: 1 });
  const t = (code: string, params: Record<string, string> = {}) => [code, ...Object.values(params)].join(' ');
  assert.equal(mailboxColumnFigure(all, t), 'app.mail.column_failing 1');
  assert.equal(mailboxColumnFigure([box(true, false, 'mail.mailbox_unreachable')], t), undefined, 'one failed read is not yet an outage');
});
