// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import type { Container, StructureSettings } from '@hubtask/sync-engine';

import { captureRulesOf } from '../entries/capture.ts';
import { NOTHING } from './parse.ts';
import { mayCapture, recognitionOf, resolveTarget, rulesOf, topLevelOf, type Admits, type Where } from './target.ts';

const container = (id: string, type: 'HUB' | 'COLLECTION', parent: string | null, order: string): Container =>
  ({ id, type, parent_id: parent, name: id, order_key: order, version: 1 }) as unknown as Container;

const HOUSE = container('house', 'HUB', null, 'a0');
const KITCHEN = container('kitchen', 'COLLECTION', 'house', 'a1');
const ATTIC = container('attic', 'COLLECTION', 'house', 'a0');
const OLD = container('old', 'COLLECTION', 'house', 'a2');
const ALL = [HOUSE, KITCHEN, ATTIC, OLD];

/** Who may create where: `true`, `false`, or not read yet. */
function where(admitted: Record<string, boolean | undefined>, overrides: Partial<Where> = {}): Where {
  const admits: Admits = (collection) => admitted[collection.id];
  return {
    find: (id) => ALL.find((each) => each.id === id),
    collectionsOf: (hubId) => (hubId === 'house' ? [KITCHEN, ATTIC, OLD] : undefined),
    admits,
    jumbleWritable: true,
    ...overrides,
  };
}

const kindOf = (target: ReturnType<typeof resolveTarget>) =>
  target.kind === 'collection' ? `collection:${target.collection.id}` : target.kind;

// ------------------------------------------------------------------------------------------------
// The order (record D6)
// ------------------------------------------------------------------------------------------------

const ROWS: readonly [string, Record<string, boolean | undefined>, Partial<Where>, string][] = [
  ['@ names a collection the reader may write in', { kitchen: true }, { namedCollectionId: 'kitchen', currentCollectionId: 'attic' }, 'collection:kitchen'],
  ['@ names one refused since: nowhere, never somewhere unnamed', { kitchen: false, attic: true }, { namedCollectionId: 'kitchen', currentCollectionId: 'attic' }, 'nowhere'],
  ['# alone: the hub’s first collection in order that admits', { attic: true, kitchen: true }, { namedHubId: 'house' }, 'collection:attic'],
  ['# alone: the first refused, the next taken', { attic: false, kitchen: true }, { namedHubId: 'house' }, 'collection:kitchen'],
  ['# alone, nothing in the hub admits: nowhere', { attic: false, kitchen: false, old: false }, { namedHubId: 'house' }, 'nowhere'],
  ['# alone, the hub’s level unread: pending', {}, { namedHubId: 'house', collectionsOf: () => undefined }, 'pending'],
  ['the current collection', { kitchen: true }, { currentCollectionId: 'kitchen' }, 'collection:kitchen'],
  ['the current collection archived or refused: the jumble', { kitchen: false }, { currentCollectionId: 'kitchen' }, 'jumble'],
  ['no current collection: the jumble', {}, {}, 'jumble'],
  ['on a hub’s page: the hub’s first collection that admits', { attic: true, kitchen: true }, { currentHubId: 'house' }, 'collection:attic'],
  ['on a hub’s page where nothing admits: the jumble, not nowhere', { attic: false, kitchen: false, old: false }, { currentHubId: 'house' }, 'jumble'],
  ['the current collection wins over the hub', { attic: true, kitchen: true }, { currentCollectionId: 'kitchen', currentHubId: 'house' }, 'collection:kitchen'],
  ['no jumble either: nowhere', { kitchen: false }, { currentCollectionId: 'kitchen', jumbleWritable: false }, 'nowhere'],
  ['the manifest unread: pending, not nowhere', {}, { jumbleWritable: undefined }, 'pending'],
  // D14: the same keys must not land in the jumble because Enter came before the role arrived.
  ['role unknown is not the next target: the current collection', { kitchen: undefined }, { currentCollectionId: 'kitchen' }, 'pending'],
  ['role unknown is not the next target: @', { kitchen: undefined }, { namedCollectionId: 'kitchen' }, 'pending'],
  ['role unknown is not the next target: # alone', { attic: undefined, kitchen: true }, { namedHubId: 'house' }, 'pending'],
];

for (const [name, admitted, overrides, expected] of ROWS) {
  test(`target: ${name}`, () => {
    assert.equal(kindOf(resolveTarget(where(admitted, overrides))), expected);
  });
}

test('a collection target names its hub, for the line under the field', () => {
  const target = resolveTarget(where({ kitchen: true }, { currentCollectionId: 'kitchen' }));
  assert.equal(target.kind === 'collection' ? target.hub?.id : undefined, 'house');
});

// ------------------------------------------------------------------------------------------------
// Who sees the control (D13)
// ------------------------------------------------------------------------------------------------

const COLLECTIONS = [KITCHEN, ATTIC];

test('the control: the jumble is writable', () => {
  assert.equal(mayCapture({ isManifestRead: true, jumbleWritable: true, collections: [], admits: () => undefined }), true);
});

