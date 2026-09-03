// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What F2-03 taught the seam: to state the version it read, to read with a `POST`, to append a
// page, and to drop only what a write actually changed.
//
// Headless, like everything else here (ADR-0033 §2). The four are in one file because they are one
// change: a board is the thing that needs all four at once, and a test that proved them separately
// would not catch the way they have to work together.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SyncEngine } from '../src/SyncEngine.ts';
import { TransportError } from '../src/errors.ts';
import { FakeTransport, FixedClock } from './fakes.ts';

const engineWith = (transport: FakeTransport) => new SyncEngine({ transport, clock: new FixedClock() });

/**
 * Loads a resource and hands back the settled state.
 *
 * `refresh` rather than `subscribe`, because it returns the promise the load is: a subscription
 * starts the load and hands back an unsubscribe, so a test built on it would be waiting on turns
 * of the microtask queue and would start passing or failing with the number of awaits inside the
 * engine.
 */
async function loaded<T>(engine: SyncEngine, request: { path: string; body?: unknown }) {
  return engine.refresh<T>(request);
}

// --- the version a write states ------------------------------------------------------------------

test('a read carries its ETag and a write sends it back', async () => {
  const transport = new FakeTransport()
    .answer('/items/i1', { id: 'i1', title: 'Draft' }, 'W/"7"')
    .answer('/items/i1', { id: 'i1', title: 'Final' }, 'W/"7"');
  const engine = engineWith(transport);

  const state = await loaded<{ id: string }>(engine, { path: '/items/i1' });
  assert.equal(state.status, 'ready');
  assert.equal(state.status === 'ready' ? state.etag : undefined, 'W/"7"');

  await engine.mutate('PATCH', '/items/i1', { title: 'Final' }, {
    ifMatch: state.status === 'ready' ? state.etag : undefined,
  });

  const write = transport.calls.find((c) => c.method === 'PATCH');
  assert.equal(write?.options.ifMatch, 'W/"7"');
});

test('etagFor answers the tag a caller did not keep', async () => {
  const transport = new FakeTransport().answer('/items/i1', { id: 'i1' }, 'W/"3"');
  const engine = engineWith(transport);
  await loaded(engine, { path: '/items/i1' });
  assert.equal(engine.etagFor('/items/i1'), 'W/"3"');
  assert.equal(engine.etagFor('/items/i2'), undefined);
});

test('a stale version is a version conflict a caller can act on, not a generic failure', async () => {
  const transport = new FakeTransport().fail(
    '/items/i1',
    new TransportError('problem', { status: 409, code: 'version_conflict' }),
  );
  const engine = engineWith(transport);

  await assert.rejects(
    () => engine.mutate('PATCH', '/items/i1', { title: 'Mine' }, { ifMatch: 'W/"1"' }),
    (error: unknown) => {
      assert.ok(error instanceof TransportError);
      assert.ok(error.isVersionConflict, 'the UI cannot tell the reader their row moved');
      return true;
    },
  );
});

test('a 409 that is not a version conflict is not reported as one', async () => {
  // A container whose name is taken is refused with the same status and is a different sentence to
  // a different person. Reading the status alone would merge the two.
  const clash = new TransportError('problem', { status: 409, code: 'container.name_taken' });
  assert.equal(clash.isVersionConflict, false);
});

// --- a read that is a POST ------------------------------------------------------------------------

test('a query is a POST that reads: it is subscribed to, and it invalidates nothing', async () => {
  const transport = new FakeTransport()
    .answer('/items:query', { data: [{ id: 'i1' }], groups: [], page: { next_cursor: null, has_more: false } })
    .answer('/containers', { data: [{ id: 'c1' }], page: { next_cursor: null, has_more: false } });
  const engine = engineWith(transport);

  await loaded(engine, { path: '/containers' });
  const query = await loaded(engine, { path: '/items:query', body: { scope: { container_id: 'c1' } } });

  assert.equal(query.status, 'ready');
  assert.equal(transport.calls.filter((c) => c.path === '/items:query')[0]?.method, 'POST');
  // The list read before it is still held: a read may not drop what another read put there.
  assert.equal(engine.peek({ path: '/containers' }).status, 'ready');
});

