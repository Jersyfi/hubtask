// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a collection offers (UC-WRK-23 check 4): the installation's manifest, narrowed to the
// structure in force - never widened, and nothing offered that the collection hides.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities, StructureSettings } from '@hubtask/sync-engine';

import { allowedChildTypes, capabilityVerdict, rootTypes } from './capability.ts';
import { narrowedTo, offerVerdict, offeredChild, offeredPlacement } from './offer.ts';
import type { Verdict } from './capability.ts';

const codeOf = (verdict: Verdict): string | undefined => (verdict.status === 'refused' ? verdict.code : undefined);

const TASK = ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'MEMBERS', 'BUCKET', 'NOTES', 'LABELS', 'COMMENTS', 'COVER', 'ATTACHMENTS', 'HISTORY', 'RECURRENCE', 'CUSTOM_FIELDS'];
const manifest = {
  item_types: [
    { type: 'TASK', capabilities: TASK, allowed_child_types: ['WORK_PACKAGE', 'ACTIVITY'], max_depth: 3 },
    { type: 'WORK_PACKAGE', capabilities: ['COMPLETION', 'DUE_DATE', 'NOTES'], allowed_child_types: ['ACTIVITY'], max_depth: 2 },
    { type: 'ACTIVITY', capabilities: ['COMPLETION', 'DUE_DATE', 'ASSIGNMENT'], allowed_child_types: [], max_depth: 1 },
  ],
  view_layouts: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'],
} as unknown as Capabilities;

const settings = (levels: string[], fields: Record<string, string[]> = {}, views?: string[]): StructureSettings =>
  ({
    levels: levels.map((type) => ({ type, name: null })),
    fields: Object.fromEntries(Object.entries(fields).map(([type, capabilities]) => [type, { capabilities }])),
    ...(views ? { views: { offered: views, default: views[0] } } : {}),
  }) as unknown as StructureSettings;

/** Hubtask's *List*: one level, a title and completion, the list alone. */
const LIST = settings(['TASK'], { TASK: ['COMPLETION', 'HISTORY', 'CUSTOM_FIELDS'] }, ['LIST_COLLAPSED']);

test('no structure known: the installation answers', () => {
  assert.equal(narrowedTo(manifest, undefined), manifest);
  assert.equal(narrowedTo(undefined, LIST), undefined);
});

test('a list: no date, person, label, notes or child entry, and no board or timeline (UC-WRK-23 check 4)', () => {
  const narrowed = narrowedTo(manifest, LIST);
  for (const capability of ['DUE_DATE', 'ASSIGNMENT', 'MEMBERS', 'LABELS', 'NOTES']) {
    assert.equal(capabilityVerdict(narrowed, 'TASK', capability).status, 'refused', capability);
  }
  assert.equal(capabilityVerdict(narrowed, 'TASK', 'COMPLETION').status, 'permitted');
  assert.deepEqual(allowedChildTypes(narrowed, 'TASK'), []);
  assert.deepEqual(rootTypes(narrowed), ['TASK']);
  assert.deepEqual(narrowed?.view_layouts, ['LIST_COLLAPSED']);
});

test('completion stays, even where the fields leave it out', () => {
  const narrowed = narrowedTo(manifest, settings(['TASK'], { TASK: ['NOTES'] }));
  assert.equal(capabilityVerdict(narrowed, 'TASK', 'COMPLETION').status, 'permitted');
  assert.equal(capabilityVerdict(narrowed, 'TASK', 'NOTES').status, 'permitted');
});

test('a type left out of the fields carries what the installation gives it', () => {
  const narrowed = narrowedTo(manifest, settings(['TASK', 'ACTIVITY'], { TASK: ['COMPLETION'] }));
  assert.equal(capabilityVerdict(narrowed, 'ACTIVITY', 'ASSIGNMENT').status, 'permitted');
  assert.deepEqual(allowedChildTypes(narrowed, 'TASK'), ['ACTIVITY']);
});

test('never wider than the installation: a field it does not carry stays refused', () => {
  const narrowed = narrowedTo(manifest, settings(['TASK', 'ACTIVITY'], { ACTIVITY: ['COMPLETION', 'NOTES'] }));
  assert.equal(capabilityVerdict(narrowed, 'ACTIVITY', 'NOTES').status, 'refused');
});

test('an offered level no offered level reaches is not a level of its own', () => {
  // A work package without the task above it: nothing in the collection can hold it.
  const narrowed = narrowedTo(manifest, settings(['TASK', 'ACTIVITY']));
  assert.deepEqual(rootTypes(narrowed), ['TASK']);
  const orphan = narrowedTo(manifest, { levels: [{ type: 'WORK_PACKAGE', name: null }] } as unknown as StructureSettings);
  assert.deepEqual(orphan?.item_types, []);
});

test('the views it offers, in the installation order', () => {
  assert.deepEqual(narrowedTo(manifest, settings(['TASK'], {}, ['KANBAN', 'LIST_COLLAPSED']))?.view_layouts, ['LIST_COLLAPSED', 'KANBAN']);
  assert.deepEqual(narrowedTo(manifest, settings(['TASK']))?.view_layouts, manifest.view_layouts);
});

test('a hidden field is refused in the words of the server, not as unsupported', () => {
  assert.deepEqual(offerVerdict(manifest, LIST, 'TASK', 'DUE_DATE'), {
    status: 'refused',
    code: 'items.capability_hidden',
    params: { item_type: 'TASK', capability: 'DUE_DATE' },
  });
  assert.equal(codeOf(offerVerdict(manifest, LIST, 'ACTIVITY', 'NOTES')), 'items.capability_not_supported');
  assert.equal(offerVerdict(manifest, LIST, 'TASK', 'COMPLETION').status, 'permitted');
});

test('a hidden level is refused as hidden: as a child and as a place', () => {
  assert.deepEqual(offeredChild(manifest, LIST, 'TASK', 'ACTIVITY'), { status: 'refused', code: 'items.level_hidden', params: { item_type: 'ACTIVITY' } });
  assert.equal(offeredChild(manifest, settings(['TASK', 'ACTIVITY']), 'TASK', 'ACTIVITY').status, 'permitted');
  assert.equal(offeredPlacement(manifest, LIST, 'TASK', null).status, 'permitted');
  assert.equal(codeOf(offeredPlacement(manifest, LIST, 'ACTIVITY', { type: 'TASK', depth: 1 })), 'items.level_hidden');
  assert.equal(offeredPlacement(manifest, LIST, 'TASK', undefined).status, 'undetermined');
});
