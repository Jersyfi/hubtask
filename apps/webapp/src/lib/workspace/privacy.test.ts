// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ownsHub, shutOut, type Held } from './privacy.ts';

const hub = 'hub-1';
const memberships: readonly Held[] = [
  { account_id: 'paula', scope_type: 'TENANT', role: 'OWNER' },
  { account_id: 'max', scope_type: 'TENANT', role: 'ADMIN' },
  { account_id: 'kim', scope_type: 'TENANT', role: 'MEMBER' },
  { account_id: 'kim', scope_type: 'HUB', scope_id: hub, role: 'OWNER' },
  { account_id: 'max', scope_type: 'HUB', scope_id: 'another-hub', role: 'OWNER' },
  { group_id: 'family', scope_type: 'TENANT', role: 'VIEWER' },
];

test('only an OWNER on the hub itself owns it, never a role on the workspace', () => {
  assert.equal(ownsHub(memberships, 'kim', hub), true);
  assert.equal(ownsHub(memberships, 'paula', hub), false, 'the workspace owner');
  assert.equal(ownsHub(memberships, 'max', hub), false, 'an owner of another hub');
  assert.equal(ownsHub(memberships, undefined, hub), false);
});

test('who loses it is whoever reads it through the workspace and holds nothing on the hub', () => {
  assert.deepEqual(shutOut(memberships, hub), ['paula', 'max']);
});

test('a person is named once, however many workspace roles they hold', () => {
  assert.deepEqual(
    shutOut([...memberships, { account_id: 'paula', scope_type: 'TENANT', role: 'ADMIN' }], hub),
    ['paula', 'max'],
  );
});
