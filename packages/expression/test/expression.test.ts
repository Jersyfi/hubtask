// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The browser's evaluator held to the server's answers: the form's state as `TestFormState`
// puts it together, a condition over the names `FormEnvironment` declares, and a formula's whole
// numbers as `TestAFormulaMixesWholeNumbersWithNumberFields` reads them.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { calculate, evaluateForm, wholeNumbersAsDecimals } from '../src/index.ts';

test('the state: a door\'s hidden field is hidden, a hidden field is never required, a hidden field\'s validation is not asked', () => {
  const form = {
    fields: [
      { key: 'custom_fields.severity', required: true },
      { key: 'due_date', required_when: 'entry.custom_fields.severity == "high"' },
      { key: 'notes', read_only_when: 'entry.completed', hidden_when: '"auditor" in actor.roles' },
    ],
    validations: [
      { key: 'v', field: 'notes', holds: 'false' },
      { key: 'w', holds: 'size(entry.title) > 3' },
    ],
    channels: [{ channel: 'MAIL' as const, closed: true, hidden: ['custom_fields.severity'], presets: { 'custom_fields.severity': 'low' } }],
  };
  const entry = { title: 'abc', completed: true, customFields: { severity: 'high' } };
  const actor = { role: 'MEMBER', roles: ['MEMBER', 'auditor'], permissions: ['READ'] };

  const app = evaluateForm(form, 'bug', 'APP', entry, actor);
  assert.ok(app);
  assert.equal(app.fields['custom_fields.severity']?.required, true);
  assert.equal(app.fields.due_date?.required, true);
  assert.deepEqual(app.fields.notes, { required: false, read_only: true, hidden: true });
  assert.deepEqual(app.failed, ['w'], 'a validation of a hidden field was not asked');
  assert.equal(app.closed, false);

  const mail = evaluateForm(form, 'bug', 'MAIL', entry, actor);
  assert.deepEqual(mail?.fields['custom_fields.severity'], { required: false, read_only: false, hidden: true });
  assert.equal(mail?.closed, true);
});

test('a condition reads the entry, the actor, the door and the step by the server\'s names', () => {
  const ask = (condition: string, entry = {}, actor = {}) =>
    evaluateForm({ fields: [{ key: 'notes', required_when: condition }] }, 'bug', 'SYNC', entry, actor, 'close')?.fields.notes?.required;
  assert.equal(ask('channel == "SYNC"'), true);
  assert.equal(ask('step == "close"'), true);
  assert.equal(ask('entry.entry_type == "bug"'), true);
  assert.equal(ask('actor.role == "ADMIN"', {}, { role: 'ADMIN' }), true);
  assert.equal(ask('"field.read:salary" in actor.permissions', {}, { permissions: ['field.read:salary'] }), true);
  assert.equal(ask('entry.due_date == "2026-12-24T09:30:00Z"', { dueAt: '2026-12-24T09:30:00.000Z' }), true, 'an instant to the second, in UTC');
  assert.equal(ask('entry.due_date == null'), true, 'no due date is null');
  assert.equal(ask('entry.custom_fields.hours > 1', { customFields: {} }), undefined, 'a key the form does not name is not read');
});

test('a condition that cannot be answered gives no state, and the editor draws the form as it stands', () => {
  assert.equal(evaluateForm({ fields: [{ key: 'notes', hidden_when: 'unknown_name == 1' }] }, 'bug', 'APP', {}, {}), undefined);
  assert.equal(evaluateForm({ fields: [{ key: 'notes', hidden_when: '"not a boolean"' }] }, 'bug', 'APP', {}, {}), undefined);
  assert.equal(evaluateForm({ fields: [{ key: 'notes', hidden_when: '(' }] }, 'bug', 'APP', {}, {}), undefined);
});

test('a formula\'s whole numbers meet its number fields, on the left as on the right', () => {
  const cases: Record<string, number> = {
    'entry.custom_fields.hours * 2': 5,
    '2 * entry.custom_fields.hours': 5,
    'entry.custom_fields.hours + 1': 3.5,
    '10 - entry.custom_fields.hours': 7.5,
    'entry.custom_fields.hours / 2': 1.25,
    'entry.custom_fields.hours > 1 ? 1 : 0': 1,
    '(entry.custom_fields.hours * 2 + 1) / 3': 2,
  };
  for (const [formula, want] of Object.entries(cases)) {
    assert.equal(calculate(formula, { hours: 2.5 }), want, formula);
  }
});

test('a formula that cannot answer leaves no value', () => {
  assert.equal(calculate('entry.custom_fields.hours * 2', {}), null, 'an empty input');
  assert.equal(calculate('entry.custom_fields.hours / 0', { hours: 1 }), null, 'a division by zero');
  assert.equal(calculate('entry.custom_fields.hours * 1000000000000', { hours: 1 }), null, 'past the bound');
  assert.equal(calculate('entry.custom_fields.hours / 3', { hours: 1 }), 0.3333, 'rounded to four places');
});

test('only whole-number literals are rewritten', () => {
  assert.equal(wholeNumbersAsDecimals('a2 * 2 + 1.5 + 1e3 + 0x1F + 3u + "7" + \'8\''), 'a2 * 2.0 + 1.5 + 1e3 + 0x1F + 3u + "7" + \'8\'');
  assert.equal(wholeNumbersAsDecimals('r"\\d 4" + 4'), 'r"\\d 4" + 4.0');
});
