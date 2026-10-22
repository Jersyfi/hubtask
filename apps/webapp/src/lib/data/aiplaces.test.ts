// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where each AI question is asked and each proposal shown on an entry (UC-AI-04 checks 6, 8, 9).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Suggestion } from '@hubtask/sync-engine';

import { askOf, isBudgetNearlySpent, offersAt, placeOf, type PlaceContext } from './aiplaces.ts';

const proposal = (over: Partial<Suggestion>): Suggestion =>
  ({
    id: 's1', target_type: 'WORK_ITEM', target_id: 'i1', kind: 'FIELDS', status: 'PROPOSED',
    payload: {}, source: 'AI', model: 'm', prompt_id: 'suggest-item-fields', prompt_version: 'v3',
    produced_at: '2030-01-01T00:00:00Z', created_at: '2030-01-01T00:00:00Z', version: 1,
    ...over,
  }) as Suggestion;

test('a proposal stands under the subject it is about', () => {
  assert.equal(placeOf(proposal({ payload: { title: 'Book the tiler' } })), 'title');
  assert.equal(placeOf(proposal({ payload: { notes: 'Ask about Thursday' } })), 'notes');
  assert.equal(placeOf(proposal({ payload: { due_date: '2030-01-02' } })), 'due');
  assert.equal(placeOf(proposal({ prompt_id: 'summarize', payload: { notes: 'Short' } })), 'notes');
  assert.equal(placeOf(proposal({ prompt_id: 'summarize-thread', payload: { notes: 'Agreed' } })), 'comments');
  assert.equal(placeOf(proposal({ payload: { label_ids: ['l1'] } })), 'labels');
  assert.equal(placeOf(proposal({ payload: { bucket_id: 'b1' } })), 'labels');
  assert.equal(placeOf(proposal({ kind: 'DECOMPOSITION', payload: { children: [{ title: 'A', type: 'WORK_PACKAGE' }] } })), 'steps');
  assert.equal(placeOf(proposal({ kind: 'DUPLICATES', payload: { duplicates: [] } })), 'title');
  // Several fields at once - from a rule, an agent, an older ask - go where the entry begins.
  assert.equal(placeOf(proposal({ payload: { title: 'T', due_date: '2030-01-02' } })), 'title');
  // A kind this version has never met is not lost: it is shown at the title.
  assert.equal(placeOf(proposal({ kind: 'NEW_KIND' as Suggestion['kind'] })), 'title');
});

test('a standing proposal answers one question, so its ✦ shows it rather than asking again', () => {
  assert.equal(askOf(proposal({ payload: { title: 'T' } })), 'suggest-title');
  assert.equal(askOf(proposal({ payload: { due_date: '2030-01-02' } })), 'suggest-due');
  assert.equal(askOf(proposal({ prompt_id: 'summarize', payload: { notes: 'N' } })), 'summarize');
  assert.equal(askOf(proposal({ prompt_id: 'summarize-thread', payload: { notes: 'N' } })), 'summarize-thread');
  assert.equal(askOf(proposal({ payload: { title: 'T', notes: 'N' } })), undefined);
});

const writer = (over: Partial<PlaceContext> = {}): PlaceContext => ({
  features: { ai_suggestions: true, semantic_search: true },
  item: { archived_at: null },
  mayChange: true,
  hasComments: true,
  choices: true,
  ...over,
});

const asks = (offers: readonly { ask: string }[]) => offers.map((offer) => offer.ask);

test('each place offers its own questions to somebody who may change the entry', () => {
  assert.deepEqual(asks(offersAt('title', writer())), ['suggest-title', 'duplicates', 'translate']);
  assert.deepEqual(asks(offersAt('notes', writer())), ['suggest-notes', 'summarize']);
  assert.deepEqual(asks(offersAt('due', writer())), ['suggest-due']);
  assert.deepEqual(asks(offersAt('labels', writer())), ['classify']);
  assert.deepEqual(asks(offersAt('steps', writer())), ['decompose']);
  assert.deepEqual(asks(offersAt('comments', writer())), ['summarize-thread']);
});

test('a reader is offered the two reading aids and nothing that writes (check 8, P-05)', () => {
  const reader = writer({ mayChange: false });
  assert.deepEqual(asks(offersAt('title', reader)), ['translate']);
  assert.deepEqual(asks(offersAt('comments', reader)), ['summarize-thread']);
  for (const place of ['notes', 'due', 'labels', 'steps'] as const) assert.deepEqual(offersAt(place, reader), []);
});

test('without AI there is no ✦ anywhere, and no discussion means no summary of one', () => {
  for (const place of ['title', 'notes', 'due', 'labels', 'steps', 'comments'] as const) {
    assert.deepEqual(offersAt(place, writer({ features: { ai_suggestions: false } })), []);
  }
  assert.deepEqual(offersAt('comments', writer({ hasComments: false })), []);
});

test('what cannot be asked now stands with its reason, inside the menu', () => {
  const archived = offersAt('notes', writer({ item: { archived_at: '2030-01-01T00:00:00Z' } }));
  assert.ok(archived.every((offer) => offer.reason?.code === 'app.entries.archived'));
  // A reading aid is a read: an archived entry is still read in another language.
  assert.equal(offersAt('title', writer({ item: { archived_at: '2030-01-01T00:00:00Z' } }))[2]?.reason, undefined);
  assert.equal(offersAt('title', writer({ features: { ai_suggestions: true } }))[1]?.reason?.code, 'app.suggestions.no_semantic_search');
  assert.equal(offersAt('labels', writer({ choices: false }))[0]?.reason?.code, 'app.suggestions.no_choices');
  assert.equal(offersAt('labels', writer({ choices: 'reading' }))[0]?.reason?.code, 'app.suggestions.choices_reading');
});

test('a spent budget is said on every item before asking, with when it renews (check 9)', () => {
  const offers = offersAt('title', writer({ budget: { state: 'SPENT', renews_at: '2030-01-02T00:00:00Z' } }));
  for (const offer of offers) {
    assert.equal(offer.reason?.code, 'ai.unavailable');
    assert.equal(offer.reason?.params?.reason, 'budget_spent');
    assert.equal(offer.reason?.params?.renews_at, '2030-01-02T00:00:00Z');
  }
  assert.equal(isBudgetNearlySpent({ state: 'NEARLY_SPENT' }), true);
  assert.equal(isBudgetNearlySpent({ state: 'OK' }), false);
  assert.equal(isBudgetNearlySpent(undefined), false);
});

test('a question whose answer already stands shows it instead of asking again', () => {
  const offers = offersAt('comments', writer({ mayChange: false, standing: { 'summarize-thread': 's7' } }));
  assert.deepEqual(offers, [{ ask: 'summarize-thread', standing: 's7' }]);
});
