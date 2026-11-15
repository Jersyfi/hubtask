// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  careTodos, distinctNames, healthFigures, isByHand, monthsUntil, registerSummary, serviceGraph, serviceWork, sourceRows, troublePath, warrantyTone, workMarks,
} from './inventoryscenes.ts';

const node = (ci_id: string, layer = 3) => ({ ci_id, class: 'server', name: ci_id, stage: 'IN_USE', layer });
// portal → api → db → rack, portal → front → lb → vpn, api → app; a group member under portal.
const MAP = {
  nodes: ['portal', 'api', 'front', 'db', 'app', 'lb', 'rack', 'vpn', 'member', 'elsewhere'].map((id) => node(id)),
  edges: [
    { source: 'portal', target: 'api', type: 'depends_on' },
    { source: 'portal', target: 'front', type: 'depends_on' },
    { source: 'api', target: 'db', type: 'depends_on' },
    { source: 'api', target: 'app', type: 'runs_on' },
    { source: 'front', target: 'lb', type: 'runs_on' },
    { source: 'db', target: 'rack', type: 'located_in' },
    { source: 'lb', target: 'vpn', type: 'depends_on' },
    { source: 'portal', target: 'member', type: 'group:web' },
    { source: 'elsewhere', target: 'db', type: 'depends_on' },
  ],
};
const ids = (graph: ReturnType<typeof serviceGraph>) => graph.columns.map((column) => column.map((each) => each.ci_id));

test('a service map holds what the service rests on, column by column, as deep as asked', () => {
  assert.deepEqual(ids(serviceGraph(MAP, 'portal', 1)), [['portal'], ['api', 'front', 'member']]);
  assert.deepEqual(ids(serviceGraph(MAP, 'portal', 3)), [['portal'], ['api', 'front', 'member'], ['app', 'db', 'lb'], ['rack', 'vpn']]);
  // What rests on the service from elsewhere is not below it.
  assert.ok(!ids(serviceGraph(MAP, 'portal', 3)).flat().includes('elsewhere'));
  // A group's member is joined by its own kind of line.
  assert.ok(serviceGraph(MAP, 'portal', 1).edges.find((edge) => edge.target === 'member')?.isGroup);
  assert.deepEqual(serviceGraph(MAP, 'nowhere', 3), { columns: [], edges: [] });
});

test('problems only keeps the ways to a troubled object, and the service when there is none', () => {
  assert.deepEqual(ids(serviceGraph(MAP, 'portal', 3, new Set(['db']))), [['portal'], ['api'], ['db']]);
  assert.deepEqual(ids(serviceGraph(MAP, 'portal', 3, new Set(['vpn', 'app']))), [['portal'], ['api', 'front'], ['app', 'lb'], ['vpn']]);
  // A trouble deeper than the depth asked is not reached.
  assert.deepEqual(ids(serviceGraph(MAP, 'portal', 1, new Set(['rack']))), [['portal']]);
});

test('the way of a disruption runs from the service down to the struck object', () => {
  const graph = serviceGraph(MAP, 'portal', 3);
  assert.deepEqual([...troublePath(graph, new Set(['db']))].sort(), ['api>db:depends_on', 'portal>api:depends_on']);
  assert.deepEqual([...troublePath(graph, new Set())], []);
});

const entry = (key: string, extra: Record<string, unknown> = {}) => ({
  item_id: key, key, title: key, entry_type: 'incident', collection_id: 'c', hub_id: 'h', status: null, completed: false,
  start_at: null, due_at: null, reason: 'BENEATH' as const, via: { ci_id: 'db', name: 'db', class: 'server' }, ...extra,
});

