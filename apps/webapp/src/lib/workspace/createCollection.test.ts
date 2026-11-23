// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The dialog *Create collection* as data (user-experience.md §5.2): what each kind sends.
// UC-WRK-01 check 12 makes *Like its hub* a choice of its own; it still sends no preset.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DESK, LIKE_HUB, SCRUM, collectionRequest, type CollectionDraft } from './createCollection.ts';

const HUB = '01a0e2e0-0000-7000-8000-000000000002';
const DRAFT: CollectionDraft = { parentId: HUB, name: '  Pantry ', description: ' ', symbol: undefined, colour: undefined, kind: LIKE_HUB };

test('Like its hub sends the request without a preset, as nothing chosen did', () => {
  const request = collectionRequest(DRAFT);
  // The body the dialog sent before the kinds were one choice, with nothing pressed.
  assert.deepEqual(request.body, { type: 'COLLECTION', parent_id: HUB, name: 'Pantry', description: null, icon: null, color_token: null });
  assert.equal(request.isDesk, false);
  assert.equal(request.landing('c1'), undefined);
});

test('each preset sends its key, and a saved preset its id', () => {
  for (const key of ['LIST', 'TASKS', 'PROJECT', 'FLOW']) {
    const body = collectionRequest({ ...DRAFT, kind: key }).body;
    assert.equal(body.preset, key);
    assert.equal(body.preset_id, undefined);
  }
  const saved = '01a0e2e0-0000-7000-8000-0000000000aa';
  const body = collectionRequest({ ...DRAFT, kind: saved }).body;
  assert.equal(body.preset_id, saved);
  assert.equal(body.preset, undefined);
});

test('Scrum sends Project and opens on the sprint view; Desk sends Tasks and is shaped after', () => {
  const scrum = collectionRequest({ ...DRAFT, kind: SCRUM });
  assert.equal(scrum.body.preset, 'PROJECT');
  assert.equal(scrum.isDesk, false);
  assert.equal(scrum.landing('c1'), '/collections/c1/sprint');

  const desk = collectionRequest({ ...DRAFT, kind: DESK });
  assert.equal(desk.body.preset, 'TASKS');
  assert.equal(desk.isDesk, true);
  assert.equal(desk.landing('c1'), undefined);
});

test('the description and the look are sent as typed and chosen', () => {
  const body = collectionRequest({ ...DRAFT, description: ' Weekly ', symbol: 'heart', colour: 'red' }).body;
  assert.equal(body.description, 'Weekly');
  assert.equal(body.icon, 'heart');
  assert.equal(body.color_token, 'red');
});
