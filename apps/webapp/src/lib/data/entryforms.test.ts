// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-FLW-01 check 2 on the client: the form is drawn in its sections and order, capture unfolds
// what is required or may become so and never what the server's answer hides, a refusal lands on
// its field, and the editor sends what somebody designed and nothing empty.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { EntryForm, EntryFormState } from '@hubtask/sync-engine';

import {
  addableKeys, channelIn, draftOfForm, formOfDraft, groupsOf, hasForm, moveField, refusalsByField, removeField,
  removeSection, standingOf, unfilled, unfoldedIn, validationWords,
} from './entryforms.ts';

const form: EntryForm = {
  sections: [{ key: 'impact', name: 'Impact' }, { key: 'empty', name: 'Nothing here' }],
  fields: [
    { key: 'title', required: false },
    { key: 'custom_fields.severity', section: 'impact', required: true },
    { key: 'notes', required: false, required_when: 'entry.custom_fields.severity == "high"' },
    { key: 'custom_fields.browser', required: false, hidden_when: 'channel == "MAIL"' },
  ],
  validations: [{ key: 'short', field: 'title', holds: 'size(entry.title) < 80', message: 'Keep it short.' }],
};

const state = (fields: EntryFormState['fields']): EntryFormState => ({
  entry_type: 'bug', channel: 'APP', fields, failed: [], closed: false,
});

test('an empty form asks nothing', () => {
  assert.equal(hasForm(undefined), false);
  assert.equal(hasForm({ sections: [], fields: [] }), false);
  assert.equal(hasForm(form), true);
});

test('the fields are grouped by section in the form order, empty sections left out', () => {
  const groups = groupsOf(form);
  assert.deepEqual(groups.map((group) => group.section?.key), [undefined, 'impact']);
  assert.deepEqual(groups[0]?.fields.map((field) => field.key), ['title', 'notes', 'custom_fields.browser']);
});

test('capture unfolds required and conditional fields, never the title or a hidden one', () => {
  assert.deepEqual(unfoldedIn(form, undefined).map((field) => field.key), ['custom_fields.severity', 'notes']);
  const answered = state({
    title: { required: false, read_only: false, hidden: false },
    'custom_fields.severity': { required: true, read_only: false, hidden: false },
    notes: { required: true, read_only: false, hidden: true },
    'custom_fields.browser': { required: true, read_only: false, hidden: false },
  });
  assert.deepEqual(unfoldedIn(form, answered).map((field) => field.key), ['custom_fields.severity', 'custom_fields.browser']);
  assert.deepEqual(unfilled(form, answered, { custom: { severity: 'high' } }), ['custom_fields.browser']);
});

test('the answer decides; without one the form says what it says', () => {
  const field = { key: 'notes', required: true };
  assert.deepEqual(standingOf(field, undefined), { required: true, readOnly: false, hidden: false });
  assert.deepEqual(
    standingOf(field, state({ notes: { required: false, read_only: true, hidden: false } })),
    { required: false, readOnly: true, hidden: false },
  );
});

test('a refusal lands on its field, and a validation speaks the workspace words', () => {
  const byField = refusalsByField(new Map([['/custom_fields/severity', 'Fill this in.'], ['/notes', 'No.']]));
  assert.equal(byField.get('custom_fields.severity'), 'Fill this in.');
  assert.equal(byField.get('notes'), 'No.');
  assert.equal(validationWords(form, 'short'), 'Keep it short.');
  assert.equal(validationWords(form, 'other'), undefined);
});

test('the editor moves, removes and sends only what was designed', () => {
  const draft = draftOfForm(form);
  assert.deepEqual(addableKeys(draft, ['severity', 'size']), ['due_date', 'start_at', 'custom_fields.size']);

  moveField(draft, 'notes', -1);
  assert.deepEqual(draft.fields.map((field) => field.key).slice(0, 2), ['title', 'notes']);
  moveField(draft, 'title', -1);
  assert.equal(draft.fields[0]?.key, 'title');

  const mail = channelIn(draft, 'MAIL');
  mail.hidden = ['custom_fields.browser'];
  mail.presets = { 'custom_fields.browser': 'unknown' };
  channelIn(draft, 'MCP');
  removeField(draft, 'custom_fields.browser');
  assert.deepEqual(mail.hidden, []);
  assert.deepEqual(mail.presets, {});

  removeSection(draft, 'impact');
  draft.fields[0]!.help = '  ';
  const severity = draft.fields.find((field) => field.key === 'custom_fields.severity')!;
  severity.default = '3';
  const sent = formOfDraft(draft, (key, typed) => (key === 'custom_fields.severity' ? Number(typed) : typed));
  assert.deepEqual(sent.fields?.[0], { key: 'title', required: false });
  assert.deepEqual(sent.fields?.find((field) => field.key === 'custom_fields.severity'), { key: 'custom_fields.severity', required: true, default: 3 });
  assert.deepEqual(sent.channels, [], 'a channel that narrows nothing is not sent');
  // The source form stays as it was.
  assert.equal(form.fields?.length, 4);
});
