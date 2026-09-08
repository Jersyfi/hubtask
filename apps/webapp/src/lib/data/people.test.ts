// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Membership } from '@hubtask/sync-engine';

import { candidatesOf, groupsNamedBy, holdersOf, membershipsPath, scopesAlong } from './people.ts';

const HUB = '11111111-0000-4000-8000-000000000001';
const COLLECTION = '11111111-0000-4000-8000-000000000002';
const ITEM = '11111111-0000-4000-8000-000000000003';
const AMELIE = '22222222-0000-4000-8000-00000000000a';
const JONAS = '22222222-0000-4000-8000-00000000000b';
const FITTERS = '33333333-0000-4000-8000-00000000000f';

function grant(over: Partial<Membership> & Pick<Membership, 'id' | 'scope_type' | 'role'>): Membership {
  return { account_id: null, group_id: null, scope_id: null, ...over } as Membership;
}

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
