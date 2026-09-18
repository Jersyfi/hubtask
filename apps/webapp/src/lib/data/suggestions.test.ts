// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a screen works out about a suggestion, and how an ask is followed - the half of F5-02 a
// test can assert without a browser. The follow runs against a real `SyncEngine` over a fake
// transport, because the thing worth asserting is the reads it makes and when it stops.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SyncEngine, TransportError } from '@hubtask/sync-engine';
import type { Response, Suggestion, Transport, WorkItem } from '@hubtask/sync-engine';

import {
  FOLLOW_ATTEMPTS,
  acceptCodeOf,
  arrivedSince,
  followArrival,
  headingCodeOf,
  isStale,
  isSummary,
  offersFor,
  operationOf,
  shapeOf,
  suggestionsPath,
} from './suggestions.ts';

const ITEM = '0192f000-0000-7000-8000-00000000aa01';

const item = (over: Partial<WorkItem> = {}): WorkItem =>
  ({
    id: ITEM,
    type: 'TASK',
    collection_id: 'c1',
    title: 'deck thursday',
    notes: null,
    due_at: null,
    due_date_only: false,
    updated_at: '2026-09-16T10:00:00Z',
    archived_at: null,
    version: 3,
    ...over,
  }) as WorkItem;

const suggestion = (over: Partial<Suggestion> = {}): Suggestion =>
  ({
    id: 's1',
    target_type: 'WORK_ITEM',
    target_id: ITEM,
    kind: 'FIELDS',
    status: 'PROPOSED',
    payload: { title: 'Prepare the quarterly deck' },
    source: 'AI',
    model: 'claude-opus-5',
    prompt_id: 'suggest-item-fields',
    prompt_version: 'v3',
    produced_at: '2026-09-16T11:00:00Z',
    created_at: '2026-09-16T11:00:01Z',
    version: 1,
    ...over,
  }) as Suggestion;

// --- what the menu offers -----------------------------------------------------------------------

test('AI off means no operation at all, not six disabled ones', () => {
  assert.deepEqual(offersFor({ ai_suggestions: false }, item(), true), []);
  assert.deepEqual(offersFor(undefined, item(), true), []);
});

test('with AI on every operation is offered, and the ones that cannot be asked say why', () => {
  const offers = offersFor({ ai_suggestions: true, semantic_search: false }, item(), false);
  assert.equal(offers.length, 6);
  const by = Object.fromEntries(offers.map((offer) => [offer.operation, offer.disabledCode]));
  assert.equal(by['suggest-fields'], undefined);
  assert.equal(by['summarize-thread'], 'app.suggestions.no_comments');
  assert.equal(by.duplicates, 'app.suggestions.no_semantic_search');
});

test('an archived entry is not written to, so nothing that accepts into it is asked', () => {
  const offers = offersFor({ ai_suggestions: true, semantic_search: true }, item({ archived_at: '2026-09-01T00:00:00Z' }), true);
  const by = Object.fromEntries(offers.map((offer) => [offer.operation, offer.disabledCode]));
  assert.equal(by['suggest-fields'], 'app.entries.archived');
  assert.equal(by.decompose, 'app.entries.archived');
  // Looking for look-alikes writes nothing, so an archived entry may still ask.
  assert.equal(by.duplicates, undefined);
});

// --- staleness ----------------------------------------------------------------------------------

test('a proposal made before the entry last changed is stale, and one made after is not', () => {
  assert.ok(isStale(suggestion({ produced_at: '2026-09-16T09:00:00Z' }), item()));
  assert.ok(!isStale(suggestion({ produced_at: '2026-09-16T11:00:00Z' }), item()));
});

test('an unreadable time marks nothing', () => {
  assert.ok(!isStale(suggestion({ produced_at: 'yesterday' }), item()));
});

// --- shapes -------------------------------------------------------------------------------------

