// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-FLW-01 checks 3, 5 and 8 on the client: a new entry is offered the types of its level that are
// not archived, the level's own first; a row marks a type of the workspace's own and never the
// level's; and without the workflows module nothing is shown.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities, EntryType } from '@hubtask/sync-engine';

import { ICON_NAMES } from '@hubtask/design-system/icons';

import {
  TYPE_ICONS, childTypes, copyKeyFor, droppedBy, entryTypesOffered, iconOf, keyFrom, languagesLeft, levelTypes, offeredTypes, reorder, rowMarkOf,
  sameTranslations, typesAtHub, translationRowsOf, translationsOf, typeNameOf, typeOrder, typesShown,
} from './entrytypes.ts';

const base = {
  completable: true, workflow: false, record: false, schedulable: true, estimable: false,
  allowed_children: [] as string[], archived: false, version: 1,
};

const types: EntryType[] = [
  { ...base, key: 'feature', level: 'TASK', system: false, name: 'Feature', icon: 'star', color_token: 'blue', position: 2 },
  { ...base, key: 'task', level: 'TASK', system: true, name: null, icon: 'square-check', color_token: 'slate', position: 0, version: 0 },
  { ...base, key: 'bug', level: 'TASK', system: false, name: 'Bug', icon: 'not-an-icon', color_token: 'red', position: 1 },
  { ...base, key: 'old', level: 'TASK', system: false, name: 'Old', icon: 'tag', color_token: 'red', position: 0, archived: true },
  { ...base, key: 'subtask', level: 'WORK_PACKAGE', system: false, name: 'Sub-task', icon: 'check', color_token: 'teal', position: 0 },
];

test('a new task is offered the task types in use, the level own first', () => {
  assert.deepEqual(offeredTypes(types, 'TASK').map((type) => type.key), ['task', 'bug', 'feature']);
  assert.deepEqual(offeredTypes(types, 'WORK_PACKAGE').map((type) => type.key), ['subtask']);
});

test('a collection offers only its own types, the level own still first and an unknown key passed over', () => {
  assert.deepEqual(offeredTypes(types, 'TASK', ['feature', 'bug']).map((type) => type.key), ['feature', 'bug']);
  assert.deepEqual(offeredTypes(types, 'TASK', ['feature', 'task']).map((type) => type.key), ['task', 'feature']);
  assert.deepEqual(offeredTypes(types, 'TASK', ['story', 'old']).map((type) => type.key), ['task', 'bug', 'feature']);
  assert.deepEqual(entryTypesOffered({ entry_types: { TASK: ['bug'] } }, 'TASK'), ['bug']);
  assert.equal(entryTypesOffered({ entry_types: {} }, 'TASK'), undefined);
});

test('a row marks a type of the workspace own, an archived one too, and never the level own', () => {
  assert.equal(rowMarkOf(types, 'TASK', undefined), undefined);
  assert.equal(rowMarkOf(types, 'TASK', 'task'), undefined);
  assert.deepEqual(rowMarkOf(types, 'TASK', 'feature'), { icon: 'star', colorToken: 'blue', name: 'Feature' });
  assert.equal(rowMarkOf(types, 'TASK', 'old')?.name, 'Old');
});

test('an icon the set does not have is drawn as a tag', () => {
  assert.equal(iconOf({ icon: 'not-an-icon' }), 'tag');
});

test('without the workflows module no type is shown', () => {
  assert.equal(typesShown(undefined), false);
  assert.equal(typesShown({ modules_on: [] } as unknown as Capabilities), false);
  assert.equal(typesShown({ modules_on: ['WORKFLOWS'] } as unknown as Capabilities), true);
});

// UC-FLW-01 check 1: the administration lists a level's types with the archived ones last, orders
// them by moving one place, and proposes a key from a name.
test('the administration lists a level\u2019s types, the archived last', () => {
  assert.deepEqual(levelTypes(types, 'TASK').map((type) => type.key), ['task', 'bug', 'feature', 'old']);
});

test('moving a type renumbers only the types whose place changes', () => {
  const moves = reorder(types, 'feature', -1);
  assert.deepEqual(moves.map(({ type, position }) => [type.key, position]), [['feature', 1], ['bug', 2]]);
  assert.deepEqual(reorder(types, 'bug', -1), [], 'the first of its own cannot move above the level own');
  assert.deepEqual(reorder(types, 'task', 1), [], 'the level own type stays first');
  assert.deepEqual(reorder(types, 'old', 1), [], 'an archived type is not ordered');
});

test('a key is proposed from the name', () => {
  assert.equal(keyFrom('Change request'), 'change_request');
  assert.equal(keyFrom('  Überprüfung!  '), 'uberprufung');
});

test('every icon offered for a type is in the design system set', () => {
  for (const icon of TYPE_ICONS) assert.ok((ICON_NAMES as readonly string[]).includes(icon), icon);
});

test('a list groups and a filter offers the types by level, each level own first, archived last', () => {
  assert.deepEqual(typeOrder(types).map((type) => type.key), ['task', 'bug', 'feature', 'old', 'subtask']);
});

