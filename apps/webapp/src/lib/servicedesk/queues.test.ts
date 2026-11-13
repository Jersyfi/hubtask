// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DESK_QUEUES, desksAnswered, desksOf, groupedByStatus, priorityLevel, priorityOf, queueQuery, queueTotal } from './queues.ts';

// UC-SVC-03 checks 1 to 3: a queue is ordered by what breaks first and counted exactly; the four
// queues of §7.4 differ only in their narrowing, all inside the hub's desks and open.
test('each queue asks the hub for the open entries of its desks, by what breaks first, counted', () => {
  for (const queue of DESK_QUEUES) {
    const query = queueQuery(queue, 'hub', ['desk-1', 'desk-2']) as { scope: unknown; filter: { nodes: unknown[] }; sort: unknown; count: string };
    assert.deepEqual(query.scope, { container_id: 'hub', include_descendants: true });
    assert.deepEqual(query.filter.nodes.slice(0, 2), [
      { op: 'IN', field: 'collection_id', value: ['desk-1', 'desk-2'] },
      { op: 'EQ', field: 'is_completed', value: false },
    ]);
    assert.deepEqual(query.sort, [{ field: 'sla_due', dir: 'ASC', nulls: 'LAST' }]);
    assert.equal(query.count, 'exact');
  }
  const narrowing = (queue: (typeof DESK_QUEUES)[number]) => (queueQuery(queue, 'hub', ['d']) as { filter: { nodes: unknown[] } }).filter.nodes.slice(2);
  assert.deepEqual(narrowing('mine'), [{ op: 'EQ', field: 'assignee_id', value: '@me' }]);
  assert.deepEqual(narrowing('unassigned'), [{ op: 'IS_NULL', field: 'assignee_id' }]);
  assert.deepEqual(narrowing('breaching'), [{ op: 'IN', field: 'sla_standing', value: ['AT_RISK', 'BREACHED'] }]);
  assert.deepEqual(narrowing('open'), []);
});

test('a hub without a desk is not asked', () => {
  assert.equal(queueQuery('open', 'hub', []), undefined);
});

test('a queue is counted across hubs only once every hub has answered', () => {
  assert.equal(queueTotal({ a: 3, b: 4 }, ['a', 'b']), 7);
  assert.equal(queueTotal({ a: 3 }, ['a', 'b']), undefined);
  assert.equal(queueTotal({ a: 3, b: null }, ['a', 'b']), undefined);
  assert.equal(queueTotal({}, []), 0);
});

// UC-SVC-05: a collection is a desk where it offers a type the portal takes; only those are asked.
// A Scrum collection beside an ITIL one stays a Scrum collection, though the ITIL types are the
// workspace's and so usable everywhere.
test('a hub\'s desks are its collections offering a type open to the portal', () => {
  const type = (key: string, channels?: { channel: string }[]) => ({ key, level: 'TASK', system: false, position: 0, archived: false, form: channels ? { channels } : undefined }) as never;
  const types = [type('incident', [{ channel: 'PORTAL' }]), type('story')];
  const collection = (id: string, offered?: string[]) => ({ id, parent_id: 'hub', structure: offered ? { settings: { entry_types: { TASK: offered } } } : null });
  assert.deepEqual(desksOf([collection('help', ['incident']), collection('scrum', ['story']), collection('all')], types), ['help']);
  assert.deepEqual(desksOf([collection('help', ['incident'])], [type('story')]), []);
});

// user-experience.md §7.4: the queue's priority column reads the field the policies name.
test('a priority is the first field the policies name that the entry answers, else priority', () => {
  assert.equal(priorityOf({ custom_fields: { urgency: 'High', priority: 'P2' } }, ['urgency']), 'High');
  assert.equal(priorityOf({ custom_fields: { priority: 'P2' } }, [null, 'severity']), 'P2');
  assert.equal(priorityOf({ custom_fields: { priority: 3 } }, []), undefined);
  assert.equal(priorityOf({ custom_fields: null }, ['priority']), undefined);
});

test('a priority reads as one of four levels, by its number or its word', () => {
  assert.equal(priorityLevel('P1'), 1);
  assert.equal(priorityLevel('P4'), 4);
  assert.equal(priorityLevel('Highest'), 1);
  assert.equal(priorityLevel('high'), 2);
  assert.equal(priorityLevel('Medium'), 3);
  assert.equal(priorityLevel('low'), 4);
  assert.equal(priorityLevel('whenever'), 3);
});

test('requests stand grouped by status, the group of what breaks first on top, each in its order', () => {
  const rows = [{ id: 'a', status: 'open' }, { id: 'b', status: 'waiting' }, { id: 'c', status: 'open' }, { id: 'd', status: undefined }, { id: 'e', status: 'waiting' }];
  assert.deepEqual(groupedByStatus(rows, (row) => row.status ?? '').map((row) => row.id), ['a', 'c', 'b', 'e', 'd']);
  assert.deepEqual(groupedByStatus([], (row: { id: string }) => row.id), []);
});

test('the desks are the server\'s answer per hub, whatever the entry types read here', () => {
  const ready = (desks?: string[]) => ({ status: 'ready' as const, at: 0, source: 'server' as const, data: { data: [], ...(desks ? { desks } : {}) } });
  assert.equal(desksAnswered({ h1: ready(['d1']) }, ['h1', 'h2']), undefined, 'not until every hub answered');
  assert.deepEqual(desksAnswered({ h1: ready(['d1', 'd2']), h2: ready([]) }, ['h1', 'h2']), { h1: ['d1', 'd2'], h2: [] },
    'a desk with open requests is a desk ("no desk takes requests", concept §10)');
  assert.deepEqual(desksAnswered({ h1: { status: 'failed' as const, error: {} as never }, h2: ready() }, ['h1', 'h2']), { h1: [], h2: [] }, 'a refused hub and an older server name none');
});