test('a service\'s open work is apart from what is planned, and the map marks each on its object', () => {
  const entries = [
    entry('SA-12'),
    entry('SA-8', { reason: 'NAMES', via: null }),
    entry('CHA-2', { entry_type: 'change', start_at: '2026-11-20T22:00:00Z', due_at: '2026-11-20T23:00:00Z', via: { ci_id: 'vpn', name: 'vpn', class: 'x' } }),
    entry('CHA-1', { entry_type: 'Change', start_at: '2026-11-19T22:00:00Z', via: { ci_id: 'vpn', name: 'vpn', class: 'x' } }),
    entry('SA-1', { completed: true }),
  ];
  const work = serviceWork(entries);
  assert.deepEqual(work.open.map((each) => each.key), ['SA-12', 'SA-8']);
  assert.deepEqual(work.planned.map((each) => each.key), ['CHA-1', 'CHA-2']);
  const marks = workMarks(entries, 'portal');
  assert.equal(marks.get('db')?.struck?.key, 'SA-12');
  assert.equal(marks.get('portal')?.struck?.key, 'SA-8');
  assert.equal(marks.get('vpn')?.planned?.key, 'CHA-2');
  assert.equal(marks.get('vpn')?.struck, undefined);
});

const HEALTH = {
  total: 12, completeness: 1, correctness: 0.92, stale: 0, orphans: 1, duplicates: 0, without_owner: 1, review_overdue: 1,
  problems: [
    { ci_id: 'nb', class: 'laptop', kinds: ['NO_OWNER' as const, 'STALE' as const] },
    { ci_id: 'ex', class: 'service', kinds: ['ORPHAN' as const, 'DUPLICATE' as const] },
    { ci_id: 'mail', class: 'service', kinds: ['REVIEW_OVERDUE' as const] },
    { ci_id: 'srv', class: 'server', kinds: ['INCOMPLETE' as const], missing: ['serial'] },
  ],
};

test('the six figures are shares and counts, wanting below 100 % or above none', () => {
  const figures = healthFigures(HEALTH, 1);
  assert.deepEqual(figures.map((each) => [each.id, each.value, each.isWanting]), [
    ['complete', 100, false], ['correct', 92, true], ['stale', 0, false], ['orphans', 1, true], ['duplicates', 0, false], ['without_owner', 1, true],
  ]);
  assert.equal(figures[0]!.detail, 1, 'the incomplete objects');
  assert.equal(figures[1]!.detail, 1, 'the deviations from a source');
  // An answer without its counts counts none.
  assert.ok(healthFigures({}, 0).every((each) => !each.isWanting));
});

test('the to-dos are one per problem with one act, the pair and the hold once each', () => {
  const hold = { id: 'h1', hub_id: 'h', run_id: 'r1', source: 'glpi', reason: 'cmdb.hold', item: { ref: 'db-01', class: 'server', name: 'db-01' }, created_at: '2026-11-01T08:00:00Z' };
  const pair = { id: 'd1', hub_id: 'h', ci_ids: ['ex', 'ex2'], rule: 'name', created_at: '2026-11-01T08:00:00Z' };
  const todos = careTodos(HEALTH, [hold], [pair]);
  assert.deepEqual(todos.map((each) => [each.kind, each.act, each.ciId ?? each.hold?.id]), [
    ['NO_OWNER', 'owner', 'nb'], ['ORPHAN', 'relate', 'ex'], ['REVIEW_OVERDUE', 'review', 'mail'], ['HOLD', 'compare', 'h1'],
    ['DUPLICATE', 'decide', 'ex'], ['STALE', 'review', 'nb'], ['INCOMPLETE', 'review', 'srv'],
  ]);
  assert.deepEqual(todos.at(-1)?.missing, ['serial']);
  assert.deepEqual(careTodos(undefined, [], []), []);
});

