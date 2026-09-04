// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the installation permits, and the one thing these tests are for: **nothing is compiled in**.
//
// Every case below hands the reader a different manifest and expects a different answer. That is
// the acceptance criterion rather than a convenience — `domain-model.md` §2's extension example (a
// new type `MILESTONE` is a profile entry and no code change) is only true if no answer here is
// reachable without the manifest saying so.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities } from '@hubtask/sync-engine';

import {
  allowedChildTypes,
  capabilityVerdict,
  changeVerdict,
  childVerdict,
  itemAccess,
  permissionVerdict,
  rootTypes,
} from './capability.ts';

/** The three types of `0.2.0`, as the capability matrix in domain-model.md §2 has them. */
const manifest = {
  item_types: [
    {
      type: 'TASK',
      capabilities: ['COMPLETION', 'BUCKET', 'LABELS', 'NOTES', 'COVER'],
      allowed_child_types: ['WORK_PACKAGE'],
      max_depth: 3,
    },
    {
      type: 'WORK_PACKAGE',
      capabilities: ['COMPLETION', 'LABELS', 'NOTES'],
      allowed_child_types: ['ACTIVITY'],
      max_depth: 2,
    },
    { type: 'ACTIVITY', capabilities: ['COMPLETION'], allowed_child_types: [], max_depth: 1 },
  ],
  roles: [
    {
      role: 'ADMIN',
      permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE'],
      item_access: { read: 'ALL', create: 'ALL', change: 'ALL', comment: 'ALL' },
    },
    {
      role: 'CONTRIBUTOR',
      permissions: ['READ', 'WRITE_ITEMS'],
      item_access: { read: 'ALL', create: 'ALL', change: 'ASSIGNED', comment: 'ALL' },
    },
    {
      role: 'GUEST',
      permissions: ['READ'],
      item_access: { read: 'ALL', create: 'NONE', change: 'NONE', comment: 'ALL' },
    },
  ],
} as unknown as Capabilities;

// --- capabilities -----------------------------------------------------------------------------

test('a capability the profile carries is permitted', () => {
  assert.equal(capabilityVerdict(manifest, 'TASK', 'BUCKET').status, 'permitted');
});

test('a capability the profile does not carry is refused, with the code that says which', () => {
  // The BUCKET row of §2: buckets apply to items directly under the collection, so a work package
  // has none. A client that offered the selector anyway would be building a 422.
  const verdict = capabilityVerdict(manifest, 'WORK_PACKAGE', 'BUCKET');
  assert.equal(verdict.status, 'refused');
  assert.equal(verdict.status === 'refused' ? verdict.code : null, 'items.capability_not_supported');
});

test('a type the manifest does not declare is refused, not permitted', () => {
  // Guessing in the permissive direction is what the manifest exists to prevent — the contract
  // says so about the role half in its own words.
  const verdict = capabilityVerdict(manifest, 'MILESTONE', 'COMPLETION');
  assert.equal(verdict.status, 'refused');
  assert.equal(verdict.status === 'refused' ? verdict.code : null, 'items.type_unsupported');
});

test('nothing is knowable before the manifest is read', () => {
  // Not `refused`, and certainly not `permitted`: a control shown as available while the
  // installation has not answered is one that disappears a moment later.
  assert.equal(capabilityVerdict(undefined, 'TASK', 'BUCKET').status, 'undetermined');
  assert.equal(childVerdict(undefined, 'TASK', 'WORK_PACKAGE').status, 'undetermined');
  assert.equal(permissionVerdict(undefined, 'ADMIN', 'READ').status, 'undetermined');
});

test('a new type in the manifest is a new answer, with no code change', () => {
  // §2's own extension example. If this needed a line here, the claim would be false.
  const extended = {
    ...manifest,
    item_types: [
      ...(manifest.item_types ?? []),
      { type: 'MILESTONE', capabilities: ['COMPLETION', 'LABELS'], allowed_child_types: ['TASK'], max_depth: 4 },
    ],
  } as unknown as Capabilities;

  assert.equal(capabilityVerdict(extended, 'MILESTONE', 'LABELS').status, 'permitted');
  assert.equal(capabilityVerdict(extended, 'MILESTONE', 'BUCKET').status, 'refused');
  assert.deepEqual(allowedChildTypes(extended, 'MILESTONE'), ['TASK']);
});

test('a capability withdrawn from the manifest is withdrawn from the surface', () => {
  // A tenant may narrow a system profile (§2). The surface has to narrow with it, which is the
  // other direction of the same test and the one a hard-coded list would fail.
  const narrowed = {
    ...manifest,
    item_types: [{ type: 'TASK', capabilities: ['COMPLETION'], allowed_child_types: [], max_depth: 1 }],
  } as unknown as Capabilities;

  assert.equal(capabilityVerdict(narrowed, 'TASK', 'BUCKET').status, 'refused');
  assert.deepEqual(allowedChildTypes(narrowed, 'TASK'), []);
});

// --- the hierarchy ----------------------------------------------------------------------------

test('a permitted child type at a permitted depth is permitted', () => {
  assert.equal(childVerdict(manifest, 'TASK', 'WORK_PACKAGE', 0).status, 'permitted');
});

