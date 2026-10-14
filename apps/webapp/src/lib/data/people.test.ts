// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Membership } from '@hubtask/sync-engine';

import { candidatesOf, effectiveRole, grantableOf, groupsNamedBy, holdersOf, mayRevoke, membershipsPath, ownershipOf, scopesAlong, scopesInForce } from './people.ts';

const HUB = '11111111-0000-4000-8000-000000000001';
const COLLECTION = '11111111-0000-4000-8000-000000000002';
const ITEM = '11111111-0000-4000-8000-000000000003';
const AMELIE = '22222222-0000-4000-8000-00000000000a';
const JONAS = '22222222-0000-4000-8000-00000000000b';
const FITTERS = '33333333-0000-4000-8000-00000000000f';

function grant(over: Partial<Membership> & Pick<Membership, 'id' | 'scope_type' | 'role'>): Membership {
  return { account_id: null, group_id: null, scope_id: null, ...over } as Membership;
}

test('in a private hub the workspace holds nothing: its roles are not in force (UC-ID-16)', () => {
  const path = { hubId: HUB, collectionId: COLLECTION };
  assert.deepEqual(scopesInForce(path, true), [
    { scopeType: 'HUB', scopeId: HUB },
    { scopeType: 'COLLECTION', scopeId: COLLECTION },
  ]);
  assert.deepEqual(scopesInForce(path, false), scopesAlong(path));
});

test('the path is read from the workspace downwards, and the workspace carries no identifier', () => {
  assert.deepEqual(scopesAlong({ hubId: HUB, collectionId: COLLECTION, itemId: ITEM }), [
    { scopeType: 'TENANT' },
    { scopeType: 'HUB', scopeId: HUB },
    { scopeType: 'COLLECTION', scopeId: COLLECTION },
    { scopeType: 'ITEM', scopeId: ITEM },
  ]);
});

test('a level that does not exist contributes no scope', () => {
  // A hub has no collection above an entry, and asking for one would be asking about nothing.
  assert.deepEqual(scopesAlong({ hubId: HUB }), [
    { scopeType: 'TENANT' },
    { scopeType: 'HUB', scopeId: HUB },
  ]);
  assert.deepEqual(scopesAlong({}), [{ scopeType: 'TENANT' }]);
});

test('the workspace is asked for without a scope identifier', () => {
  // The contract: "omitted for the whole workspace, and only then". An empty `scope_id=` would be
  // a value rather than an absence, and the server reads it as one.
  assert.equal(membershipsPath({ scopeType: 'TENANT' }), '/memberships?scope_type=TENANT');
  assert.equal(
    membershipsPath({ scopeType: 'HUB', scopeId: HUB }),
    `/memberships?scope_type=HUB&scope_id=${HUB}`,
  );
});

test('a person granted at two levels is offered once', () => {
  const memberships = [
    grant({ id: 'm1', scope_type: 'TENANT', role: 'VIEWER', account_id: AMELIE }),
    grant({ id: 'm2', scope_type: 'HUB', scope_id: HUB, role: 'MEMBER', account_id: AMELIE }),
    grant({ id: 'm3', scope_type: 'HUB', scope_id: HUB, role: 'MEMBER', account_id: JONAS }),
  ];
  assert.deepEqual(candidatesOf(memberships, {}), [AMELIE, JONAS]);
});

test('a grant to a group offers the people in it', () => {
  const memberships = [grant({ id: 'm1', scope_type: 'HUB', scope_id: HUB, role: 'MEMBER', group_id: FITTERS })];

  assert.deepEqual(groupsNamedBy(memberships), [FITTERS]);
  // A group is not itself a candidate: an entry is assigned to an account.
  assert.deepEqual(candidatesOf(memberships, { [FITTERS]: [AMELIE, JONAS] }), [AMELIE, JONAS]);
});

