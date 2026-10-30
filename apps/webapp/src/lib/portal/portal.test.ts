// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-SVC-05 checks 1, 2 and 4, UC-DOC-13 check 3: the portal asks the catalogue's questions by
// their kind, sends only real answers, draws a goal by how it stands, and asks articles for the
// words typed so far.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { answersOf, askable, deflectionText, goalTone } from './portal.ts';

const questions = [
  { key: 'room', kind: 'SELECT', options: ['floor_1', 'floor_2'] },
  { key: 'devices', kind: 'MULTI_SELECT', options: ['laptop', 'phone'] },
  { key: 'count', kind: 'NUMBER', options: [] },
  { key: 'urgent', kind: 'BOOL', options: [] },
  { key: 'note', kind: 'TEXT', options: [] },
  { key: 'owner', kind: 'USER', options: [] },
];

test('a question the portal cannot answer is not asked', () => {
  assert.deepEqual(askable(questions).map((q) => q.key), ['room', 'devices', 'count', 'urgent', 'note']);
});

test('only real answers are sent, each in its kind', () => {
  assert.deepEqual(
    answersOf(questions, { room: 'floor_3', devices: ['laptop', 'tablet'], count: '2,5', urgent: true, note: '  ', owner: 'x' }),
    { devices: ['laptop'], count: 2.5, urgent: true },
  );
  assert.deepEqual(answersOf(questions, { room: 'floor_2', count: 'many' }), { room: 'floor_2' });
});

test('a goal is drawn by how it stands', () => {
  const now = new Date('2026-10-30T10:00:00Z');
  assert.equal(goalTone({ standing: 'RUNNING', due_at: '2026-10-30T12:00:00Z', at_risk_at: '2026-10-30T09:00:00Z' }, now), 'warning');
  assert.equal(goalTone({ standing: 'RUNNING', due_at: '2026-10-30T12:00:00Z', at_risk_at: '2026-10-30T11:00:00Z' }, now), 'info');
  assert.equal(goalTone({ standing: 'BREACHED', due_at: null, at_risk_at: null }, now), 'danger');
  assert.equal(goalTone({ standing: 'MET', due_at: null, at_risk_at: null }, now), 'success');
});

test('articles are asked for the words typed so far, never for two letters', () => {
  assert.equal(deflectionText('pr', ''), '');
  assert.equal(deflectionText('Printer  prints', 'blank\npages'), 'Printer prints blank pages');
});
