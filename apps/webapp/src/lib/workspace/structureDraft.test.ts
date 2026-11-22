// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Structure panel's draft (UC-WRK-23 checks 1-3, 7): what one save sends, and what a lock keeps.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities, EffectiveStructure, Preset } from '@hubtask/sync-engine';

import {
  afterSave,
  changeOf,
  chooseEntryTypesOf,
  chooseFieldsOf,
  choosePreset,
  countOf,
  followStructure,
  isOwn,
  levelRefusal,
  renameLevel,
  resetSetting,
  setValue,
  startAnswered,
  startDraft,
  toggleLevel,
  typesTopDown,
} from './structureDraft.ts';

const manifest = {
  item_types: [
    { type: 'TASK', capabilities: ['COMPLETION', 'DUE_DATE', 'NOTES'], allowed_child_types: ['WORK_PACKAGE', 'ACTIVITY'] },
    { type: 'WORK_PACKAGE', capabilities: ['COMPLETION', 'DUE_DATE'], allowed_child_types: ['ACTIVITY'] },
    { type: 'ACTIVITY', capabilities: ['COMPLETION'], allowed_child_types: [] },
  ],
} as unknown as Capabilities;

const project = {
  preset: 'PROJECT',
  settings: {
    levels: [{ type: 'TASK', name: null }, { type: 'WORK_PACKAGE', name: null }, { type: 'ACTIVITY', name: null }],
    fields: { TASK: { capabilities: ['COMPLETION', 'DUE_DATE', 'NOTES'] } },
    views: { offered: ['LIST_EXPANDED', 'KANBAN'], default: 'LIST_EXPANDED' },
    counts: { figure: 'PROGRESS', field_key: null },
    completion: 'ROLLUP',
    capture: ['DATE', 'PERSON', 'LABEL'],
    celebration: 'ALL',
    automation: [],
  },
  sources: { levels: 'COLLECTION', fields: 'COLLECTION', views: 'COLLECTION', counts: 'HUB', completion: 'COLLECTION', capture: 'DEFAULT', celebration: 'DEFAULT', automation: 'DEFAULT' },
  locks: { views: { origin: 'WORKSPACE', locked_by: '01a0e2e0-0000-7000-8000-000000000011' } },
} as unknown as EffectiveStructure;

const list = {
  key: 'LIST',
  settings: {
    levels: [{ type: 'TASK', name: null }],
    fields: { TASK: { capabilities: ['COMPLETION'] } },
    views: { offered: ['LIST_COLLAPSED'], default: 'LIST_COLLAPSED' },
    counts: { figure: 'OPEN', field_key: null },
    completion: 'MANUAL',
    capture: ['LINES', 'AMOUNT'],
    celebration: 'SUBTLE',
    automation: [],
  },
} as unknown as Preset;

test('nothing changed, nothing sent', () => {
  const draft = startDraft(project);
  assert.equal(changeOf(draft, project, undefined), undefined);
  assert.equal(countOf(undefined), 0);
});

test('one setting changed sends that setting alone; a set compares as a set', () => {
  const reordered = setValue(startDraft(project), 'capture', ['LABEL', 'DATE', 'PERSON']);
  assert.equal(changeOf(reordered, project, undefined), undefined);
  const draft = setValue(startDraft(project), 'celebration', 'SUBTLE');
  assert.deepEqual(changeOf(draft, project, undefined), { settings: { celebration: 'SUBTLE' } });
  assert.equal(countOf(changeOf(draft, project, undefined)), 1);
});

test('a preset replaces the values, keeps the locked ones, and is sent as itself', () => {
  const draft = choosePreset(startDraft(project), project, list);
  assert.deepEqual(draft.values.levels, [{ type: 'TASK', name: null }]);
  assert.deepEqual(draft.values.views, project.settings.views, 'a locked setting kept its value');
  assert.deepEqual(changeOf(draft, project, list), { preset: 'LIST' });
  // A setting changed after the preset was chosen travels on top of it.
  const after = setValue(draft, 'celebration', 'ALL');
  assert.deepEqual(changeOf(after, project, list), { preset: 'LIST', settings: { celebration: 'ALL' } });
});

