// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkflowList } from '@hubtask/sync-engine';

import { BAR_KINDS, PAGE_KINDS, arrange, countBy, keyIn, ordered, kindsOn, marked, markWords, pathOf, requestOf, saysItsKind, scopesOf, sinceOf, statusOfHit, tileOf, wordsForUnknownKeys, type Hit } from './everything.ts';

const hit = (kind: Hit['kind'], id: string, extra: Partial<Hit> = {}): Hit => ({
  kind, id, title: id, hub_id: null, collection_id: null, tile: null, snippet: null, state: null, status: null, key: null, updated_at: null, created_at: null, author_id: null, on: null, ...extra,
});

test('a kind whose module is off is never asked (P-17)', () => {
  assert.deepEqual(kindsOn(BAR_KINDS, []), ['ENTRY', 'PERSON', 'FILE']);
  assert.deepEqual(kindsOn(BAR_KINDS, ['KNOWLEDGE', 'WHITEBOARD', 'ASSETS']), ['ENTRY', 'PAGE', 'POST', 'WHITEBOARD', 'PERSON', 'FILE', 'ASSET']);
  assert.deepEqual(kindsOn(BAR_KINDS, ['SERVICE_DESK', 'KNOWLEDGE', 'WHITEBOARD', 'ASSETS', 'GOALS', 'PLANNING']), [...BAR_KINDS]);
});

// UC-WRK-18 check 9. The header search asks what the search page asks; only comments, which its
// scopes do not name (§6.11), are the page's alone.
test('the header search asks every kind the search page asks, comments aside', () => {
  assert.deepEqual([...BAR_KINDS].sort(), PAGE_KINDS.filter((kind) => kind !== 'COMMENT').sort());
  assert.deepEqual(kindsOn(BAR_KINDS, ['GOALS']).filter((kind) => kind === 'GOAL' || kind === 'DASHBOARD'), ['GOAL', 'DASHBOARD']);
});

test('the line travels in the body, whole; nothing is asked without it (Q-5)', () => {
  assert.equal(requestOf('  ', ['ENTRY']), undefined);
  assert.deepEqual(requestOf('passkey type: task', ['ENTRY', 'PAGE'], { hubId: 'h', size: 3 }), { q: 'passkey type: task', kinds: ['ENTRY', 'PAGE'], hub_id: 'h', size: 3 });
});

// UC-WRK-27 check 6. The scopes over the hits, with their counts.
test('the scopes: all with the sum, each kind that found something, here where one stands (§6.11)', () => {
  const scopes = scopesOf([{ kind: 'ENTRY', count: 9, more: false }, { kind: 'PAGE', count: 0, more: false }, { kind: 'FILE', count: 2, more: true }], ['ENTRY', 'PAGE', 'FILE'], true);
  assert.deepEqual(scopes.map((each) => [each.id, each.count]), [['ALL', 11], ['ENTRY', 9], ['FILE', 2], ['HERE', undefined]]);
  assert.equal(scopes[0]?.more, true);
});

// UC-WRK-27 check 6. The best hits first, one of each other kind after them.
test('the best hits are the first of three kinds, the other kinds one each; narrowed, one kind only', () => {
  const hits = [hit('ENTRY', 'e1'), hit('ENTRY', 'e2'), hit('PAGE', 'p1'), hit('WHITEBOARD', 'w1'), hit('PERSON', 'u1'), hit('FILE', 'f1')];
  const { best, more } = arrange(hits, BAR_KINDS);
  assert.deepEqual(best.map((each) => each.id), ['e1', 'p1', 'w1']);
  assert.deepEqual(more.map((each) => each.id), ['u1', 'f1']);
  assert.deepEqual(arrange(hits, BAR_KINDS, 'ENTRY').best.map((each) => each.id), ['e1', 'e2']);
});

test('a hit opens itself; a file or a comment what it is on; a person has no page', () => {
  assert.equal(pathOf(hit('ASSET', 'a')), '/cis/a');
  assert.equal(pathOf(hit('COMMENT', 'c', { on: { kind: 'PAGE', id: 'p' } })), '/pages/p');
  assert.equal(pathOf(hit('FILE', 'f', { on: { kind: 'ENTRY', id: 'e' } })), '/items/e');
  assert.equal(pathOf(hit('PERSON', 'u')), undefined);
});

// UC-WRK-18 check 9. Portfolios, releases and sprints are found by both searches again, each
// opening where its product draws it and saying its kind beside its title.
test('a portfolio, a release and a sprint are asked where their module is on, and open their place', () => {
  for (const kinds of [BAR_KINDS, PAGE_KINDS]) {
    assert.deepEqual(kindsOn(kinds, ['GOALS', 'PLANNING']).filter((kind) => ['PORTFOLIO', 'RELEASE', 'SPRINT'].includes(kind)), ['PORTFOLIO', 'RELEASE', 'SPRINT']);
    assert.deepEqual(kindsOn(kinds, []).filter((kind) => ['PORTFOLIO', 'RELEASE', 'SPRINT'].includes(kind)), []);
  }
  assert.equal(pathOf(hit('PORTFOLIO', 'p')), '/portfolios/p');
  assert.equal(pathOf(hit('RELEASE', 'r', { hub_id: 'h' })), '/hubs/h/plan/releases');
  assert.equal(pathOf(hit('SPRINT', 's', { hub_id: 'h', collection_id: 'c' })), '/collections/c/sprint');
  assert.equal(tileOf(hit('RELEASE', 'r')).product, 'plans');
  assert.equal(tileOf(hit('SPRINT', 's')).product, 'plans');
  assert.equal(tileOf(hit('PORTFOLIO', 'p')).product, 'goals');
  assert.deepEqual(['PORTFOLIO', 'RELEASE', 'SPRINT', 'FILE', 'COMMENT', 'ENTRY', 'GOAL'].map((kind) => saysItsKind(kind as Hit['kind'])), [true, true, true, true, true, false, false]);
});

