// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Structure panel's draft (UC-WRK-23 checks 1-3, 7): what one save sends, and what a lock keeps.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities, EffectiveStructure, Preset } from '@hubtask/sync-engine';

import {
  changeOf,
  chooseFieldsOf,
  choosePreset,
  countOf,
  isOwn,
  levelRefusal,
  renameLevel,
  resetSetting,
  setValue,
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
