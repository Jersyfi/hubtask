// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-FLW-01 check 2 on the client: the form is drawn in its sections and order, capture unfolds
// what is required or may become so and never what the server's answer hides, a refusal lands on
// its field, and the editor sends what somebody designed and nothing empty.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { EntryForm, EntryFormState } from '@hubtask/sync-engine';

import {
  addableKeys, channelIn, panelRows, draftOfForm, formOfDraft, groupsOf, hasForm, moveField, refusalsByField, removeField,
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

test('the entry panel lays the custom fields out by the form, hidden ones left out', () => {
  const answer = state({
    'custom_fields.severity': { required: true, read_only: true, hidden: false },
    'custom_fields.browser': { required: false, read_only: false, hidden: true },
  });
  assert.deepEqual(panelRows(form, answer, ['size', 'browser', 'severity']), [
    { key: 'severity', opens: 'Impact', required: true, readOnly: true },
    { key: 'size', required: false, readOnly: false },
  ]);
  assert.deepEqual(panelRows(undefined, undefined, ['size']), [{ key: 'size', required: false, readOnly: false }]);
});

// Offline: the browser's evaluator stands in for `form:evaluate`, over the copy and the door the
// push arrives by.
import { actorOf, offlineFormState, withOwnGrants } from './entryforms.ts';
import type { EntryType, WorkItem } from '@hubtask/sync-engine';

test('offline, the form is answered in the browser through SYNC, the draft over the entry the copy holds', () => {
  const type = {
    key: 'bug', level: 'TASK',
    form: {
      fields: [
        { key: 'custom_fields.impact', required: false },
        { key: 'custom_fields.severity', required: false, required_when: 'entry.custom_fields.impact == "high" && channel == "SYNC"' },
        { key: 'notes', required: false, hidden_when: '"auditor" in actor.roles' },
      ],
      validations: [{ key: 'titled', holds: 'size(entry.title) > 2' }],
      channels: [],
    },
  } as unknown as EntryType;
  const held = { id: 'i-1', title: 'ok', collection_id: 'c-1', custom_fields: { impact: 'high' }, completion: { is_completed: false } } as unknown as WorkItem;
  const state = offlineFormState(type, { item_id: 'i-1', title: 'Leak' }, held, actorOf({ roles: ['MEMBER', 'auditor'], permissions: [] } as never));
  assert.equal(state?.channel, 'SYNC');
  assert.equal(state?.fields['custom_fields.severity']?.required, true, 'the copy\'s value is read');
  assert.equal(state?.fields.notes?.hidden, true, 'a custom role the reader holds is read');
  assert.deepEqual(state?.failed, [], 'the draft\'s title stands over the copy\'s');
  assert.equal(offlineFormState({ key: 'plain', level: 'TASK' } as unknown as EntryType, {}, undefined, {}), undefined, 'no form, no answer');
});

test('the built-in role a condition reads is the strongest the reader holds there', () => {
  assert.equal(actorOf({ roles: ['auditor', 'VIEWER', 'MEMBER'], permissions: ['READ'] } as never).role, 'MEMBER');
  assert.equal(actorOf(undefined).role, undefined);
});

test('offline, the actor holds the custom roles its replica grants along the path, with what they carry', () => {
  const actor = withOwnGrants(
    actorOf({ roles: ['MEMBER'], permissions: ['READ'] } as never),
    [
      { role: 'auditor', scope_type: 'TENANT' },
      { role: 'triage', scope_type: 'COLLECTION', scope_id: 'c-1' },
      { role: 'lead', scope_type: 'HUB', scope_id: 'h-other' },
    ],
    [{ key: 'auditor', permissions: ['READ_CONFIGURATION'] }, { key: 'triage', permissions: ['WRITE'] }, { key: 'lead', permissions: ['MANAGE_MEMBERS'] }],
    ['c-1', 'h-1'],
  );
  assert.deepEqual([...(actor.roles ?? [])].sort(), ['MEMBER', 'auditor', 'triage']);
  assert.deepEqual([...(actor.permissions ?? [])].sort(), ['READ', 'READ_CONFIGURATION', 'WRITE']);
  assert.equal(actor.role, 'MEMBER');
});