test('a FIELDS payload of title, notes and a date is drawn beside what the entry holds', () => {
  const shape = shapeOf(
    suggestion({ payload: { title: 'Prepare the deck', notes: 'Slide 4 is provisional', due_date: '2026-09-18' } }),
    item({ notes: 'old notes' }),
  );
  assert.equal(shape.shape, 'fields');
  if (shape.shape !== 'fields') return;
  assert.deepEqual(
    shape.proposals.map((p) => [p.field, p.proposed, p.current]),
    [
      ['title', 'Prepare the deck', 'deck thursday'],
      ['notes', 'Slide 4 is provisional', 'old notes'],
      ['due_date', '2026-09-18', undefined],
    ],
  );
  assert.equal(headingCodeOf(shape), 'app.suggestions.kind_fields');
  assert.equal(acceptCodeOf(shape), 'app.suggestions.accept_fields');
});

test('notes and nothing else is a summary, and says so in its heading and its verb', () => {
  const summary = shapeOf(suggestion({ payload: { notes: 'Eleven comments over three days.' } }), item());
  assert.ok(isSummary(summary));
  assert.equal(headingCodeOf(summary), 'app.suggestions.kind_summary');
  assert.equal(acceptCodeOf(summary), 'app.suggestions.accept_summary');
  const fields = shapeOf(suggestion({ payload: { title: 'A title', notes: 'and notes' } }), item());
  assert.ok(!isSummary(fields));
});

test('a classification names what accepting does: labels, a column, or both', () => {
  const labels = shapeOf(suggestion({ payload: { label_ids: ['l1'] } }), item());
  assert.equal(headingCodeOf(labels), 'app.suggestions.kind_labels');
  assert.equal(acceptCodeOf(labels), 'app.suggestions.accept_labels');
  const column = shapeOf(suggestion({ payload: { bucket_id: 'b1' } }), item());
  assert.equal(acceptCodeOf(column), 'app.suggestions.accept_column');
  const both = shapeOf(suggestion({ payload: { label_ids: ['l1'], bucket_id: 'b1' } }), item());
  assert.equal(headingCodeOf(both), 'app.suggestions.kind_classification');
  assert.equal(acceptCodeOf(both), 'app.suggestions.accept_classification');
});

test('a stale proposal is asked again as the operation that made it, read from the prompt', () => {
  const notes = shapeOf(suggestion({ payload: { notes: 'x' } }), item());
  assert.equal(operationOf(suggestion({ prompt_id: 'summarize-thread' }), notes), 'summarize-thread');
  assert.equal(operationOf(suggestion({ prompt_id: 'summarize' }), notes), 'summarize');
  assert.equal(operationOf(suggestion({ prompt_id: 'suggest-item-fields' }), notes), 'suggest-fields');
  // A prompt this version has never met falls back to the shape.
  assert.equal(operationOf(suggestion({ prompt_id: 'something-newer' }), notes), 'summarize');
  const tree = shapeOf(suggestion({ kind: 'DECOMPOSITION', payload: { children: [] } }), item());
  assert.equal(operationOf(suggestion({ prompt_id: 'something-newer' }), tree), 'decompose');
});

test('a FIELDS payload naming labels or a column is a classification', () => {
  const shape = shapeOf(suggestion({ payload: { label_ids: ['l1', 'l2'], bucket_id: 'b1', custom_fields: { priority: 'high' } } }), item());
  assert.equal(shape.shape, 'classification');
  if (shape.shape !== 'classification') return;
  assert.deepEqual(shape.labelIds, ['l1', 'l2']);
  assert.equal(shape.bucketId, 'b1');
  assert.deepEqual(shape.customFields, { priority: 'high' });
});

test('a DECOMPOSITION is one list, parent before children, each with its depth', () => {
  const shape = shapeOf(
    suggestion({
      kind: 'DECOMPOSITION',
      payload: {
        children: [
          { type: 'WORK_PACKAGE', title: 'Collect', children: [{ type: 'ACTIVITY', title: 'Ask finance' }] },
          { type: 'WORK_PACKAGE', title: 'Write', notes: 'the narrative' },
          { title: '' },
        ],
      },
    }),
    item(),
  );
  assert.equal(shape.shape, 'breakdown');
  if (shape.shape !== 'breakdown') return;
  assert.deepEqual(
    shape.nodes.map((n) => [n.depth, n.type, n.title, n.notes]),
    [
      [0, 'WORK_PACKAGE', 'Collect', undefined],
      [1, 'ACTIVITY', 'Ask finance', undefined],
      [0, 'WORK_PACKAGE', 'Write', 'the narrative'],
    ],
  );
  assert.equal(acceptCodeOf(shape), 'app.suggestions.accept_breakdown');
});