test('a group whose members have not arrived yet blocks nobody', () => {
  const memberships = [
    grant({ id: 'm1', scope_type: 'HUB', scope_id: HUB, role: 'MEMBER', group_id: FITTERS }),
    grant({ id: 'm2', scope_type: 'HUB', scope_id: HUB, role: 'MEMBER', account_id: JONAS }),
  ];
  // The picker is usable while the second request is still out, and it grows when it lands.
  assert.deepEqual(candidatesOf(memberships, {}), [JONAS]);
});

test('a role granted higher up is shown, and marked as not revocable here', () => {
  const rows = holdersOf(
    [
      grant({ id: 'm1', scope_type: 'TENANT', role: 'OWNER', account_id: AMELIE }),
      grant({ id: 'm2', scope_type: 'HUB', scope_id: HUB, role: 'MEMBER', account_id: JONAS }),
    ],
    { scopeType: 'HUB', scopeId: HUB },
  );

  // Who is on this hub is a question about effect: a workspace owner is on every hub whether or
  // not anybody granted them anything there.
  assert.equal(rows.length, 2);
  assert.equal(rows[0]?.isHere, false);
  assert.equal(rows[1]?.isHere, true);
});

test('a workspace row is granted here when the workspace is what is being looked at', () => {
  const rows = holdersOf(
    [grant({ id: 'm1', scope_type: 'TENANT', role: 'OWNER', account_id: AMELIE })],
    { scopeType: 'TENANT' },
  );
  // `null` and "absent" are the same scope here, and treating them differently would make the one
  // scope that has no identifier the one scope nobody can revoke at.
  assert.equal(rows[0]?.isHere, true);
});

test('a revocation is somebody else\'s, the reader\'s own, or the reader\'s last', () => {
  const here = { scopeType: 'TENANT' } as const;
  const holders = holdersOf(
    [
      grant({ id: 'm-amelie-admin', scope_type: 'TENANT', role: 'ADMIN', account_id: AMELIE }),
      grant({ id: 'm-amelie-auditor', scope_type: 'TENANT', role: 'AUDITOR', account_id: AMELIE }),
      grant({ id: 'm-jonas', scope_type: 'TENANT', role: 'MEMBER', account_id: JONAS }),
      grant({ id: 'm-fitters', scope_type: 'TENANT', role: 'VIEWER', group_id: FITTERS }),
    ],
    here,
  );
  const nobody = () => [];
  const fitters = (groupId: string) => (groupId === FITTERS ? [JONAS] : []);

  // Somebody else's role, and nobody signed in.
  assert.equal(ownershipOf(holders, 'm-jonas', AMELIE, nobody), 'other');
  assert.equal(ownershipOf(holders, 'm-jonas', undefined, nobody), 'other');
  assert.equal(ownershipOf(holders, 'm-unknown', AMELIE, nobody), 'other');
  // One of two: the other remains.
  assert.equal(ownershipOf(holders, 'm-amelie-admin', AMELIE, nobody), 'own');
  // The only one.
  assert.equal(ownershipOf(holders, 'm-jonas', JONAS, nobody), 'last');
  // A role through a group reaches the reader too - so Jonas keeps one, and the group\'s is his own.
  assert.equal(ownershipOf(holders, 'm-jonas', JONAS, fitters), 'own');
  assert.equal(ownershipOf(holders, 'm-fitters', JONAS, fitters), 'own');
  assert.equal(ownershipOf(holders, 'm-fitters', AMELIE, fitters), 'other');
});

test('a role inherited from above counts as one that remains', () => {
  const holders = holdersOf(
    [
      grant({ id: 'm-workspace', scope_type: 'TENANT', role: 'MEMBER', account_id: AMELIE }),
      grant({ id: 'm-hub', scope_type: 'HUB', scope_id: HUB, role: 'ADMIN', account_id: AMELIE }),
    ],
    { scopeType: 'HUB', scopeId: HUB },
  );
  assert.equal(ownershipOf(holders, 'm-hub', AMELIE, () => []), 'own');
});

