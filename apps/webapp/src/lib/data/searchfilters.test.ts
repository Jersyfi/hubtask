// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import { clearChip, fromQuery, isNarrowed, toFilter, toQuery, toggle } from './searchfilters.ts';

test('a chip carries several answers, and choosing one twice takes it off again', () => {
  let chosen = toggle({}, 'type', 'TASK');
  chosen = toggle(chosen, 'type', 'ACTIVITY');
  assert.deepEqual(chosen.type, ['TASK', 'ACTIVITY']);

  chosen = toggle(chosen, 'type', 'TASK');
  assert.deepEqual(chosen.type, ['ACTIVITY']);

  // Emptied, the chip leaves rather than staying as an empty list nobody can see the difference of.
  chosen = toggle(chosen, 'type', 'ACTIVITY');
  assert.equal('type' in chosen, false);
  assert.equal(isNarrowed(chosen), false);
});

test('the address carries the narrowing and reads it back', () => {
  const chosen = toggle(toggle(toggle({}, 'type', 'TASK'), 'type', 'ACTIVITY'), 'state', 'open');
  const query = toQuery(chosen);

  assert.deepEqual(query, { type: 'TASK,ACTIVITY', state: 'open' });
  assert.deepEqual(fromQuery(query), chosen);

  // Anything the address carries that is not a chip is not a chip.
  assert.deepEqual(fromQuery({ type: 'TASK', q: 'tiles', nonsense: 'x' }), { type: ['TASK'] });
  assert.deepEqual(fromQuery({ type: ' , ' }), {});
});

test('several answers of one question widen it, and several questions narrow it', () => {
  const one = toFilter(toggle({}, 'type', 'TASK'));
  assert.deepEqual(one, { op: 'IN', field: 'type', value: ['TASK'] });

  const two = toFilter(toggle(toggle({}, 'type', 'TASK'), 'state', 'done'));
  assert.deepEqual(two, {
    op: 'AND',
    nodes: [
      { op: 'IN', field: 'type', value: ['TASK'] },
      { op: 'EQ', field: 'is_completed', value: true },
    ],
  });

  // Two answers to "when" are an OR: overdue *or* undated is one question with two answers.
  assert.deepEqual(toFilter(toggle(toggle({}, 'when', 'overdue'), 'when', 'undated')), {
    op: 'OR',
    nodes: [
      { op: 'LT', field: 'due_at', value: '@today' },
      { op: 'IS_NULL', field: 'due_at' },
    ],
  });
});

test('a question whose answers are all of them is no narrowing, and costs nothing', () => {
  const both = toggle(toggle({}, 'state', 'open'), 'state', 'done');
  assert.equal(isNarrowed(both), true, 'the chip still shows what was chosen');
  assert.equal(toFilter(both), undefined, 'and it compiles to nothing, because it excludes nothing');
});

test('nothing chosen compiles to no filter at all', () => {
  assert.equal(toFilter({}), undefined);
  assert.deepEqual(toQuery({}), {});
  assert.deepEqual(clearChip(toggle({}, 'label', 'a'), 'label'), {});
});

test('the dates are the server’s placeholders, never a date this browser computed', () => {
  const filter = toFilter(toggle({}, 'when', 'week'));
  assert.deepEqual(filter, { op: 'LTE', field: 'due_at', value: '@end_of_week' });
});