test('the sources are the connectors bringing objects with their last run, then the sources without one', () => {
  const connector = (key: string, extra: Record<string, unknown> = {}) => ({
    id: key, hub_id: 'h', key, kind: 'GLPI', source: key, push: false, inventory: false, base_url: '', settings: {}, allowed_hosts: [], hosts: [],
    every_minutes: 1440, enabled: true, collection_id: null, has_credential: true, next_run_at: '2026-11-15T06:00:00Z', last_run_at: '2026-11-14T06:00:00Z',
    last_outcome: 'OK', failures: 0, created_at: '', updated_at: '', version: 1, ...extra,
  });
  const counts = { created: 0, updated: 1, unchanged: 11, held: 0, duplicates: 0, refused: 0 };
  const run = (id: string, source: string, extra: Record<string, unknown> = {}) => ({ id, hub_id: 'h', source, actor_id: 'a', started_at: '2026-11-14T06:00:00Z', dry_run: false, counts, ...extra });
  const rows = sourceRows(
    [connector('glpi') as never, connector('alerts', { push: true, kind: 'ALERTMANAGER' }) as never, connector('intune', { enabled: false, last_run_at: null }) as never],
    [run('r3', 'glpi') as never, run('r2', 'glpi', { counts: { ...counts, created: 5 } }) as never, run('r1', 'csv') as never, run('r0', 'csv', { dry_run: true }) as never],
  );
  assert.deepEqual(rows.map((each) => [each.name, each.read, each.changed, each.nextAt]), [
    ['glpi', 12, 1, '2026-11-15T06:00:00Z'], ['intune', undefined, undefined, undefined], ['csv', 12, 1, undefined],
  ]);
  assert.equal(rows[0]!.run?.id, 'r3');
  assert.ok(isByHand({ provenance: { name: { source: 'manual' } } }));
  assert.ok(isByHand({ provenance: {} }));
  assert.ok(!isByHand({ provenance: { name: { source: 'manual' }, ram: { source: 'glpi' } } }));
});

test('months are counted whole and a warranty is ending within six of them on a device still kept', () => {
  assert.equal(monthsUntil('2026-03-14', '2026-07-13'), 3);
  assert.equal(monthsUntil('2026-03-14', '2026-07-14'), 4);
  assert.equal(monthsUntil('2026-11-14', '2028-03-31'), 16);
  const today = '2026-11-14';
  assert.equal(warrantyTone({ state: 'DEPLOYED', warranty_until: '2027-03-20', warranty_ended: false }, today), 'warning');
  assert.equal(warrantyTone({ state: 'DEPLOYED', warranty_until: '2028-03-31', warranty_ended: false }, today), 'neutral');
  assert.equal(warrantyTone({ state: 'IN_STOCK', warranty_until: '2026-01-01', warranty_ended: true }, today), 'warning');
  assert.equal(warrantyTone({ state: 'RETIRED', warranty_until: '2024-01-31', warranty_ended: true }, today), 'neutral');
  assert.equal(warrantyTone({ state: 'ORDERED', warranty_until: null, warranty_ended: false }, today), 'neutral');
});

test('the register says the value issued per currency and the warranties ending soon', () => {
  const asset = (state: string, cost_minor: number, warranty_until: string | null, currency = 'EUR') => ({ state, cost_minor, currency, warranty_until, warranty_ended: false }) as never;
  const summary = registerSummary([
    asset('DEPLOYED', 148000, '2028-03-31'), asset('DEPLOYED', 148000, '2027-03-20'), asset('IN_STOCK', 139000, '2028-09-30'),
    asset('ORDERED', 79900, null), asset('RETIRED', 124000, '2027-01-31'), asset('DEPLOYED', 5000, null, 'CHF'),
  ], '2026-11-14');
  assert.deepEqual(summary, { inUse: [{ currency: 'EUR', minor: 296000 }, { currency: 'CHF', minor: 5000 }], ending: 1, firstInMonths: 4 });
  assert.deepEqual(registerSummary([], '2026-11-14'), { inUse: [], ending: 0 });
});

// Two objects of one name in a list of to-dos are told apart by what differs between them (N26):
// the first attribute both carry with different values, else the end of their identifiers.
test('objects of one name are told apart by what differs between them', () => {
  const ci = (id: string, name: string, attributes: Record<string, unknown>) => ({ id, name, attributes });
  const names = distinctNames([
    ci('a-0001', 'PostgreSQL', { version: '16.4', vendor: 'PGDG' }),
    ci('a-0002', 'PostgreSQL', { version: '15.8', vendor: 'PGDG' }),
    ci('a-0003', 'nginx', {}),
    ci('a-0004', 'Router', { site: 'A' }),
    ci('a-0005', 'router ', { site: 'A' }),
    { id: 'a-0006' } as never,
  ]);
  assert.deepEqual(names, {
    'a-0001': 'PostgreSQL · 16.4',
    'a-0002': 'PostgreSQL · 15.8',
    'a-0003': 'nginx',
    'a-0004': 'Router · 0004',
    'a-0005': 'router · 0005',
  });
});