test('the control: a child with one hub and no workspace role still captures', () => {
  assert.equal(mayCapture({ isManifestRead: true, jumbleWritable: false, collections: COLLECTIONS, admits: (each) => each.id === 'attic' }), true);
});

test('the control: a reader who may create nowhere has none', () => {
  assert.equal(mayCapture({ isManifestRead: true, jumbleWritable: false, collections: COLLECTIONS, admits: () => false }), false);
});

test('the control: nothing before the manifest, and nothing while every role is unread', () => {
  assert.equal(mayCapture({ isManifestRead: false, jumbleWritable: true, collections: COLLECTIONS, admits: () => true }), false);
  assert.equal(mayCapture({ isManifestRead: true, jumbleWritable: false, collections: COLLECTIONS, admits: () => undefined }), false);
});

// ------------------------------------------------------------------------------------------------
// What a target recognises (D4), from the presets as the domain holds them
// ------------------------------------------------------------------------------------------------

interface FixturePreset {
  readonly key: string;
  readonly levels: readonly string[];
  readonly fields: Readonly<Record<string, readonly string[]>>;
  readonly capture: readonly string[];
}

const PRESETS = (
  JSON.parse(readFileSync(new URL('../../../../../api/fixtures/structure-presets.json', import.meta.url), 'utf8')) as {
    presets: readonly FixturePreset[];
  }
).presets;

const settingsOf = (key: string): StructureSettings => {
  const preset = PRESETS.find((each) => each.key === key)!;
  return {
    levels: preset.levels.map((type) => ({ type })),
    fields: Object.fromEntries(Object.entries(preset.fields).map(([type, capabilities]) => [type, { capabilities }])),
    capture: preset.capture,
  } as unknown as StructureSettings;
};

/** The installation's profiles: every capability the system matrix gives each type. */
const OFFERED: Readonly<Record<string, readonly string[]>> = {
  TASK: PRESETS.find((each) => each.key === 'PROJECT')!.fields['TASK']!,
  WORK_PACKAGE: PRESETS.find((each) => each.key === 'PROJECT')!.fields['WORK_PACKAGE']!,
  ACTIVITY: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'HISTORY'],
};
const offered = (type: string) => OFFERED[type] ?? [];
const ROOTS = ['TASK'];
const UNITS = ['g', 'kg'];

test('Tasks recognises a day, a person and a label, keeps lines together and reads no amount', () => {
  assert.deepEqual(rulesOf(settingsOf('TASKS'), ROOTS, offered), { date: true, person: true, label: true });
  assert.deepEqual(captureRulesOf({ settings: settingsOf('TASKS') }, 'TASK', UNITS), { splitsLines: false, readsAmount: false, units: UNITS });
});

test('a List takes no date, person or label, and reads lines and amounts (UC-WRK-26 check 3)', () => {
  assert.deepEqual(rulesOf(settingsOf('LIST'), ROOTS, offered), { date: false, person: false, label: false });
  assert.deepEqual(captureRulesOf({ settings: settingsOf('LIST') }, 'TASK', UNITS), { splitsLines: true, readsAmount: true, units: UNITS });
});

test('Flow has one level, so lines split there too - the rule both capture fields share', () => {
  assert.equal(captureRulesOf({ settings: settingsOf('FLOW') }, 'TASK', UNITS).splitsLines, true);
});

test('capture holds DATE but the top level lacks DUE_DATE: no date', () => {
  const narrowed = settingsOf('TASKS');
  const fields = { ...narrowed.fields, TASK: { capabilities: ['COMPLETION', 'ASSIGNMENT', 'LABELS'] } };
  assert.equal(rulesOf({ ...narrowed, fields } as StructureSettings, ROOTS, offered).date, false);
});

test('a type the fields leave out carries what the installation offers it', () => {
  const settings = { levels: [{ type: 'TASK' }, { type: 'ACTIVITY' }], fields: {}, capture: ['DATE', 'PERSON', 'LABEL'] } as unknown as StructureSettings;
  assert.deepEqual(rulesOf(settings, ROOTS, offered), { date: true, person: true, label: true });
});

test('no structure answered: Hubtask’s default, the installation’s first root', () => {
  assert.equal(topLevelOf(undefined, ROOTS), 'TASK');
  assert.deepEqual(rulesOf(undefined, ROOTS, offered), { date: true, person: true, label: true });
});

test('only a collection recognises anything: the jumble, pending and nowhere read nothing', () => {
  const rules = rulesOf(settingsOf('TASKS'), ROOTS, offered);
  const lists = { people: [{ id: 'a', name: 'A' }], labels: [] };
  assert.equal(recognitionOf({ kind: 'jumble' }, rules, lists), NOTHING);
  assert.equal(recognitionOf({ kind: 'pending' }, rules, lists), NOTHING);
  assert.equal(recognitionOf({ kind: 'nowhere' }, rules, lists), NOTHING);
  assert.equal(recognitionOf({ kind: 'collection', collection: KITCHEN }, rules, lists).person, true);
});