test('a form offers only the roles the server says the reader may grant, in the manifest order', () => {
  const installed = ['OWNER', 'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR'] as const;
  assert.deepEqual(grantableOf(installed, ['ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR']), [
    'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR',
  ]);
  assert.deepEqual(grantableOf(installed, [...installed].reverse()), installed);
  assert.deepEqual(grantableOf(installed, []), []);
  // Not yet answered is not "all of them".
  assert.deepEqual(grantableOf(installed, undefined), []);
  // A role the installation does not declare is never offered, whatever the scope said.
  assert.deepEqual(grantableOf(['MEMBER'], ['OWNER', 'MEMBER']), ['MEMBER']);
});

test('a role is revoked only by somebody who may grant it there (domain-model.md §3.2)', () => {
  const belowOwner = ['ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR'] as const;
  assert.equal(mayRevoke('OWNER', belowOwner), false);
  assert.equal(mayRevoke('ADMIN', belowOwner), true);
  assert.equal(mayRevoke('MEMBER', []), false);
  // Not yet answered is not "yes".
  assert.equal(mayRevoke('GUEST', undefined), false);
});

const ORDER = ['OWNER', 'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR'];

test('the effective role is the strongest along the path, as the server decides an entry by', () => {
  const tenant = [grant({ id: 'm0', scope_type: 'TENANT', account_id: AMELIE, role: 'VIEWER' })];
  const hub = [grant({ id: 'm1', scope_type: 'HUB', scope_id: HUB, account_id: AMELIE, role: 'MEMBER' })];
  const item = [grant({ id: 'm2', scope_type: 'ITEM', scope_id: ITEM, account_id: AMELIE, role: 'GUEST' })];
  assert.equal(effectiveRole({ answers: [tenant, hub, item], accountId: AMELIE, groupMembers: {}, order: ORDER }), 'MEMBER');
  assert.equal(effectiveRole({ answers: [tenant, hub, item], accountId: JONAS, groupMembers: {}, order: ORDER }), null);
});

test('a refused scope holds nothing, and a scope not yet answered leaves the role unknown', () => {
  const item = [grant({ id: 'm2', scope_type: 'ITEM', scope_id: ITEM, account_id: AMELIE, role: 'GUEST' })];
  // A guest on one entry: the workspace's and the collection's reads are refused.
  assert.equal(effectiveRole({ answers: ['refused', 'refused', item], accountId: AMELIE, groupMembers: {}, order: ORDER }), 'GUEST');
  assert.equal(effectiveRole({ answers: ['refused', undefined, item], accountId: AMELIE, groupMembers: {}, order: ORDER }), undefined);
  // Before the manifest has said which roles there are, nothing is ordered.
  assert.equal(effectiveRole({ answers: [item], accountId: AMELIE, groupMembers: {}, order: [] }), undefined);
  assert.equal(effectiveRole({ answers: [item], accountId: undefined, groupMembers: {}, order: ORDER }), undefined);
});

test('a role held through a group counts once its members are read, and not before', () => {
  const hub = [
    grant({ id: 'm1', scope_type: 'HUB', scope_id: HUB, group_id: FITTERS, role: 'ADMIN' }),
    grant({ id: 'm3', scope_type: 'HUB', scope_id: HUB, account_id: AMELIE, role: 'VIEWER' }),
  ];
  assert.equal(effectiveRole({ answers: [hub], accountId: AMELIE, groupMembers: {}, order: ORDER }), undefined);
  assert.equal(effectiveRole({ answers: [hub], accountId: AMELIE, groupMembers: { [FITTERS]: [AMELIE] }, order: ORDER }), 'ADMIN');
  // A group that could not be read contributes nobody.
  assert.equal(effectiveRole({ answers: [hub], accountId: AMELIE, groupMembers: { [FITTERS]: [] }, order: ORDER }), 'VIEWER');
});

test('a role this installation does not order counts for nothing', () => {
  const hub = [grant({ id: 'm1', scope_type: 'HUB', scope_id: HUB, account_id: AMELIE, role: 'SUPERUSER' as never })];
  assert.equal(effectiveRole({ answers: [hub], accountId: AMELIE, groupMembers: {}, order: ORDER }), null);
});