test('a group is headed by the type name, a level own type by its level, an unread key by itself', () => {
  const levelName = (level: string) => `level ${level}`;
  assert.equal(typeNameOf(types, 'feature', levelName), 'Feature');
  assert.equal(typeNameOf(types, 'task', levelName), 'level TASK');
  assert.equal(typeNameOf(types, 'gone', levelName), 'gone');
});

test('a change of type that would drop values names them, and a completion is never confirmed away', () => {
  // UC-FLW-01 check 4: the refusal names each field, and the screen asks before anything is lost.
  const refusal = (paths: string[]) => ({
    detailCode: 'entry_types.values_dropped',
    fieldErrors: paths.map((path) => ({ path, code: 'entry_types.values_dropped' })),
  });
  assert.deepEqual(droppedBy(refusal(['/due_at', '/recurrence'])), { fields: ['due_at', 'recurrence'], isConfirmable: true });
  assert.deepEqual(droppedBy(refusal(['/completion', '/due_at'])), { fields: ['completion', 'due_at'], isConfirmable: false });
  assert.equal(droppedBy({ detailCode: 'entry_types.not_offered', fieldErrors: [] }), undefined);
  assert.equal(droppedBy(refusal([])), undefined);
  assert.equal(droppedBy(undefined), undefined);
});

test('a type names its allowed children among the level below, none below an activity', () => {
  const withArchivedChild: EntryType[] = [
    ...types,
    { ...base, key: 'work_package', level: 'WORK_PACKAGE', system: true, name: null, icon: 'work-package', color_token: 'slate', position: 0, version: 0 },
    { ...base, key: 'gone', level: 'WORK_PACKAGE', system: false, name: 'Gone', icon: 'tag', color_token: 'red', position: 1, archived: true },
  ];
  assert.deepEqual(childTypes(withArchivedChild, 'TASK').map((type) => type.key), ['work_package', 'subtask']);
  assert.deepEqual(childTypes(withArchivedChild, 'TASK', ['gone']).map((type) => type.key), ['work_package', 'subtask', 'gone']);
  assert.deepEqual(childTypes(withArchivedChild, 'ACTIVITY'), []);
  assert.deepEqual(childTypes(withArchivedChild, 'HUB'), []);
});

// UC-SVC-05 check 7: the editor writes a type's texts per language as the contract holds them -
// trimmed, an empty member and an empty language left out - and offers only languages not yet written.
test('a type\'s texts per language are written as the contract holds them', () => {
  const rows = translationRowsOf({ translations: { fr: { name: 'Panne' }, de: { name: 'Störung', description: 'Etwas ist kaputt' } } });
  assert.deepEqual(rows.map((row) => row.language), ['de', 'fr']);
  assert.equal(rows[1]?.description, '');
  assert.deepEqual(translationsOf([
    { language: 'de', name: ' Störung ', description: '' },
    { language: 'fr', name: '  ', description: '' },
    { language: 'es', name: '', description: 'Algo está roto' },
  ]), { de: { name: 'Störung' }, es: { description: 'Algo está roto' } });
  assert.ok(sameTranslations({ de: { name: 'Störung' } }, { de: { name: 'Störung' } }));
  assert.ok(sameTranslations(undefined, {}));
  assert.ok(!sameTranslations({ de: { name: 'Störung' } }, { de: { name: 'Fehler' } }));
  assert.deepEqual(languagesLeft(['en', 'de', 'fr'], rows), ['en']);
});

test('a hub\u2019s own type is offered only inside that hub (UC-FLW-01 check 10)', () => {
  const withHubs: EntryType[] = [
    ...types,
    { ...base, key: 'incident', level: 'TASK', system: false, name: 'Incident', icon: 'bug', color_token: 'red', position: 3, hub_id: 'hub-a', copied_from: 'bug' },
    { ...base, key: 'lead', level: 'TASK', system: false, name: 'Lead', icon: 'star', color_token: 'blue', position: 4, hub_id: 'hub-b' },
  ];
  assert.deepEqual(offeredTypes(withHubs, 'TASK', undefined, 'hub-a').map((type) => type.key), ['task', 'bug', 'feature', 'incident']);
  assert.deepEqual(offeredTypes(withHubs, 'TASK', undefined, 'hub-b').map((type) => type.key), ['task', 'bug', 'feature', 'lead']);
  assert.deepEqual(offeredTypes(withHubs, 'TASK').map((type) => type.key), ['task', 'bug', 'feature']);

  const atHub = typesAtHub(withHubs, 'hub-a');
  assert.deepEqual(atHub.shared.map((type) => type.key), ['feature', 'task', 'bug', 'old', 'subtask']);
  assert.deepEqual(atHub.own.map((type) => type.key), ['incident']);
  assert.equal(copyKeyFor(withHubs, 'bug'), 'bug_copy');
  assert.equal(copyKeyFor([...withHubs, { ...base, key: 'bug_copy', level: 'TASK', system: false, name: 'x', icon: 'tag', color_token: 'red', position: 0 }], 'bug'), 'bug_copy_2');
});
