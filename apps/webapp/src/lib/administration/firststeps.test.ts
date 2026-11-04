// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { firstSteps, isOwed, type FirstStepFacts } from './firststeps.ts';

const owner: FirstStepFacts = {
  holds: () => true,
  isDeskOn: true,
  hasSeveralPeople: false,
  hasModuleOn: true,
  hasMailbox: false,
  hasSignIn: false,
  hasNightlyBackup: true,
  mayAddBackupTarget: true,
  hasSupportContact: false,
};

test('an owner of a new workspace is shown every step, the done ones ticked', () => {
  const steps = firstSteps(owner);
  assert.equal(steps.isSettled, true);
  assert.deepEqual(steps.steps.map((step) => [step.id, step.isDone]), [
    ['invite', false], ['products', true], ['mailbox', false], ['signin', false], ['backup', true], ['support', false],
  ]);
  assert.equal(steps.done, 2);
  assert.equal(isOwed(steps), true);
});

test('a step is shown only to whoever may act on it', () => {
  const steps = firstSteps({ ...owner, holds: (permission) => permission === 'MANAGE_MEMBERS' });
  assert.deepEqual(steps.steps.map((step) => step.id), ['invite']);
  const none = firstSteps({ ...owner, holds: () => false });
  assert.deepEqual(none.steps, []);
  assert.equal(isOwed(none), false, 'a reader who may act on nothing is shown no card');
});

test('the mailbox step is the service desk\'s', () => {
  assert.equal(firstSteps({ ...owner, isDeskOn: false }).steps.some((step) => step.id === 'mailbox'), false);
});

test('the backup step is left out where the operator keeps the backups', () => {
  const steps = firstSteps({ ...owner, hasNightlyBackup: false, mayAddBackupTarget: false });
  assert.equal(steps.steps.some((step) => step.id === 'backup'), false, 'a step the workspace may never tick is not drawn');
});

test('the card waits for every fact a shown step needs', () => {
  const steps = firstSteps({ ...owner, hasNightlyBackup: undefined });
  assert.equal(steps.isSettled, false);
  assert.equal(isOwed(steps), false, 'nothing is drawn that turns done a moment later');
  assert.equal(firstSteps({ ...owner, hasNightlyBackup: undefined, holds: (permission) => permission !== 'DELETE_CONTAINER' }).isSettled, true,
    'a fact of a step the reader is not shown is not waited for');
});

test('a fact that cannot be read leaves its step out rather than open for good', () => {
  const steps = firstSteps({ ...owner, hasMailbox: null });
  assert.equal(steps.isSettled, true);
  assert.equal(steps.steps.some((step) => step.id === 'mailbox'), false);
});

test('the card is gone once everything shown is done', () => {
  const done = firstSteps({ ...owner, hasSeveralPeople: true, hasMailbox: true, hasSignIn: true, hasSupportContact: true });
  assert.equal(done.done, done.steps.length);
  assert.equal(isOwed(done), false);
});
