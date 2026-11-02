// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-SVC-05 checks 1, 2 and 4, UC-DOC-13 check 3: the portal asks the catalogue's questions by
// their kind, sends only real answers, draws a goal by how it stands, and asks articles for the
// words typed so far.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { answersOf, askable, deflectionText, deskHead, deskLogoOf, goalTone, isTeamView } from './portal.ts';

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

// UC-SVC-05 check 8: the portal's head is the desk's own name and description, the portal's own
// words standing in for what the desk leaves empty or before the catalogue answers.
test('the head is the desk the portal is for', () => {
  const words = { title: 'Ask for help', intro: 'Tell the team what you need.' };
  assert.deepEqual(deskHead({ name: 'IT help', description: 'Printers and laptops', icon: 'printer', color_token: 'blue' }, words),
    { title: 'IT help', subtitle: 'Printers and laptops' });
  assert.deepEqual(deskHead({ name: 'HR', description: '', icon: '', color_token: '' }, words),
    { title: 'HR', subtitle: 'Tell the team what you need.' });
  assert.deepEqual(deskHead(undefined, words), { title: 'Ask for help', subtitle: 'Tell the team what you need.' });
});

// UC-SVC-05 check 9: a desk with a logo of its own is drawn by it; one without, by its icon.
test('a desk is drawn by its own logo where it has one', () => {
  const desk = { name: 'IT help', description: '', icon: 'printer', color_token: 'blue' };
  assert.equal(deskLogoOf({ ...desk, logo_media_id: '01936f2a-7c1e-7000-8000-00000000e101' }), '01936f2a-7c1e-7000-8000-00000000e101');
  assert.equal(deskLogoOf({ ...desk, logo_media_id: null }), undefined);
  assert.equal(deskLogoOf(desk), undefined);
  assert.equal(deskLogoOf(undefined), undefined);
});

test('the desk\'s team opening its portal is told so, not shown a refusal', () => {
  assert.equal(isTeamView({ status: 403 }), true);
  assert.equal(isTeamView({ status: 404 }), false, 'a desk that is not there is a failure');
  assert.equal(isTeamView({ status: 503 }), false);
  assert.equal(isTeamView(undefined), false);
});