test('two identical questions are one entry, two different ones are two', async () => {
  const transport = new FakeTransport().answer('/items:query', { data: [], page: { next_cursor: null } });
  const engine = engineWith(transport);

  await loaded(engine, { path: '/items:query', body: { scope: { container_id: 'c1' }, include_archived: false } });

  // The same question with its keys written in the other order finds what the first one loaded.
  // Without a stable key this is `idle`, and a board loads each column twice.
  const sameQuestion = { path: '/items:query', body: { include_archived: false, scope: { container_id: 'c1' } } };
  assert.equal(engine.peek(sameQuestion).status, 'ready');

  // A different question of the same path is a different entry, which is why the key is not the
  // path alone: two columns of one board are both `POST /items:query`.
  assert.equal(engine.peek({ path: '/items:query', body: { scope: { container_id: 'c2' } } }).status, 'idle');
});

// --- a page appended -------------------------------------------------------------------------------

test('a second page is appended to the first, not put in its place', async () => {
  // The second page is a different request, so it is a different entry in the fake: the cursor is
  // in the path. That is the engine being right rather than the test being awkward.
  const transport = new FakeTransport()
    .answer('/containers', { data: [{ id: 'c1' }], page: { next_cursor: 'cur-2', has_more: true } })
    .answer('/containers?cursor=cur-2', { data: [{ id: 'c2' }], page: { next_cursor: null, has_more: false } });
  const engine = engineWith(transport);

  await loaded(engine, { path: '/containers' });
  const state = await engine.loadMore<{ data: { id: string }[] }>({ path: '/containers' });

  assert.equal(state.status, 'ready');
  assert.deepEqual(state.status === 'ready' ? state.data.data.map((c) => c.id) : [], ['c1', 'c2']);
  // The state stays under the request the caller subscribed to, not under the cursored one, or a
  // component would lose its list the moment it asked for more of it.
  assert.equal(engine.peek({ path: '/containers' }).status, 'ready');
});

test('a query pages on one path, and its rows accumulate', async () => {
  // A query's cursor travels in the document, so both pages are the same path - which is what
  // `answerEach` exists for, and what a board's column actually does.
  const transport = new FakeTransport().answerEach('/items:query', [
    { data: [{ id: 'i1' }], groups: [], page: { next_cursor: 'cur-2', has_more: true } },
    { data: [{ id: 'i2' }], groups: [], page: { next_cursor: null, has_more: false } },
  ]);
  const engine = engineWith(transport);
  const column = { path: '/items:query', body: { scope: { container_id: 'c1' } } };

  await loaded(engine, column);
  const state = await engine.loadMore<{ data: { id: string }[] }>(column);

  assert.deepEqual(state.status === 'ready' ? state.data.data.map((i) => i.id) : [], ['i1', 'i2']);
});

test('the cursor of a GET goes in the query string, once', async () => {
  const transport = new FakeTransport().answerEach('/containers?type=HUB', [
    { data: [{ id: 'c1' }], page: { next_cursor: 'cur-2', has_more: true } },
  ]);
  const engine = engineWith(transport);
  await loaded(engine, { path: '/containers?type=HUB' });
  await engine.loadMore({ path: '/containers?type=HUB' });

  const paths = transport.calls.map((c) => c.path);
  assert.equal(paths[1], '/containers?type=HUB&cursor=cur-2');
  assert.equal((paths[1]?.match(/\?/g) ?? []).length, 1, 'a hand-built query string grew a second ?');
});

test('the cursor of a query goes in the document, leaving the question alone', async () => {
  const transport = new FakeTransport().answerEach('/items:query', [
    { data: [{ id: 'i1' }], groups: [], page: { next_cursor: 'cur-2', has_more: true } },
  ]);
  const engine = engineWith(transport);
  const request = { path: '/items:query', body: { scope: { container_id: 'c1' }, page: { size: 50 } } };

  await loaded(engine, request);
  await engine.loadMore(request);

  assert.deepEqual(transport.calls[1]?.body, {
    scope: { container_id: 'c1' },
    page: { size: 50, cursor: 'cur-2' },
  });
});