test('a saved preset is chosen by its id and sent as preset_id (UC-WRK-30 check 2)', () => {
  const saved = { ...list, key: undefined, origin: 'OWN', id: '01a0e2e0-0000-7000-8000-0000000000f1', name: 'Client project' } as unknown as Preset;
  const draft = choosePreset(startDraft(project), project, saved);
  assert.equal(draft.preset, saved.id);
  assert.deepEqual(changeOf(draft, project, saved), { preset_id: saved.id });
  assert.equal(countOf(changeOf(draft, project, saved)), 1);
});

test('a reset sends the setting back; a locked setting is never sent', () => {
  const draft = resetSetting(startDraft(project), project, 'completion');
  assert.deepEqual(changeOf(draft, project, undefined), { reset: ['completion'] });
  const locked = setValue(startDraft(project), 'views', { offered: ['KANBAN'], default: 'KANBAN' });
  assert.equal(changeOf(locked, project, undefined), undefined);
  // Changing it again after a reset takes the reset back.
  assert.deepEqual(setValue(draft, 'completion', 'MANUAL').reset, []);
});

test('only a value set here, and not locked, may be reset (UC-WRK-23 check 3)', () => {
  assert.equal(isOwn(project, 'completion', 'COLLECTION'), true);
  assert.equal(isOwn(project, 'counts', 'COLLECTION'), false, 'set by the hub');
  assert.equal(isOwn(project, 'views', 'COLLECTION'), false, 'locked');
  assert.equal(isOwn(project, 'completion', 'HUB'), false, "a hub's own is HUB");
});

test('the levels: the top always, a level only beneath one it belongs under', () => {
  assert.deepEqual(typesTopDown(manifest), ['TASK', 'WORK_PACKAGE', 'ACTIVITY']);
  assert.equal(levelRefusal(manifest, ['TASK'], 'TASK'), 'top');
  assert.equal(levelRefusal(manifest, ['TASK'], 'ACTIVITY'), undefined, 'an activity may sit under a task');

  const withoutPackages = toggleLevel(manifest, project.settings.levels, 'WORK_PACKAGE');
  assert.deepEqual(withoutPackages.map((level) => level.type), ['TASK', 'ACTIVITY']);
  const again = toggleLevel(manifest, withoutPackages, 'WORK_PACKAGE');
  assert.deepEqual(again.map((level) => level.type), ['TASK', 'WORK_PACKAGE', 'ACTIVITY'], 'the order is top-down');
});

test('a level that loses every level above it goes with it', () => {
  const narrow = { item_types: [
    { type: 'TASK', allowed_child_types: ['WORK_PACKAGE'] },
    { type: 'WORK_PACKAGE', allowed_child_types: ['ACTIVITY'] },
    { type: 'ACTIVITY', allowed_child_types: [] },
  ] } as unknown as Capabilities;
  const levels = [{ type: 'TASK', name: null }, { type: 'WORK_PACKAGE', name: null }, { type: 'ACTIVITY', name: null }] as never;
  assert.deepEqual(toggleLevel(narrow, levels, 'WORK_PACKAGE').map((level) => level.type), ['TASK']);
  assert.equal(levelRefusal(narrow, ['TASK'], 'ACTIVITY'), 'parent');
});

test('a level named, and its name taken away', () => {
  const named = renameLevel(project.settings.levels, 'TASK', 'Item');
  assert.equal(named[0]?.name, 'Item');
  assert.deepEqual(changeOf(setValue(startDraft(project), 'levels', named), project, undefined), { settings: { levels: named } });
  assert.equal(renameLevel(named, 'TASK', '  ')[0]?.name, null);
});

test('the fields checkboxes keep what they do not draw: the categories and an amount not offered here', () => {
  const fields = { TASK: { capabilities: ['COMPLETION', 'NOTES', 'AMOUNT'], categories: ['l1', 'l2'] } } as never;
  const next = chooseFieldsOf(fields, 'TASK', ['COMPLETION', 'DUE_DATE', 'NOTES'], ['DUE_DATE']);
  assert.deepEqual(next.TASK, { capabilities: ['COMPLETION', 'DUE_DATE', 'AMOUNT'], categories: ['l1', 'l2'] });
  // Offered and chosen, an amount is carried like any other field; *Done or open* always.
  const offered = chooseFieldsOf(fields, 'TASK', ['COMPLETION', 'NOTES', 'AMOUNT'], ['AMOUNT']);
  assert.deepEqual(offered.TASK?.capabilities, ['COMPLETION', 'AMOUNT']);
  assert.deepEqual(offered.TASK?.categories, ['l1', 'l2']);
});

