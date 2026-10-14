// SPDX-License-Identifier: Apache-2.0
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
 * entry, and that refusal is a sentence the reader gets. A picker that tried to be the
 * gate would be a second implementation of an authorisation rule, always one deployment behind.
 */

import type { Membership, MembershipRole, MembershipScope } from '@hubtask/sync-engine';

/**
 * A subject is **shown** with every membership they hold: `AUDITOR` is not a rung on the same
 * ladder - it reads the trail and no content - and "the rights add up rather than the stronger one
 * winning" for what a role carries on the workspace. A client that collapsed them into one badge
 * would be inventing an order to show.
 *
 * What may be done **to one entry** is the exception, and it is the server's own: it takes the
 * strongest role along the entry's path (`service.EffectiveRole`, in the role order the manifest
 * reports) and applies that role's per-entry cell. `effectiveRole` below predicts exactly that.
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

/**
 * The scopes whose memberships are in force along a path. A private hub is reached only through a
 * membership on it or below it (UC-ID-16): a role on the workspace grants nothing there, so it
 * names nobody who may be shown as a member, chosen as an assignee or listed as holding a role.
 */
export function scopesInForce(path: Path, isPrivate: boolean): readonly Scope[] {
  const scopes = scopesAlong(path);
  return isPrivate ? scopes.filter((scope) => scope.scopeType !== 'TENANT') : scopes;
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

/**
 * Whose role a revocation would end, from the reader's side: somebody else's, the reader's own,
 * or the reader's own and last one in force at this place.
 *
 * The distinction is what the confirmation says before the revoke: a role of one's
 * own ends the reader's reach along with the subject's, and the last one ends it entirely -
 * the server allows that, because an owner may hand over and leave, so the client is the half
 * that says what it costs. A role held through a group counts as the reader's own, and a role
 * inherited from above counts as one that remains, because both decide what the reader can still
 * read afterwards.
 */
export type Ownership = 'other' | 'own' | 'last';

export function ownershipOf(
  holders: readonly Holder[],
  membershipId: string,
  me: string | undefined,
  membersOf: (groupId: string) => readonly string[],
): Ownership {
  if (!me) return 'other';
  const reaches = (holder: Holder): boolean =>
    holder.accountId === me || (holder.groupId !== undefined && membersOf(holder.groupId).includes(me));
  const revoked = holders.find((holder) => holder.membershipId === membershipId);
  if (!revoked || !reaches(revoked)) return 'other';
  const remaining = holders.some((holder) => holder.membershipId !== membershipId && reaches(holder));
  return remaining ? 'own' : 'last';
}

/**
 * The roles a form may offer at a scope: the installation's roles, in the manifest's order, that
 * the server says this reader may grant there (`grantable_roles`, domain-model.md §3.2).
 *
 * Both halves, because each says something the other does not: the manifest is which roles exist
 * and how they are ordered, the scope's answer which of them this reader may give. A role in only
 * one of the two is not offered, and before the scope has answered nothing is (P-05): an
 * administrator is never shown *Owner* only to be refused.
 */
export function grantableOf(
  installed: readonly MembershipRole[],
  grantable: readonly MembershipRole[] | undefined,
): readonly MembershipRole[] {
  if (!grantable) return [];
  return installed.filter((role) => grantable.includes(role));
}

/**
 * Whether a membership at a scope may be revoked by this reader: taking a role away needs what
 * handing it out needs (domain-model.md §3.2), so it is the scope's `grantable_roles` that answers.
 * Before the scope has answered, no (P-05): an administrator is never offered to revoke an owner.
 */
export function mayRevoke(role: MembershipRole, grantable: readonly MembershipRole[] | undefined): boolean {
  return grantable?.includes(role) ?? false;
}

/**
 * Whether revoking this row would end the workspace's last owner, which the server refuses
 * (`memberships.last_owner`, domain-model.md §3.2): a person's own `OWNER` on the workspace, the
 * person able to act, and no other `OWNER` row on the workspace at all.
 *
 * `true` only where the refusal is certain from what the screen holds — every row of the workspace
 * read (`complete`), no group holding `OWNER` whose members the screen does not know, no second
 * holder whose standing it cannot judge. Anywhere less certain the control stays and the refusal's
 * sentence says why, rather than a control hidden that the server would have allowed.
 */
export function endsTheLastOwner(
  holders: readonly Holder[],
  membershipId: string,
  complete: boolean,
  canAct: (accountId: string) => boolean,
): boolean {
  if (!complete) return false;
  const revoked = holders.find((holder) => holder.membershipId === membershipId);
  if (!revoked || revoked.role !== 'OWNER' || revoked.scopeType !== 'TENANT' || !revoked.accountId) return false;
  if (!canAct(revoked.accountId)) return false;
  return !holders.some(
    (holder) => holder.membershipId !== membershipId && holder.role === 'OWNER' && holder.scopeType === 'TENANT',
  );
}

/**
 * What the read of one scope's memberships has answered: its rows, `refused` where the server
 * refused the read - which it does exactly where the reader holds no role that reads there, so it
 * holds nothing there - or `undefined` while it has not answered.
 */
export type ScopeAnswer = readonly Membership[] | 'refused' | undefined;

/**
 * The reader's effective role along a path: the strongest role they hold on it, their own or a
 * group's, in the manifest's order (strongest first) - the role the server decides an entry by.
 *
 * Three answers. `undefined` while anything that could change it is unknown: a scope not answered,
 * a group named on the path whose members have not been read, no account, no role order. `null`
 * where everything has answered and the reader holds nothing. Otherwise the role.
 */
export function effectiveRole(input: {
  readonly answers: readonly ScopeAnswer[];
  readonly accountId: string | undefined;
  readonly groupMembers: Readonly<Record<string, readonly string[]>>;
  readonly order: readonly string[];
}): string | null | undefined {
  const { answers, accountId, groupMembers, order } = input;
  if (!accountId || order.length === 0 || answers.some((answer) => answer === undefined)) return undefined;
  const memberships = answers.flatMap((answer) => (Array.isArray(answer) ? answer : []));
  if (groupsNamedBy(memberships).some((group) => groupMembers[group] === undefined)) return undefined;

  let best: number | undefined;
  for (const membership of memberships) {
    const reaches =
      membership.account_id === accountId ||
      (membership.group_id ? (groupMembers[membership.group_id] ?? []).includes(accountId) : false);
    if (!reaches) continue;
    const rank = order.indexOf(membership.role);
    if (rank >= 0 && (best === undefined || rank < best)) best = rank;
  }
  return best === undefined ? null : (order[best] ?? null);
}
