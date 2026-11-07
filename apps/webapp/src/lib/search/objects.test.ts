// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { PRODUCTS } from '../navigation.ts';
import { SEARCH_KINDS, kindsOffered } from './kinds.ts';
import { OBJECT_SOURCES, objectHits, readsOf, sourcesWanted } from './objects.ts';

// user-experience.md §5.3: every module that is on contributes a source. Literal on purpose: a
// module that gains named objects is added here, and its source with it. Time and the agents
// have no object a reader looks for by name; a desk is a collection, found among the places;
// pages are searched by the server.
const OBJECT_KINDS: Record<string, readonly string[]> = {
  PLANNING: ['releases', 'sprints'], KNOWLEDGE: [], WHITEBOARD: ['boards'], SERVICE_DESK: [], ASSETS: ['assets'],
  TIME: [], GOALS: ['goals', 'portfolios', 'dashboards'], AGENTS: [],
};

test('every product module contributes its named objects as a source, and each is a kind of the search', () => {
  assert.deepEqual(Object.keys(OBJECT_KINDS).sort(), PRODUCTS.map((row) => row.module).sort(), 'a product without an entry here');
  for (const [module, kinds] of Object.entries(OBJECT_KINDS)) {
    assert.deepEqual(sourcesWanted([module], () => true).map((source) => source.kind), kinds, module);
    for (const kind of kinds) assert.ok(kindsOffered([module]).includes(kind as never), `${kind} is offered with ${module}`);
  }
  for (const source of OBJECT_SOURCES) assert.ok((SEARCH_KINDS as readonly string[]).includes(source.kind));
});

test('a module that is off reads nothing; one that is on reads per place', () => {
  assert.deepEqual(sourcesWanted([], () => true), []);
  assert.deepEqual(sourcesWanted(undefined, () => true), []);
  const reads = readsOf(sourcesWanted(['PLANNING', 'ASSETS', 'GOALS'], (kind) => kind !== 'goals'), { hubs: ['h1', 'h2'], collections: ['c1'] });
  assert.deepEqual(reads.map((read) => read.path), [
    '/portfolios', '/dashboards', '/hubs/h1/cis?limit=500', '/hubs/h2/cis?limit=500', '/hubs/h1/releases', '/hubs/h2/releases', '/collections/c1/sprints',
  ]);
});

test('each source turns its list into rows that open the object', () => {
  const rows = (kind: string, list: unknown) => OBJECT_SOURCES.find((source) => source.kind === kind)?.rows(list) ?? [];
  assert.deepEqual(rows('releases', { data: [{ id: 'r1', hub_id: 'h1', name: 'Autumn' }] }), [{ kind: 'releases', id: 'r1', title: 'Autumn', href: '/hubs/h1/releases', placeId: 'h1' }]);
  assert.deepEqual(rows('sprints', { data: [{ id: 's1', collection_id: 'c1', name: 'Sprint 4' }] })[0]?.href, '/collections/c1/sprints');
  assert.deepEqual(rows('portfolios', { data: [{ id: 'p1', name: 'Growth' }] })[0], { kind: 'portfolios', id: 'p1', title: 'Growth', href: '/portfolios/p1' });
  assert.deepEqual(rows('dashboards', { data: [{ id: 'd1', name: 'Ops', hub_id: null }] })[0]?.href, '/dashboards/d1');
  // A merged duplicate is not found; its survivor is.
  assert.deepEqual(rows('assets', { data: [{ id: 'a', name: 'FortiGate', hub_id: 'h1', merged_into: 'b' }, { id: 'b', name: 'FortiGate', hub_id: 'h1', merged_into: null }] }).map((row) => row.id), ['b']);
  assert.deepEqual(rows('goals', undefined), []);
});

test('the hits hold every word, per kind at most the limit, a name starting with them first', () => {
  const row = (kind: 'goals' | 'releases', id: string, title: string) => ({ kind, id, title, href: `/${id}` });
  const read = {
    '/goals': [row('goals', 'g1', 'Grow the autumn release'), row('goals', 'g2', 'Autumn launch'), row('goals', 'g3', 'Autumn fair')],
    '/hubs/h1/releases': [row('releases', 'r1', 'Autumn')],
  };
  assert.deepEqual(objectHits(read, 'autumn', 2).map((hit) => hit.id), ['g2', 'g3', 'r1']);
  assert.deepEqual(objectHits(read, '  ', 5), []);
});
