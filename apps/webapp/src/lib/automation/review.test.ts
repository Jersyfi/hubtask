// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { emptyDraft, newStep, type Draft } from './model.ts';
import { cannotRun, review, type ReviewField } from './review.ts';

const FIELDS: Record<string, readonly ReviewField[]> = {
  ADD_LABEL: [
    { name: 'item_id', required: true },
    { name: 'label_id', required: true },
    { name: 'id', required: true, rule: false },
  ],
  SEND_WEBHOOK: [
    { name: 'subscription_id', required: true },
    { name: 'note', required: false },
  ],
};

/** A draft that is ready but for what each test takes away. */
function ready(): Draft {
  return {
    ...emptyDraft('de.hubtask.work.item.overdue.v1'),
    runAs: 'sa-1',
    actions: [{ kind: 'ADD_LABEL', params: { label_id: 'label-1' } }],
  };
}

test('a draft that is ready says nothing', () => {
  assert.deepEqual(review(ready(), FIELDS), []);
  assert.equal(cannotRun([]), false);
});

test('the trigger is read for what its kind needs', () => {
  const draft = ready();
  assert.deepEqual(
    review({ ...draft, trigger: { kind: 'EVENT' } }, FIELDS).map((note) => note.code),
    ['app.flow.review_event_missing'],
  );
  assert.deepEqual(
    review({ ...draft, trigger: { kind: 'SCHEDULE' } }, FIELDS).map((note) => note.code),
    ['app.flow.review_schedule_missing', 'app.flow.review_timezone_missing', 'app.flow.review_needs_entry'],
  );
  assert.deepEqual(
    review({ ...draft, trigger: { kind: 'RELATIVE_DATE' } }, FIELDS).map((note) => note.code),
    ['app.flow.review_offset_missing'],
  );
  assert.deepEqual(
    review({ ...draft, trigger: { kind: 'INBOUND_WEBHOOK' } }, FIELDS, { hasInboundAddress: false }).map((note) => note.code),
    ['app.flow.review_address_missing'],
  );
  assert.deepEqual(review({ ...draft, trigger: { kind: 'INBOUND_WEBHOOK' } }, FIELDS, { hasInboundAddress: true }), []);
});

test('an account has to act, and a rule that does nothing says so', () => {
  const notes = review({ ...ready(), runAs: '', actions: [] }, FIELDS);
  assert.deepEqual(notes.map((note) => [note.level, note.card, note.code]), [
    ['broken', 'run_as', 'app.flow.review_runner_missing'],
    ['attention', '', 'app.flow.review_no_steps'],
  ]);
  assert.equal(cannotRun(notes), true);
});

test('a required parameter the rule does not carry is named at its card, plumbing and the run\'s own aside', () => {
  const notes = review({ ...ready(), actions: [{ kind: 'ADD_LABEL', params: {} }, { kind: 'SEND_WEBHOOK', params: {} }] }, FIELDS);
  // `item_id` is the run's to supply and `id` is the caller's plumbing: neither is the rule's.
  assert.deepEqual(notes.map((note) => [note.card, note.params?.parameter]), [
    ['0', 'label_id'],
    ['1', 'subscription_id'],
  ]);
  assert.equal(notes[0]?.level, 'attention');
});

test('a scheduled run brings no entry, so a step that needs one cannot work', () => {
  const draft: Draft = { ...ready(), trigger: { kind: 'SCHEDULE', rrule: 'FREQ=DAILY', timezone: 'Europe/Berlin' } };
  const notes = review(draft, FIELDS);
  assert.deepEqual(notes.map((note) => [note.level, note.card, note.code, note.params?.kind]), [
    ['broken', '0', 'app.flow.review_needs_entry', 'ADD_LABEL'],
  ]);
  // The same rule on an event: the run has the entry the event is about.
  assert.deepEqual(review(ready(), FIELDS), []);
});

test('a branch that asks nothing, and a branch that does nothing, are both said', () => {
  const branch = newStep('BRANCH');
  const empty = { ...branch, params: { condition: '' } };
  const notes = review({ ...ready(), actions: [empty] }, FIELDS);
  assert.deepEqual(notes.map((note) => [note.level, note.card, note.code]), [
    ['broken', '0', 'app.flow.review_branch_condition_empty'],
    ['attention', '0', 'app.flow.review_branch_empty'],
  ]);

  // An arm's steps are read like the chain's, at their own paths.
  const filled = { ...branch, then: [{ kind: 'ADD_LABEL', params: {} }], else: [{ kind: 'STOP', params: {} }] };
  assert.deepEqual(
    review({ ...ready(), actions: [filled] }, FIELDS).map((note) => note.card),
    ['0/then/0'],
  );
});

test('a wait without a duration is a step that cannot run', () => {
  const notes = review({ ...ready(), actions: [newStep('WAIT')] }, FIELDS);
  assert.deepEqual(notes.map((note) => [note.level, note.code, note.params?.parameter]), [
    ['broken', 'app.flow.review_parameter_missing', 'duration'],
  ]);
});

test('an empty condition of the gate is named at its own card', () => {
  const notes = review({ ...ready(), conditions: ['', "item.type == 'TASK'"] }, FIELDS);
  assert.deepEqual(notes.map((note) => note.card), ['conditions/0']);
});
