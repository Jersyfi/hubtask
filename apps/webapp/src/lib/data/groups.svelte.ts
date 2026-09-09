// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's groups: what they are called, who is in them, and the four writes.
 *
 * **Any member may read them, and that is the contract's reasoning rather than a relaxation.** A
 * membership granted to a group is unreadable until the group can be shown as the people it
 * reaches — so `GET /groups` is open to every member, and what it discloses is a name and a set of
 * identifiers that resolve through the same minimal read as everywhere else
 * (`data-protection.md` §9).
 *
 * **Members are set whole.** `PATCH /groups/{id}` takes "the complete membership after the
 * change", which is the contract's shape: adding somebody is sending the list with them in it.
 * That is worth knowing at the call site, because a caller that sent one identifier would empty
 * the group.
 *
 * **Deleting a group takes its grants with it.** What the group granted, nobody holds afterwards —
 * so the screen says that before it offers the button, and this module invalidates the memberships
 * as well as the groups.
 */

import type { Group, GroupDetail, GroupPage, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/groups';
/** A group's grants are memberships, so a write here changes what a members screen shows. */
const TOUCHES = [PATH, '/memberships'];

class Groups {
  #page = $state<ResourceState<GroupPage>>({ status: 'idle' });
  #details = $state<Record<string, GroupDetail>>({});

  get state(): ResourceState<GroupPage> {
    return this.#page;
  }

  /** The groups, or nothing while they are unread. */
  get all(): readonly Group[] {
    return this.#page.status === 'ready' ? (this.#page.data.data ?? []) : [];
  }

  /** One group's name, for a membership row that names a group rather than a person. */
  nameOf(groupId: string): string | undefined {
    return this.all.find((group) => group.id === groupId)?.name;
  }

  /** One group with its members, once it has been read. */
  detail(groupId: string): GroupDetail | undefined {
    return this.#details[groupId];
  }

  /** Starts the listing. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<GroupPage>({ path: PATH }, (next) => {
      this.#page = next;
    });
  }

  /** Reads one group's members. Asked for by the screen that shows them, once per group. */
  async read(groupId: string): Promise<void> {
    const state = await engine.refresh<GroupDetail>({ path: `${PATH}/${groupId}` });
    if (state.status === 'ready') this.#details = { ...this.#details, [groupId]: state.data };
  }

  async create(name: string, description?: string): Promise<Group> {
    return engine.mutate<Group>(
      'POST',
      PATH,
      { name, ...(description ? { description } : {}) },
      { idempotencyKey: crypto.randomUUID(), invalidates: TOUCHES },
    );
  }

  /**
   * Renames a group, changes its description, or sets its members — whichever the caller passes.
   *
   * `members` is the complete list afterwards, not an addition. The caller composes it, because
   * only the caller knows whether it is adding or removing.
   */
  async update(
    groupId: string,
    change: { name?: string; description?: string | null; members?: readonly string[] },
  ): Promise<Group> {
    const group = await engine.mutate<Group>('PATCH', `${PATH}/${groupId}`, change, {
      invalidates: TOUCHES,
    });
    await this.read(groupId);
    return group;
  }

  /** Removes it, and with it every membership it granted. */
  async remove(groupId: string): Promise<void> {
    await engine.mutate('DELETE', `${PATH}/${groupId}`, undefined, { invalidates: TOUCHES });
    const { [groupId]: _gone, ...rest } = this.#details;
    this.#details = rest;
  }
}

export const groups = new Groups();
export { PATH as groupsPath };
