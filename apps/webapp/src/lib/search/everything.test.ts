// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { BAR_KINDS, arrange, countBy, keyIn, kindsOn, marked, markWords, pathOf, requestOf, scopesOf, sinceOf, wordsForUnknownKeys, type Hit } from './everything.ts';

const hit = (kind: Hit['kind'], id: string, extra: Partial<Hit> = {}): Hit => ({
  kind, id, title: id, hub_id: null, collection_id: null, tile: null, snippet: null, state: null, key: null, updated_at: null, on: null, ...extra,
});

test('a kind whose module is off is never asked (P-17)', () => {
  assert.deepEqual(kindsOn(BAR_KINDS, []), ['ENTRY', 'PERSON', 'FILE']);
  assert.deepEqual(kindsOn(BAR_KINDS, ['KNOWLEDGE', 'WHITEBOARD', 'ASSETS']), ['ENTRY', 'PAGE', 'WHITEBOARD', 'PERSON', 'FILE', 'ASSET']);
  assert.deepEqual(kindsOn(BAR_KINDS, ['SERVICE_DESK', 'KNOWLEDGE', 'WHITEBOARD', 'ASSETS']), [...BAR_KINDS]);
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
