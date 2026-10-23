// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Suggestion } from '@hubtask/sync-engine';

import { collectionOf, dayOf, entriesOf, entriesProposalSince, offersMakeEntries } from './entries.ts';

const ON = { aiOn: true, intoCollection: true };

test('Make entries is offered for pasted text of more than one line, with AI on and a collection to go to', () => {
  assert.equal(offersMakeEntries('Pancakes\n200 g flour\n2 eggs', ON), true);
  assert.equal(offersMakeEntries('flour\r\n\r\neggs', ON), true, 'blank lines and CRLF do not count against it');
  assert.equal(offersMakeEntries('Buy flour tomorrow', ON), false, 'one line is captured as one entry');
  assert.equal(offersMakeEntries('flour\n   \n', ON), false, 'a line and a blank one are one line');
  assert.equal(offersMakeEntries('flour\neggs', { aiOn: false, intoCollection: true }), false, 'without AI nothing is offered');
  assert.equal(offersMakeEntries('flour\neggs', { aiOn: true, intoCollection: false }), false, 'the jumble decides nothing');
});

const proposal = (overrides: Partial<Suggestion> = {}): Suggestion =>
  ({
    id: 'p1',
    kind: 'ENTRIES',
    status: 'PROPOSED',
    created_at: '2030-01-01T00:01:00Z',
    target_type: 'ACCOUNT',
    target_id: 'me',
    payload: {
      collection_id: 'groceries',
      type: 'TASK',
      entries: [
        { key: '1', title: 'Flour', amount: '200 g' },
        { key: '2', title: 'Eggs', due_date: '2026-10-24', notes: '' },
        { key: '3', title: '   ' },
        { title: 'No key' },
      ],
    },
    ...overrides,
  }) as unknown as Suggestion;

test('a proposal reads as its entries, in its order, with the fields it carries', () => {
  assert.deepEqual(entriesOf(proposal()), [
    { key: '1', title: 'Flour', amount: '200 g' },
    { key: '2', title: 'Eggs', dueDate: '2026-10-24' },
  ]);
  assert.equal(collectionOf(proposal()), 'groceries');
  assert.deepEqual(entriesOf(proposal({ kind: 'RULE' })), [], 'another kind holds no entries');
});

test('a proposed day is drawn as that day wherever the reader is', () => {
  assert.equal(dayOf('2026-10-24', 'en'), 'Oct 24, 2026');
  assert.equal(dayOf('not a date', 'en'), 'not a date');
});

test('the proposal an ask produced is the newest of entries for that collection since the ask', () => {
  const asked = '2030-01-01T00:00:00Z';
  const older = proposal({ id: 'older', created_at: '2029-12-31T23:59:00Z' });
  const elsewhere = proposal({ id: 'elsewhere', payload: { collection_id: 'kitchen', entries: [] } });
  const rule = proposal({ id: 'rule', kind: 'RULE' });
  const taken = proposal({ id: 'taken', status: 'ACCEPTED', created_at: '2030-01-01T00:03:00Z' });
  const mine = proposal({ id: 'mine', created_at: '2030-01-01T00:02:00Z' });
  assert.equal(entriesProposalSince([older, elsewhere, rule, taken, proposal(), mine], asked, 'groceries')?.id, 'mine');
  assert.equal(entriesProposalSince([older, rule], asked, 'groceries'), undefined);
});
