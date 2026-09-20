// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { emptyDraft, newStep } from './model.ts';
import { eventWord, generatedName, grouped, kindWord, sentence, type Catalogue, type Names } from './words.ts';

/** A catalogue that answers the code and its parameters, so a test reads the shape rather than the prose. */
const words: Catalogue = {
  t: (code, params) => (params ? `${code}(${Object.entries(params).map(([k, v]) => `${k}=${v}`).join(',')})` : code),
  has: (code) => code === 'app.flow.action.add_label' || code.startsWith('app.rules.trigger_'),
};
const names: Names = {
  scope: (scope) => scope.type,
  account: (id) => `acc:${id}`,
  event: (type) => `ev:${type}`,
};

test('a kind has a word in the catalogue or is read as words, and an event type is read without its namespace', () => {
  assert.equal(eventWord('de.hubtask.work.item.overdue.v1'), 'item overdue');
  assert.equal(eventWord('de.hubtask.jumble.entry.received.v1'), 'jumble entry received');
  assert.equal(kindWord(words, 'ADD_LABEL'), 'app.flow.action.add_label');
  assert.equal(kindWord(words, 'ROTATE_WEBHOOK_SECRET'), 'Rotate webhook secret');
});

test('the palette groups the common kinds it is served and keeps every other under everything else', () => {
  const groups = grouped(['ROTATE_WEBHOOK_SECRET', 'ADD_LABEL', 'SEND_WEBHOOK', 'ADD_COMMENT']);
  assert.deepEqual(
    groups.map((group) => [group.code, group.kinds.join(',')]),
    [
      ['app.flow.group_entries', 'ADD_LABEL'],
      ['app.flow.group_content', 'ADD_COMMENT'],
      ['app.flow.group_outbound', 'SEND_WEBHOOK'],
      ['app.flow.group_other', 'ROTATE_WEBHOOK_SECRET'],
    ],
  );
});

test('the sentence and the name are built from the rule, and follow it', () => {
  const draft = emptyDraft('de.hubtask.work.item.updated.v1');
  draft.runAs = 'sa';
  draft.conditions = ["item.type == 'TASK'", 'size(item.title) > 3'];
  draft.actions = [newStep('ADD_LABEL'), { kind: 'BRANCH', params: { condition: 'has(item.due_at)' }, then: [newStep('SEND_WEBHOOK')], else: [] }];
  assert.equal(generatedName(words, names, draft), 'app.flow.generated_name(trigger=app.flow.name_trigger_event(event=ev:de.hubtask.work.item.updated.v1),steps=app.flow.action.add_label, send webhook)');
  const said = sentence(words, names, draft);
  assert.match(said, /^app\.flow\.sentence\(when=app\.flow\.sentence_when,trigger=app\.flow\.sentence_trigger_event\(event=ev:/);
  assert.match(said, /conditions=app\.flow\.sentence_only_when\(conditions=app\.flow\.in_sentence_subject_type app\.flow\.op_is TASKapp\.flow\.sentence_andapp\.flow\.sentence_expression\)/);
  assert.match(said, /steps=app\.flow\.action\.add_labelapp\.flow\.sentence_then_sepapp\.flow\.sentence_branch\(condition=app\.flow\.in_sentence_subject_due app\.flow\.op_has,then=send webhook,else=app\.flow\.sentence_nothing\)/);
  draft.actions = [];
  assert.equal(generatedName(words, names, draft), 'app.flow.generated_name_empty(trigger=app.flow.name_trigger_event(event=ev:de.hubtask.work.item.updated.v1))');
});
