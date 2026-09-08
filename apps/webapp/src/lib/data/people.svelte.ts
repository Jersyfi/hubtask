// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The people a screen may name, and who holds which role where.
 *
 * The composition of the path is in `people.ts`, which is pure and tested; this is the half that
 * reads. It holds two things: the memberships at each scope it has been asked about, and the
 * members of each group those memberships name.
 *
 * **One read per scope, shared.** A hub's members screen and an entry's assignee picker both want
 * the hub's memberships, and they get one request between them because both ask for the same path
 * and the engine keys on it.
 *
 * **A group is read once.** Group membership does not change while somebody chooses an assignee,
 * and re-reading it per picker would be a request per keystroke's worth of nothing.
 */

import type { GroupDetail, Membership, MembershipPage, MembershipRole, MembershipScope } from '@hubtask/sync-engine';

import { accounts } from './accounts.svelte.ts';
import { engine } from './engine.ts';
import { candidatesOf, groupsNamedBy, holdersOf, membershipsPath, scopesAlong, type Holder, type Path, type Scope } from './people.ts';

/** A membership write changes who may be named, and therefore every list composed from one. */
const TOUCHES = ['/memberships'];

class People {
  /** One entry per scope path, so two screens asking the same question share one read. */
  #scopes = $state<Record<string, readonly Membership[]>>({});
  /** The accounts in each group a membership named. */
  #groups = $state<Record<string, readonly string[]>>({});
  #openScopes = new Map<string, () => void>();
  #askedGroups = new Set<string>();

  /**
   * Starts reading every scope along a path, and stops when the caller lets go.
   *
   * Idempotent per scope: a second caller asking for the same path adds a reference rather than a
   * subscription, because the engine already shares the entry and a second subscribe would only
   * add a listener that says the same thing.
   */
  open(path: Path): () => void {
    const scopes = scopesAlong(path);
    for (const scope of scopes) this.#openScope(scope);
    return () => {};
  }

  /** Starts one scope on its own — what the members screen of a hub or a collection needs. */
  openScope(scope: Scope): () => void {
    this.#openScope(scope);
    return () => {};
  }

  /** The memberships in force along a path: every scope's rows, from the workspace downwards. */
  along(path: Path): readonly Membership[] {
    return scopesAlong(path).flatMap((scope) => this.#scopes[membershipsPath(scope)] ?? []);
  }

  /** Who may be named on an entry at this path: the accounts, with the groups expanded. */
  candidates(path: Path): readonly string[] {
    const memberships = this.along(path);
    this.#resolveGroups(memberships);
    const ids = candidatesOf(memberships, this.#groups);
    // The names come from the accounts cache, which is where every other screen gets them.
    accounts.resolve(ids);
    return ids;
  }

  /** The rows of a members screen: everything in force here, with what was granted here marked. */
  holders(path: Path, here: Scope): readonly Holder[] {
    const memberships = this.along(path);
    this.#resolveGroups(memberships);
    accounts.resolve(memberships.map((membership) => membership.account_id));
    return holdersOf(memberships, here);
  }

  /** The accounts in a group, once it has been read. Empty while the request is still out. */
  membersOf(groupId: string): readonly string[] {
    return this.#groups[groupId] ?? [];
  }

  /**
   * Grants a role at a scope, to an account or to a group.
   *
   * `OWNER` is a privileged action and needs a step-up this client cannot produce until F4 brings
   * sessions (`security.md` §5). The control that offers it is switched off with that reason
   * rather than hidden, so nothing here has to refuse it a second time.
   */
  async grant(subject: { accountId?: string; groupId?: string }, role: MembershipRole, scope: Scope): Promise<void> {
    await engine.mutate('POST', '/memberships', {
      account_id: subject.accountId ?? null,
      group_id: subject.groupId ?? null,
      scope_type: scope.scopeType,
      scope_id: scope.scopeId ?? null,
      role,
    }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: TOUCHES,
    });
    await this.#reread(scope);
  }

  /** Revokes one grant. Only a grant made *here* can be revoked here; the row says which. */
  async revoke(membershipId: string, scope: Scope): Promise<void> {
    await engine.mutate('DELETE', `/memberships/${membershipId}`, undefined, { invalidates: TOUCHES });
    await this.#reread(scope);
  }

  #openScope(scope: Scope): void {
    const path = membershipsPath(scope);
    if (this.#openScopes.has(path)) return;
    const stop = engine.subscribe<MembershipPage>({ path }, (next) => {
      if (next.status !== 'ready') return;
      this.#scopes = { ...this.#scopes, [path]: next.data.data ?? [] };
    });
    this.#openScopes.set(path, stop);
  }

  /**
   * Re-reads one scope after a write.
   *
   * The engine's invalidation already re-reads what somebody is watching, and this waits for it so
   * that a caller can await the write and then draw the list. Without it the screen would redraw
   * one turn later, which reads as a control that did nothing.
   */
  async #reread(scope: Scope): Promise<void> {
    const path = membershipsPath(scope);
    const state = await engine.refresh<MembershipPage>({ path });
    if (state.status === 'ready') this.#scopes = { ...this.#scopes, [path]: state.data.data ?? [] };
  }

  /** Asks for the members of every group these memberships name, each group once per session. */
  #resolveGroups(memberships: readonly Membership[]): void {
    for (const groupId of groupsNamedBy(memberships)) {
      if (this.#askedGroups.has(groupId)) continue;
      this.#askedGroups.add(groupId);
      void this.#readGroup(groupId);
    }
  }

  async #readGroup(groupId: string): Promise<void> {
    try {
      const state = await engine.refresh<GroupDetail>({ path: `/groups/${groupId}` });
      if (state.status === 'ready') {
        this.#groups = { ...this.#groups, [groupId]: state.data.members ?? [] };
      }
    } catch {
      // A group this actor may not read contributes nobody. It is not an error on the screen: the
      // picker is a courtesy, and the server is what refuses an account that cannot see the entry.
    }
  }
}

export const people = new People();
export type { Holder, Path, Scope };
export type { MembershipRole, MembershipScope };