test('the last page is a no-op rather than a request', async () => {
  const transport = new FakeTransport().answer('/containers', { data: [{ id: 'c1' }], page: { next_cursor: null } });
  const engine = engineWith(transport);
  await loaded(engine, { path: '/containers' });
  await engine.loadMore({ path: '/containers' });
  assert.equal(transport.calls.length, 1, 'a button bound to loadMore asked for a page that does not exist');
});

test('a page that fails leaves the pages that arrived on screen', async () => {
  const transport = new FakeTransport()
    .answer('/containers', { data: [{ id: 'c1' }], page: { next_cursor: 'cur-2', has_more: true } })
    .fail('/containers?cursor=cur-2', new TransportError('timeout'));
  const engine = engineWith(transport);
  await loaded(engine, { path: '/containers' });

  await assert.rejects(() => engine.loadMore({ path: '/containers' }));

  const state = engine.peek<{ data: { id: string }[] }>({ path: '/containers' });
  assert.equal(state.status, 'ready', 'a list emptied itself because its second page timed out');
  assert.deepEqual(state.status === 'ready' ? state.data.data.map((c) => c.id) : [], ['c1']);
});

// --- invalidation that names what changed -----------------------------------------------------------

test('a write drops what it names and leaves the rest', async () => {
  const transport = new FakeTransport()
    .answer('/containers', { data: [], page: { next_cursor: null } })
    .answer('/items:query', { data: [], groups: [], page: { next_cursor: null } })
    .answer('/items/i1:reorder', { id: 'i1' });
  const engine = engineWith(transport);

  await loaded(engine, { path: '/containers' });
  const board = { path: '/items:query', body: { scope: { container_id: 'c1' } } };
  await loaded(engine, board);

  await engine.mutate('POST', '/items/i1:reorder', { before_item_id: null }, { invalidates: ['/items'] });

  // The board asked about items, so it is stale. The sidebar did not, so it is not.
  assert.equal(engine.peek(board).status, 'idle');
  assert.equal(engine.peek({ path: '/containers' }).status, 'ready');
});

test('a write that names nothing drops everything, because it declared nothing', async () => {
  const transport = new FakeTransport()
    .answer('/containers', { data: [], page: { next_cursor: null } })
    .answer('/items/i1', { id: 'i1' });
  const engine = engineWith(transport);

  await loaded(engine, { path: '/containers' });
  await engine.mutate('PATCH', '/items/i1', { title: 'x' });

  // Broad rather than wrong: showing a stale row is worse than reloading one that did not change.
  assert.equal(engine.peek({ path: '/containers' }).status, 'idle');
});

test('a prefix matches a path and not the question asked of it', async () => {
  const transport = new FakeTransport()
    .answer('/items:query', { data: [], groups: [], page: { next_cursor: null } })
    .answer('/containers/c1', { id: 'c1' });
  const engine = engineWith(transport);

  const one = { path: '/items:query', body: { scope: { container_id: 'c1' } } };
  const two = { path: '/items:query', body: { scope: { container_id: 'c2' } } };
  await loaded(engine, one);
  await loaded(engine, two);

  // Both columns are of the same path, so both go: a write does not know which questions it
  // changed the answer to, and guessing in the narrow direction is what shows a stale card.
  await engine.mutate('PATCH', '/containers/c1', {}, { invalidates: ['/items'] });
  assert.equal(engine.peek(one).status, 'idle');
  assert.equal(engine.peek(two).status, 'idle');
});

test('sign-out still empties everything', async () => {
  const transport = new FakeTransport().answer('/accounts/me', { id: 'a1' });
  const engine = engineWith(transport);
  await loaded(engine, { path: '/accounts/me' });

  engine.reset();

  // offline-sync.md §9.6 applies from the first day there is anything to discard, and a targeted
  // invalidation must not have given sign-out a way to keep something.
  assert.equal(engine.peek({ path: '/accounts/me' }).status, 'idle');
  assert.equal(engine.etagFor('/accounts/me'), undefined);
});
