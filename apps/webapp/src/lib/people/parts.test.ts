// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities } from '@hubtask/sync-engine';

import { capabilityVerdict } from '../data/capability.ts';
import { partOf } from './parts.ts';

const manifest = {
  item_types: [
    { type: 'TASK', capabilities: ['ASSIGNMENT', 'MEMBERS'], allowed_child_types: [], max_depth: 3 },
    { type: 'ACTIVITY', capabilities: ['ASSIGNMENT'], allowed_child_types: [], max_depth: 1 },
  ],
} as unknown as Capabilities;

test('a part the type carries is drawn', () => {
  assert.equal(partOf(capabilityVerdict(manifest, 'TASK', 'MEMBERS')), 'drawn');
  assert.equal(partOf(capabilityVerdict(manifest, 'ACTIVITY', 'ASSIGNMENT')), 'drawn');
});

test('a part the type refuses is absent, not drawn with its reason', () => {
  assert.equal(partOf(capabilityVerdict(manifest, 'ACTIVITY', 'MEMBERS')), 'absent');
});

test('before the manifest is read a part waits rather than being guessed', () => {
  assert.equal(partOf(capabilityVerdict(undefined, 'ACTIVITY', 'MEMBERS')), 'deciding');
});
