// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Who can be named on an entry, worked out from the memberships along the path it sits on.
 *
 * The server offers `GET /memberships` **at** one scope and deliberately no `effective` listing
 * that walks the path itself: "the client composes that path, because it knows the path it is on".
 * This is that composition, and it is pure so that it can be tested without a server — the rune
 * store beside it does the reading.
 *
 * **A membership applies downwards.** What is in force on an entry is the workspace's grants plus
 * its hub's plus its collection's plus its own, and a role granted higher is not repeated lower.
 * So the candidates are the union of four lists, and a person who appears twice appears once.
 *
 * **A group is expanded.** A grant to a group is a grant to everybody in it (`GET /groups/{id}`
 * answers the members), and a picker that offered the group instead would be offering something an
 * entry cannot be assigned to: `assignee_id` is an account.
 *
 * **This is a courtesy, not the enforcement.** The server refuses an account that cannot see the
 * entry, and that refusal is a sentence the reader gets (F2-07). A picker that tried to be the
 * gate would be a second implementation of an authorisation rule, always one deployment behind.
 */

import type { Membership, MembershipRole, MembershipScope } from '@hubtask/sync-engine';

/**
 * There is no "strongest role" here, and that is the model rather than an omission.
 *
 * `AUDITOR` is explicitly not a rung on the same ladder — it reads the trail and no content —
 * and the contract says what happens to somebody who needs two: "the rights add up rather than
 * the stronger one winning". So a subject is shown with every membership they hold, and a client
 * that collapsed them into one badge would be inventing an order the server does not have.
 */

/** Where an entry sits. Each level that exists contributes one list of memberships. */
export interface Path {
  readonly hubId?: string;
  readonly collectionId?: string;
  readonly itemId?: string;
}

/** One scope to ask about: the type, and the identifier where the type has one. */
export interface Scope {
  readonly scopeType: MembershipScope;
  readonly scopeId?: string;
}

/**
 * The scopes to read, from the workspace downwards.
 *
 * The workspace is always in the list and carries no identifier — the contract says `scope_id` is
 * "omitted for the whole workspace, and only then". The order is the order of authority, which is
 * also the order a reader expects to see the rows in.
 */
export function scopesAlong(path: Path): readonly Scope[] {
  const scopes: Scope[] = [{ scopeType: 'TENANT' }];
  if (path.hubId) scopes.push({ scopeType: 'HUB', scopeId: path.hubId });
  if (path.collectionId) scopes.push({ scopeType: 'COLLECTION', scopeId: path.collectionId });
  if (path.itemId) scopes.push({ scopeType: 'ITEM', scopeId: path.itemId });
  return scopes;
}

/** The query a scope is read with. Built here so the store and its test agree on one string. */
export function membershipsPath(scope: Scope): string {
  const query = new URLSearchParams({ scope_type: scope.scopeType });
  if (scope.scopeId) query.set('scope_id', scope.scopeId);
  return `/memberships?${query.toString()}`;
}

/** The groups named by any of these memberships, each once. */
export function groupsNamedBy(memberships: readonly Membership[]): readonly string[] {
  const groups = new Set<string>();
  for (const membership of memberships) if (membership.group_id) groups.add(membership.group_id);
  return [...groups];
}

/**
 * The accounts that may be named, from the memberships and from the groups they name.
 *
 * A group whose members have not arrived yet contributes nothing rather than blocking the list:
 * the picker is usable while the second request is still out, and it grows when the answer lands.
 */
export function candidatesOf(
  memberships: readonly Membership[],
  groupMembers: Readonly<Record<string, readonly string[]>>,
): readonly string[] {
  const accounts = new Set<string>();
  for (const membership of memberships) {
    if (membership.account_id) accounts.add(membership.account_id);
    if (membership.group_id) {
      for (const member of groupMembers[membership.group_id] ?? []) accounts.add(member);
    }
  }
  return [...accounts];
}

/** One row of the members screen: who, what they hold here, and where it was granted. */
export interface Holder {
  readonly membershipId: string;
  readonly accountId?: string;
  readonly groupId?: string;
  readonly role: MembershipRole;
  readonly scopeType: MembershipScope;
  readonly scopeId?: string;
  /** Whether it was granted at the scope being looked at, rather than inherited from above. */
  readonly isHere: boolean;
}

/**
 * The memberships in force at a scope, as rows, with the ones granted higher up marked.
 *
 * Both are shown, because "who is on this hub" is a question about effect and not about paperwork:
 * a workspace owner is on every hub whether or not anybody granted them anything there. Only the
 * ones granted *here* can be revoked here, which is what `isHere` is for.
 */
export function holdersOf(
  memberships: readonly Membership[],
  here: Scope,
): readonly Holder[] {
  return memberships.map((membership) => ({
    membershipId: membership.id,
    accountId: membership.account_id ?? undefined,
    groupId: membership.group_id ?? undefined,
    role: membership.role,
    scopeType: membership.scope_type,
    scopeId: membership.scope_id ?? undefined,
    isHere:
      membership.scope_type === here.scopeType &&
      (membership.scope_id ?? undefined) === here.scopeId,
  }));
}