test('a child type the parent does not take is refused', () => {
  // I-W1: an activity under a task skips a level, and the profile is what says so.
  const verdict = childVerdict(manifest, 'TASK', 'ACTIVITY', 0);
  assert.equal(verdict.status, 'refused');
  assert.equal(verdict.status === 'refused' ? verdict.code : null, 'items.parent_type_invalid');
});

test('a fourth level under a max_depth of three is refused', () => {
  const verdict = childVerdict(manifest, 'TASK', 'WORK_PACKAGE', 3);
  assert.equal(verdict.status, 'refused');
  assert.equal(verdict.status === 'refused' ? verdict.code : null, 'items.depth_exceeded');
});

test('a type that takes no children takes none', () => {
  assert.equal(childVerdict(manifest, 'ACTIVITY', 'TASK', 0).status, 'refused');
  assert.deepEqual(allowedChildTypes(manifest, 'ACTIVITY'), []);
});

// --- roles ------------------------------------------------------------------------------------

test('a permission the role carries is permitted, and one it does not is refused', () => {
  assert.equal(permissionVerdict(manifest, 'ADMIN', 'STRUCTURE').status, 'permitted');
  assert.equal(permissionVerdict(manifest, 'CONTRIBUTOR', 'STRUCTURE').status, 'refused');
});

test('a role the manifest does not declare is refused', () => {
  const verdict = permissionVerdict(manifest, 'AUDITOR', 'READ');
  assert.equal(verdict.status, 'refused');
  assert.equal(verdict.status === 'refused' ? verdict.code : null, 'app.gate.role_unknown');
});

test('an absent item_access reads as NONE, never as ALL', () => {
  // The contract: "an absent one would leave a client guessing, and guessing wrong in the
  // permissive direction is what this endpoint exists to prevent."
  const partial = { roles: [{ role: 'ODD', permissions: ['READ'] }] } as unknown as Capabilities;
  assert.equal(itemAccess(partial, 'ODD', 'change'), 'NONE');
});

test('ASSIGNED is the cell no permission name can carry', () => {
  // A contributor writes only what is assigned to them. Reading the permission alone would offer
  // an edit control on every row and have half of them refused — which is what the manifest's own
  // description of `roles` warns about.
  assert.equal(permissionVerdict(manifest, 'CONTRIBUTOR', 'WRITE_ITEMS').status, 'permitted');
  assert.equal(changeVerdict(manifest, 'CONTRIBUTOR', true).status, 'permitted');

  const refused = changeVerdict(manifest, 'CONTRIBUTOR', false);
  assert.equal(refused.status, 'refused');
  assert.equal(refused.status === 'refused' ? refused.code : null, 'app.gate.change_assigned_only');
});

test('a guest may comment without being able to change', () => {
  // The other qualifier the same description names.
  assert.equal(itemAccess(manifest, 'GUEST', 'comment'), 'ALL');
  assert.equal(changeVerdict(manifest, 'GUEST', true).status, 'refused');
});

test('an admin changes anything, assigned or not', () => {
  assert.equal(changeVerdict(manifest, 'ADMIN', false).status, 'permitted');
});

test('every code a prediction uses is in the catalogue', async () => {
  // The Definition of Done's "message codes are in locales/en.json", checked rather than
  // remembered — and it is what makes reusing the server's codes provable rather than claimed: a
  // gate that invented `capability.not_supported` beside `items.capability_not_supported` would
  // fail here, because the invented one is in no catalogue.
  const { SOURCE } = await import('../i18n/catalogue.ts');
  const known = new Set(Object.keys(SOURCE));

  const verdicts = [
    capabilityVerdict(manifest, 'WORK_PACKAGE', 'BUCKET'),
    capabilityVerdict(manifest, 'MILESTONE', 'COMPLETION'),
    childVerdict(manifest, 'TASK', 'ACTIVITY', 0),
    childVerdict(manifest, 'TASK', 'WORK_PACKAGE', 3),
    permissionVerdict(manifest, 'AUDITOR', 'READ'),
    permissionVerdict(manifest, 'CONTRIBUTOR', 'STRUCTURE'),
    changeVerdict(manifest, 'GUEST', true),
    changeVerdict(manifest, 'CONTRIBUTOR', false),
  ];

  for (const verdict of verdicts) {
    assert.equal(verdict.status, 'refused');
    if (verdict.status !== 'refused') continue;
    assert.ok(known.has(verdict.code), `${verdict.code} is in no catalogue`);
  }
});

test('the roots are the types nothing else claims as a child', () => {
  // A collection takes what no other type takes. Derived rather than named, which is what makes
  // §2's extension example true — a list of three here would be wrong on an installation with a
  // fourth, and this asserts exactly that.
  assert.deepEqual(rootTypes(manifest), ['TASK']);

  const withMilestone = {
    ...manifest,
    item_types: [
      { type: 'MILESTONE', capabilities: [], allowed_child_types: ['TASK'], max_depth: 4 },
      ...(manifest.item_types ?? []),
    ],
  } as unknown as Capabilities;
  // TASK is now claimed by MILESTONE, so the root moves — with no change here.
  assert.deepEqual(rootTypes(withMilestone), ['MILESTONE']);
});

test('an unread manifest has no roots rather than a guess', () => {
  assert.deepEqual(rootTypes(undefined), []);
});