test('a published post is asked with the pages, named as a post, and opens as one', () => {
  assert.deepEqual(kindsOn(BAR_KINDS, ['KNOWLEDGE']), ['ENTRY', 'PAGE', 'POST', 'PERSON', 'FILE']);
  assert.ok(PAGE_KINDS.includes('POST') && !kindsOn(PAGE_KINDS, []).includes('POST'));
  assert.equal(pathOf(hit('POST', 'p')), '/posts/p');
  assert.ok(saysItsKind('POST'));
  assert.deepEqual(tileOf(hit('POST', 'p')), { icon: 'bell', product: 'pages' });
});

test('found words are marked, in a snippet by its « » and in a title by the words typed', () => {
  assert.deepEqual(marked('a «Passkey» b'), [{ text: 'a ', isFound: false }, { text: 'Passkey', isFound: true }, { text: ' b', isFound: false }]);
  assert.deepEqual(markWords('Passkey-Anmeldung', 'passkey type: task').filter((each) => each.isFound).map((each) => each.text), ['Passkey']);
});

// UC-WRK-27 check 7. A typed key names its entry.
test('a typed key jumps; words are no key', () => {
  assert.equal(keyIn(' sa-12 '), 'SA-12');
  assert.equal(keyIn('TAMA-18'), 'TAMA-18');
  assert.equal(keyIn('passkey 12'), undefined);
});

test('a word: that names no key is searched as words, and named (build plan decision, Q-2)', () => {
  const known = new Set(['who', 'type']);
  assert.deepEqual(wordsForUnknownKeys('Re: Besprechung who: me', known), { line: '"Re:" Besprechung who: me', asWords: ['Re:'] });
  assert.deepEqual(wordsForUnknownKeys('type: task desk.sla: breached', known).asWords, []);
});

test('the changed facet counts today, this week and older', () => {
  const now = new Date(2026, 10, 13, 15);
  assert.equal(sinceOf(new Date(2026, 10, 13, 9).toISOString(), now), 'TODAY');
  assert.equal(sinceOf(new Date(2026, 10, 9, 9).toISOString(), now), 'WEEK');
  assert.equal(sinceOf(new Date(2026, 9, 1).toISOString(), now), 'OLDER');
  const counts = countBy([hit('ENTRY', 'a', { hub_id: 'h' }), hit('PAGE', 'b', { hub_id: 'h' }), hit('PERSON', 'c')], (each) => each.hub_id);
  assert.deepEqual([...counts], [['h', 2]]);
});

// UC-WRK-18 check 9. The search page orders by creation and counts hits by their author (§6.11, scene search 4).
test('the orders: newest made first, a hit that names no making last; the authors counted', () => {
  const hits = [
    hit('PAGE', 'a', { created_at: '2026-10-01T00:00:00Z', author_id: 'p' }),
    hit('FILE', 'b'),
    hit('ENTRY', 'c', { created_at: '2026-11-01T00:00:00Z', author_id: 'p' }),
    hit('ENTRY', 'd', { created_at: '2026-10-15T00:00:00Z', author_id: 'm' }),
  ];
  assert.deepEqual(ordered(hits, 'CREATED', 'en').map((each) => each.id), ['c', 'd', 'a', 'b']);
  assert.deepEqual([...countBy(hits, (each) => each.author_id)], [['p', 2], ['m', 1]]);
});

// Every entry hit said *Offen*: the hit carried only OPEN or DONE. It now names the status its
// collection's workflow gives it, as the board and the list do, in the status's tone.
test('an entry hit names its real status, as its board does', () => {
  const list = {
    data: [{ key: 'support', published: { initial: 'new', steps: [], statuses: [{ key: 'waiting', name: 'Wartet auf Kunde', category: 'IN_PROGRESS' }, { key: 'done', name: null, category: 'DONE' }] } }],
    scheme: [{ container_id: 'hub', entry_type: null, workflow: 'support' }],
  } as unknown as WorkflowList;
  const t = (code: string) => `«${code}»`;
  const named = statusOfHit(hit('TICKET', 'a', { status: 'waiting', collection_id: 'desk', hub_id: 'hub' }), list, t);
  assert.deepEqual(named, { label: 'Wartet auf Kunde', tone: 'warning' });
  assert.deepEqual(statusOfHit(hit('ENTRY', 'b', { status: 'done', collection_id: 'desk', hub_id: 'hub' }), list, t), { label: '«workflows.status.done»', tone: 'success' });
  // Not yet read, or out of sight: a shipped status keeps its own word, any other says nothing.
  assert.deepEqual(statusOfHit(hit('ENTRY', 'c', { status: 'in_progress', collection_id: 'x' }), undefined, t), { label: '«workflows.status.in_progress»', tone: 'warning' });
  assert.equal(statusOfHit(hit('ENTRY', 'd', { status: 'waiting', collection_id: 'x' }), undefined, t), undefined);
  assert.equal(statusOfHit(hit('ENTRY', 'e'), list, t), undefined);
});