test('DUPLICATES is links and nothing to accept', () => {
  const shape = shapeOf(
    suggestion({ kind: 'DUPLICATES', payload: { duplicates: [{ item_id: 'i9', similarity: 0.91 }, { item_id: 7 }] } }),
    item(),
  );
  assert.equal(shape.shape, 'duplicates');
  if (shape.shape !== 'duplicates') return;
  assert.deepEqual(shape.neighbours, [{ itemId: 'i9', similarity: 0.91 }]);
  assert.equal(acceptCodeOf(shape), undefined);
});

test('a TEMPLATE is a named draft with its root type and its tree, parent before children (F6-10)', () => {
  const shape = shapeOf(
    suggestion({
      kind: 'TEMPLATE',
      payload: {
        scope_type: 'COLLECTION',
        name: 'Onboarding',
        description: 'A new colleague\'s first week',
        root_type: 'TASK',
        nodes: [
          { type: 'WORK_PACKAGE', title: 'Accounts', children: [{ type: 'ACTIVITY', title: 'Create the mailbox', notes: 'IT does this' }] },
          { type: 'WORK_PACKAGE', title: 'Introductions' },
        ],
      },
    }),
    undefined,
  );
  assert.equal(shape.shape, 'template');
  if (shape.shape !== 'template') return;
  assert.equal(shape.name, 'Onboarding');
  assert.equal(shape.description, 'A new colleague\'s first week');
  assert.equal(shape.rootType, 'TASK');
  assert.deepEqual(
    shape.nodes.map((n) => [n.depth, n.type, n.title, n.notes]),
    [
      [0, 'WORK_PACKAGE', 'Accounts', undefined],
      [1, 'ACTIVITY', 'Create the mailbox', 'IT does this'],
      [0, 'WORK_PACKAGE', 'Introductions', undefined],
    ],
  );
  assert.equal(headingCodeOf(shape), 'app.suggestions.kind_template');
  assert.equal(acceptCodeOf(shape), 'app.suggestions.accept_template');
  // A row written before the count existed says nothing was dropped (issue 767).
  assert.equal(shape.dropped, 0);
  const narrowed = shapeOf(
    suggestion({ kind: 'TEMPLATE', payload: { name: 'Onboarding', root_type: 'TASK', nodes: [] }, dropped_nodes: 3 }),
    undefined,
  );
  assert.equal(narrowed.shape === 'template' ? narrowed.dropped : undefined, 3);
  // A draft without a name is nothing CreateTemplate could make: unknown, not an empty heading.
  assert.equal(shapeOf(suggestion({ kind: 'TEMPLATE', payload: { nodes: [] } }), undefined).shape, 'unknown');
});

test('a kind this version has never met is unknown rather than braces', () => {
  const shape = shapeOf(suggestion({ kind: 'LATER_KIND' as never, payload: { nodes: [] } }), item());
  assert.equal(shape.shape, 'unknown');
  assert.equal(headingCodeOf(shape), 'app.suggestions.kind_unknown');
  assert.equal(acceptCodeOf(shape), undefined);
});

// --- the follow ---------------------------------------------------------------------------------

/**
 * A transport that answers the listing from a series, one answer per read, and refuses or
 * accepts what is sent to it. Only what the follow and the decisions touch; the rest throws.
 */
class ListingTransport implements Transport {
  readonly reads: string[] = [];
  readonly sends: { method: string; path: string; body: unknown }[] = [];
  #listing: unknown[];
  #refusals = new Map<string, TransportError>();

  constructor(listing: unknown[]) {
    this.#listing = listing;
  }