test('ticking a level types narrows it, ticking all leaves it out, ticking none changes nothing', () => {
  const inUse = ['task', 'bug', 'feature'];
  assert.deepEqual(chooseEntryTypesOf(undefined, 'TASK', inUse, ['feature', 'bug']), { TASK: ['bug', 'feature'] });
  assert.deepEqual(chooseEntryTypesOf({ TASK: ['bug'] } as never, 'TASK', inUse, inUse), {});
  assert.deepEqual(chooseEntryTypesOf({ TASK: ['bug'] } as never, 'TASK', inUse, []), { TASK: ['bug'] });
});

test('a structure changed elsewhere is not counted as the reader\'s change', () => {
  const changed = { ...project, settings: { ...project.settings, celebration: 'SUBTLE', completion: 'MANUAL' } } as unknown as EffectiveStructure;
  const opened = startDraft(project);
  // The draft opened before: compared with what is in force now, it counts the change made elsewhere.
  assert.equal(countOf(changeOf(opened, changed, undefined)), 2);

  const followed = followStructure(opened, changed, undefined);
  assert.equal(countOf(changeOf(followed, changed, undefined)), 0, 'an untouched draft follows the structure');
  assert.equal(followed.values.celebration, 'SUBTLE');
  assert.equal(followStructure(followed, changed, undefined), followed, 'the same structure keeps the draft');
});

test('a draft the reader changed is kept when the structure changes elsewhere', () => {
  const changed = { ...project, settings: { ...project.settings, completion: 'MANUAL' } } as unknown as EffectiveStructure;
  const edited = setValue(startDraft(project), 'celebration', 'SUBTLE');
  assert.equal(followStructure(edited, changed, undefined), edited);
  const chosen = choosePreset(startDraft(project), project, list);
  assert.equal(followStructure(chosen, changed, list), chosen, 'a preset chosen is a change');
});

test('a just-saved draft is not rolled back while the structure in force is still the one before the save', () => {
  const answered = { ...project, settings: { ...project.settings, celebration: 'SUBTLE' } } as unknown as EffectiveStructure;
  const saved = startAnswered(answered);
  // The panel's own reassignment, the container not refreshed yet: no upstream transition.
  assert.equal(followStructure(saved, project, undefined, project), saved);
  // The refresh arrives: the draft follows it.
  const refreshed = { ...answered } as EffectiveStructure;
  assert.equal(followStructure(saved, refreshed, undefined, project).from, refreshed);
});

test('an edit undone after a change elsewhere follows the structure in force', () => {
  const changed = { ...project, settings: { ...project.settings, completion: 'MANUAL' } } as unknown as EffectiveStructure;
  const edited = setValue(startDraft(project), 'celebration', 'SUBTLE');
  // The change elsewhere arrives; the edit is kept, and the panel records what it saw.
  const kept = followStructure(edited, changed, undefined, project);
  assert.equal(kept, edited);
  // The reader takes the edit back; the panel's own reassignment runs with the same structure seen.
  const undone = setValue(kept, 'celebration', 'ALL');
  const followed = followStructure(undone, changed, undefined, changed);
  assert.equal(changeOf(followed, changed, undefined), undefined, "the change made elsewhere is not sent as the reader's");
});

test('a save answered after a newer structure came into force does not offer the answer as a change', () => {
  const answer = { ...project, settings: { ...project.settings, celebration: 'SUBTLE' } } as unknown as EffectiveStructure;
  // Saved later elsewhere, published while the save's answer was on its way.
  const newer = { ...answer, settings: { ...answer.settings, completion: 'MANUAL' } } as unknown as EffectiveStructure;
  const after = afterSave(answer, project, newer);
  const followed = followStructure(after, newer, undefined, newer);
  assert.equal(changeOf(followed, newer, undefined), undefined, 'the newer setting is not overwritten by the older answer');
});
