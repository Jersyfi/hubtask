// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ENTRY_ACTIONS, entryVerdictCode } from './entryaccess.ts';

test('each reason the server gives has its own sentence', () => {
  const reasons = ['THROUGH_ROLE', 'THROUGH_SHARE', 'AS_ASSIGNEE', 'NOT_ASSIGNEE', 'ROLE_REFUSES', 'NOTHING_HELD'] as const;
  const codes = reasons.map((reason) => entryVerdictCode({ action: 'READ', permitted: reason.startsWith('THROUGH') || reason === 'AS_ASSIGNEE', reason }));
  assert.equal(new Set(codes).size, reasons.length);
  assert.ok(codes.every((code) => code.startsWith('app.access.entry_')));
});

// A reason the server adds later is still an answer: the verdict, without its why.
test('a reason this client does not know falls back to the verdict alone', () => {
  assert.equal(entryVerdictCode({ action: 'CHANGE', permitted: true, reason: 'SOMETHING_NEW' as never }), 'app.access.entry_yes');
  assert.equal(entryVerdictCode({ action: 'CHANGE', permitted: false, reason: 'SOMETHING_NEW' as never }), 'app.access.entry_no');
});

test('the actions are the contract three, reading first', () => {
  assert.deepEqual([...ENTRY_ACTIONS], ['READ', 'CHANGE', 'COMMENT']);
});
