// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { emptyDraft, newStep } from './model.ts';
import { eventGroups, eventWord, eventWords, generatedName, grouped, kindWord, sentence, type Catalogue, type Names } from './words.ts';

/** A catalogue that answers the code and its parameters, so a test reads the shape rather than the prose. */
const words: Catalogue = {
  t: (code, params) => (params ? `${code}(${Object.entries(params).map(([k, v]) => `${k}=${v}`).join(',')})` : code),
  has: (code) => code === 'app.flow.action.add_label' || code.startsWith('app.rules.trigger_'),
};
/** A catalogue with real sentences for the codes given, and nothing for the rest. */
const catalogueOf = (entries: Record<string, string>): Catalogue => ({
  t: (code, params) => {
    let text = entries[code] ?? code;
    for (const [key, value] of Object.entries(params ?? {})) text = text.replace(`{${key}}`, String(value));
    return text;
  },
  has: (code) => code in entries,
});
const names: Names = {
  scope: (scope) => scope.type,
  account: (id) => `acc:${id}`,
  event: (type) => `ev:${type}`,
};

test('an event type is read without its namespace', () => {
  assert.equal(eventWord('de.hubtask.work.item.overdue.v1'), 'item overdue');
  assert.equal(eventWord('de.hubtask.jumble.entry.received.v1'), 'jumble entry received');
});

// The words come from the type's own segments and a table of entities and verbs (decision 13):
// composed, never listed per type; a segment the table has no word for is said as itself.
test('eventWords composes the entity and the verb, and falls back to the segments', () => {
  const words = catalogueOf({
    'app.flow.event_entity_item': 'an entry',
    'app.flow.event_group_item': 'Entries',
    'app.flow.event_verb_created': 'is created',
    'app.flow.event_verb_overdue': 'becomes overdue',
    'app.flow.event_group_other': 'Everything else',
  });
  const created = eventWords(words, 'de.hubtask.work.item.created.v1');
  assert.deepEqual(created, { entity: 'item', group: 'Entries', clause: 'an entry is created', said: 'An entry is created' });
  assert.equal(eventWords(words, 'de.hubtask.work.item.overdue.v1').said, 'An entry becomes overdue');
  // A verb the table lacks: the segments, and the entity's group still.
  const unknown = eventWords(words, 'de.hubtask.work.item.frobnicated.v1');
  assert.equal(unknown.clause, 'item frobnicated');
  assert.equal(unknown.group, 'Entries');
  // An entity the table lacks: the segments, under "everything else".
  const other = eventWords(words, 'de.hubtask.billing.invoice.created.v1');
  assert.equal(other.clause, 'billing invoice created');
  assert.equal(other.group, 'Everything else');
});

test('eventGroups arranges the manifest by entity, entries first, the unknown last', () => {
  const words = catalogueOf({
    'app.flow.event_entity_item': 'an entry', 'app.flow.event_group_item': 'Entries',
    'app.flow.event_entity_comment': 'a comment', 'app.flow.event_group_comment': 'Comments',
    'app.flow.event_verb_created': 'is created', 'app.flow.event_group_other': 'Everything else',
  });
  const groups = eventGroups(words, ['de.hubtask.billing.invoice.created.v1', 'de.hubtask.work.comment.created.v1', 'de.hubtask.work.item.created.v1']);
  assert.deepEqual(groups.map((group) => group.label), ['Entries', 'Comments', 'Everything else']);
  assert.deepEqual(groups[0]?.options, [{ value: 'de.hubtask.work.item.created.v1', label: 'An entry is created' }]);
});

test('kindWord answers the catalogue, or the kind spelled out', () => {
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
