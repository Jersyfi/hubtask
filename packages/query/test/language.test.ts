// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import { countRequest, createCatalogue, createLanguage, segments, valuesRequest, type QueryKey } from '../src/index.ts';

const wording = { key: (key: string) => key.toUpperCase(), problem: (error: { code: string }) => error.code };

// UC-WRK-43 check 6 (Q-2): a line is one row of chips - a term a chip, a group or a word a text
// chip - and what is still being typed stays the rest.
test('a line is cut into chips and the rest', () => {
  assert.deepEqual(segments('who: me, none (a OR b) passkey -label: x due: <today ty'), {
    pieces: ['who: me, none', '(a OR b)', 'passkey', '-label: x', 'due: <today'],
    rest: 'ty',
  });
  const language = createLanguage(wording);
  const { tokens, rest } = language.parse('-who: me, none is: opne ');
  assert.equal(rest, '');
  const [who, is] = tokens;
  assert.ok(who?.kind === 'chip' && who.isNegated && who.values.map((v) => v.value).join() === 'me,none');
  assert.equal(language.format(who), '-who: me, none');
  assert.ok(is?.kind === 'chip' && is.problem?.message === 'query.value_unknown' && is.problem.fix === 'open');
});

const keys: QueryKey[] = [
  { key: 'who', field: 'assignee_id', value: 'person', objects: ['task'], examples: ['who: me'], values: ['me', 'none'] },
  { key: 'due', field: 'due_at', value: 'time', objects: ['task'], examples: ['due: <today'], values: [] },
];

// UC-WRK-43 check 5 (Q-3, Q-4): the catalogue offers the server's keys, a key's own words before
// the counted values, and the count and the values are summaries of the line in a body.
test('the catalogue and the count are asked as the server answers them', () => {
  const catalogue = createCatalogue(keys, wording, () => [{ value: 'u1', label: 'Anna', countLabel: '3' }]);
  assert.deepEqual(catalogue.fields.map((f) => f.key), ['who', 'due']);
  assert.deepEqual(catalogue.values('who', '').map((v) => v.value), ['me', 'none', 'u1']);
  assert.deepEqual(countRequest('is: open passkey', { workspace: true }), {
    scope: { workspace: true }, q: 'is: open passkey', aggregate: { function: 'COUNT' },
  });
  assert.equal(countRequest('is: open sort: due', { workspace: true }), undefined);
  assert.deepEqual(valuesRequest(keys[0] as QueryKey, 'is: open sort: due', { container_id: 'c' }), {
    scope: { container_id: 'c' }, q: 'is: open', aggregate: { function: 'COUNT', group_by: 'assignee_id' },
  });
  assert.equal(valuesRequest(keys[1] as QueryKey, '', { workspace: true }), undefined);
});