  refuse(path: string, error: TransportError): this {
    this.#refusals.set(path, error);
    return this;
  }

  async get<T>(path: string): Promise<Response<T>> {
    await Promise.resolve();
    this.reads.push(path);
    const body = this.#listing.length > 1 ? this.#listing.shift() : this.#listing[0];
    return { status: 200, body: body as T };
  }

  async send<T>(method: 'POST' | 'PATCH' | 'PUT' | 'DELETE', path: string, body: unknown): Promise<Response<T>> {
    await Promise.resolve();
    this.sends.push({ method, path, body });
    const refusal = this.#refusals.get(path);
    if (refusal) throw refusal;
    return { status: 202, body: undefined as T };
  }

  stream(): never {
    throw new Error('not streamed here');
  }
  snapshot(): never {
    throw new Error('not synchronised here');
  }
  transfer(): never {
    throw new Error('not transferred here');
  }
  document(): never {
    throw new Error('not documented here');
  }
}

const page = (...items: Suggestion[]) => ({ items, has_more: false });
const noWait = async () => {};

test('the listing is re-read until a proposal made after the ask stands in it', async () => {
  const askedAt = '2026-09-16T12:00:00Z';
  const before = suggestion({ id: 'old', created_at: '2026-09-16T11:00:01Z' });
  const after = suggestion({ id: 'new', created_at: '2026-09-16T12:00:05Z' });
  const transport = new ListingTransport([page(before), page(before), page(before, after)]);
  const engine = new SyncEngine({ transport });

  const outcome = await followArrival(engine, ITEM, askedAt, noWait);

  assert.equal(outcome, 'arrived');
  assert.equal(transport.reads.length, 3, 'stops at the read that shows the answer');
  assert.ok(transport.reads.every((path) => path === suggestionsPath(ITEM)));
});

test('a decided proposal is not an arrival, and the follow gives up rather than polling forever', async () => {
  const decided = suggestion({ id: 'done', status: 'DISMISSED', created_at: '2026-09-16T12:00:05Z' });
  const transport = new ListingTransport([page(decided)]);
  const engine = new SyncEngine({ transport });

  const outcome = await followArrival(engine, ITEM, '2026-09-16T12:00:00Z', noWait);

  assert.equal(outcome, 'gave_up');
  assert.equal(transport.reads.length, FOLLOW_ATTEMPTS);
});

test('a screen that was left stops the follow between two reads', async () => {
  const transport = new ListingTransport([page()]);
  const engine = new SyncEngine({ transport });
  let reads = 0;
  const outcome = await followArrival(engine, ITEM, '2026-09-16T12:00:00Z', noWait, () => reads++ < 3);
  assert.equal(outcome, 'left');
  assert.ok(transport.reads.length <= 2);
});

test('arrivedSince reads the listing the way the follow does', () => {
  const asked = '2026-09-16T12:00:00Z';
  assert.ok(!arrivedSince([], asked));
  assert.ok(!arrivedSince([suggestion({ created_at: '2026-09-16T11:59:59Z' })], asked));
  assert.ok(arrivedSince([suggestion({ created_at: '2026-09-16T12:00:00Z' })], asked));
  assert.ok(!arrivedSince([suggestion({ created_at: '2026-09-16T12:00:00Z', status: 'ACCEPTED' })], asked));
});

// --- the stale refusal --------------------------------------------------------------------------

test('a stale acceptance is the server’s refusal, carried as the code the catalogue renders', async () => {
  const transport = new ListingTransport([page()]).refuse(
    '/suggestions/s1:accept',
    new TransportError('problem', { status: 409, code: 'errors.conflict', detailCode: 'suggestions.stale' }),
  );
  const engine = new SyncEngine({ transport });

  await assert.rejects(
    engine.mutate('POST', '/suggestions/s1:accept', {}, { idempotencyKey: 'k1' }),
    (error: unknown) => error instanceof TransportError && error.detailCode === 'suggestions.stale',
  );
  assert.equal(transport.sends[0]?.path, '/suggestions/s1:accept');
});
