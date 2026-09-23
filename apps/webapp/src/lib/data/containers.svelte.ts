// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's containers, read **one level at a time** because that is what the API answers.
 *
 * `ListContainers` reads one level of the tree: an empty `parent_id` is *the hubs*, and a named one
 * is that hub's collections. It is not an oversight — the permission question differs between the
 * two levels, and the use case says so: a level under a named hub is a question about that hub and
 * a refusal is a refusal, while the hub level is anchored to nothing and is narrowed to what the
 * actor may see rather than refused.
 *
 * So a hub's collections are read **when the hub is opened**, and a collapsed hub costs nothing.
 * That is the better shape as well as the only one: reading every hub's collections at boot would
 * be one request per hub for rows nobody has asked to see, and it would make the tree's expanded
 * state decorative rather than the thing that decides what is fetched.
 *
 * Every write names what it invalidates, because F2-03's default drops everything the client holds
 * — correct, and enough to make renaming a collection empty every other level as well.
 */

import type { Container, ContainerPage, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';
import { anchorFor } from './rank.ts';
import { ANY_ENTRY_DOCUMENT, ENTRY_LISTS } from './touches.ts';

/** The hub level: the one anchored to nothing. */
const HUBS = '/containers?type=HUB&page_size=200';

/** One hub's collections. */
const collectionsPath = (hubId: string) => `/containers?parent_id=${hubId}&page_size=200`;

/**
 * The same two levels, with what has been put aside in them.
 *
 * `include_archived` defaults to `false`, so the tree above does not hold an archived hub or
 * collection at all — it leaves the navigation and nothing shows it again (issue 933). These are
 * the archive screen's own reads, kept apart from the tree's: a tree that carried archived rows
 * would put a read-only container back among the ones somebody works in, which is the opposite of
 * what archiving is for.
 */
const ARCHIVED_HUBS = '/containers?type=HUB&include_archived=true&page_size=200';
const archivedCollectionsPath = (hubId: string) => `/containers?parent_id=${hubId}&include_archived=true&page_size=200`;

/** What a write makes stale: the structure, and nothing else — an entry list is not a container. */
const TOUCHES = ['/containers'];

/** One row of the archive: what was put aside, and the hub it sits in where it is a collection. */
export interface ArchivedContainer {
  readonly container: Container;
  readonly parent?: Container;
}

const rowsOf = (state: ResourceState<ContainerPage>): readonly Container[] =>
  state.status === 'ready' ? (state.data.data ?? []) : [];

class Containers {
  #hubs = $state<ResourceState<ContainerPage>>({ status: 'idle' });
  /** One entry per opened hub. A hub nobody opened is not in here and was never requested. */
  #levels = $state<Record<string, ResourceState<ContainerPage>>>({});
  /** Single containers read by id, for a deep link that arrived before any level was loaded. */
  #single = $state<Record<string, ResourceState<Container>>>({});
  /** The archive screen's own two levels, which include what has been put aside. */
  #archivedHubs = $state<ResourceState<ContainerPage>>({ status: 'idle' });
  #archivedLevels = $state<Record<string, ResourceState<ContainerPage>>>({});

  get hubsState(): ResourceState<ContainerPage> {
    return this.#hubs;
  }

  get hubs(): readonly Container[] {
    return rowsOf(this.#hubs);
  }

  /** Whether the workspace has no hubs — which is not the same as none matching a filter. */
  get hasNoHubs(): boolean {
    return this.#hubs.status === 'ready' && this.hubs.length === 0;
  }

  /** One hub's collections, empty until that hub has been opened. */
  collectionsOf(hubId: string): readonly Container[] {
    const state = this.#levels[hubId];
    return state ? rowsOf(state) : [];
  }

  isLevelLoading(hubId: string): boolean {
    const status = this.#levels[hubId]?.status;
    return status === undefined || status === 'idle' || status === 'loading';
  }

  /**
   * A container by id, from whatever is already loaded and otherwise from its own read.
   *
   * The deep link is why the last part exists: `/collections/{id}` may be the first thing the
   * client ever asks for, and the hub it sits in is then not loaded either. Looking only in the
   * levels would render "not in this workspace" for a container that is simply not fetched yet.
   */
  find(id: string): Container | undefined {
    const inHubs = this.hubs.find((container) => container.id === id);
    if (inHubs) return inHubs;

    for (const state of Object.values(this.#levels)) {
      const found = rowsOf(state).find((container) => container.id === id);
      if (found) return found;
    }
    const single = this.#single[id];
    return single?.status === 'ready' ? single.data : undefined;
  }

  /** Whether a single read for this id has settled, so a view can tell "missing" from "not yet". */
  isSettled(id: string): boolean {
    if (this.find(id)) return true;
    return this.#single[id]?.status === 'failed' || this.#hubs.status === 'failed';
  }

  /** Starts the hub level. What the frame calls once a session exists. */
  start(): () => void {
    return engine.subscribe<ContainerPage>({ path: HUBS }, (next) => {
      this.#hubs = next;
    });
  }

  /**
   * Starts one hub's collections. What the tree calls when a hub is opened.
   *
   * **Call this from `untrack`.** The listener writes `#levels`, and writing it reads it — the
   * spread below is a read — so an `$effect` that subscribes *and* is tracking that read re-runs
   * on its own first delivery, and its cleanup unsubscribes the listener before the answer
   * arrives. The symptom is a level that fetches with a 200 and never appears.
   */
  openLevel(hubId: string): () => void {
    return engine.subscribe<ContainerPage>({ path: collectionsPath(hubId) }, (next) => {
      this.#levels = { ...this.#levels, [hubId]: next };
    });
  }

  /**
   * Starts the archive's reads: the hubs including the archived ones, and every hub's collections
   * the same way.
   *
   * One request per hub, which is what the API's two levels cost and what the tree already pays
   * for the hubs somebody opens. The screen that calls this is not one anybody keeps open.
   */
  openArchive(): () => void {
    const stops: (() => void)[] = [
      engine.subscribe<ContainerPage>({ path: ARCHIVED_HUBS }, (next) => {
        this.#archivedHubs = next;
        for (const hub of rowsOf(next)) {
          if (this.#archivedLevels[hub.id] !== undefined) continue;
          this.#archivedLevels = { ...this.#archivedLevels, [hub.id]: { status: 'idle' } };
          stops.push(
            engine.subscribe<ContainerPage>({ path: archivedCollectionsPath(hub.id) }, (level) => {
              this.#archivedLevels = { ...this.#archivedLevels, [hub.id]: level };
            }),
          );
        }
      }),
    ];
    return () => {
      for (const stop of stops) stop();
      this.#archivedHubs = { status: 'idle' };
      this.#archivedLevels = {};
    };
  }

  /** What has been put aside: the archived containers, each with the hub it sits in. */
  get archived(): readonly ArchivedContainer[] {
    const hubs = rowsOf(this.#archivedHubs);
    const rows: ArchivedContainer[] = hubs.filter((hub) => hub.archived_at).map((container) => ({ container }));
    for (const hub of hubs) {
      for (const collection of rowsOf(this.#archivedLevels[hub.id] ?? { status: 'idle' })) {
        if (collection.archived_at) rows.push({ container: collection, parent: hub });
      }
    }
    return rows;
  }

  /** Whether the archive's reads have all settled, so the screen can tell "none" from "not yet". */
  get isArchiveReady(): boolean {
    if (this.#archivedHubs.status !== 'ready') return false;
    return rowsOf(this.#archivedHubs).every((hub) => {
      const status = this.#archivedLevels[hub.id]?.status;
      return status === 'ready' || status === 'failed';
    });
  }

  get archiveState(): ResourceState<ContainerPage> {
    return this.#archivedHubs;
  }

  /** Starts one container's own read, for a deep link that named it. From `untrack`, as above. */
  openSingle(id: string): () => void {
    return engine.subscribe<Container>({ path: `/containers/${id}` }, (next) => {
      this.#single = { ...this.#single, [id]: next };
    });
  }

  async refresh(): Promise<void> {
    await engine.refresh<ContainerPage>({ path: HUBS });
  }

  /** One hub's collections read again - what an import that landed under it calls (F6-09). */
  async refreshLevel(hubId: string): Promise<void> {
    await engine.refresh<ContainerPage>({ path: collectionsPath(hubId) });
  }

  /** The `Idempotency-Key` is the caller's: pressing "create" twice is one intent, not two hubs. */
  async create(
    body: {
      type: 'HUB' | 'COLLECTION';
      parent_id?: string | null;
      name: string;
      description?: string | null;
      icon?: string | null;
      color_token?: string | null;
    },
    idempotencyKey: string,
  ): Promise<Container> {
    return engine.mutate<Container>('POST', '/containers', body, {
      idempotencyKey,
      invalidates: TOUCHES,
    });
  }

  /**
   * Renames, describes, or recolours one.
   *
   * `ifMatch` is the version the reader had when they started typing, so a rename that lost a race
   * is refused rather than winning by being second (ADR-0025).
   */
  async update(
    id: string,
    body: { name?: string; description?: string | null; icon?: string | null; color_token?: string | null },
    version: number,
  ): Promise<Container> {
    return engine.mutate<Container>('PATCH', `/containers/${id}`, body, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /**
   * Replaces a collection's policies - the way it works, as opposed to what it is called (issue
   * 773). A PUT: the whole document travels, and a key that is not sent falls back to its
   * default. `ifMatch` is the version the reader had when the form opened (ADR-0025).
   */
  async setPolicies(id: string, policies: Container['policies'], version: number): Promise<Container> {
    return engine.mutate<Container>('PUT', `/containers/${id}/policies`, policies, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /**
   * Ranks a container within its own level.
   *
   * A **hub** has no other way: it sits in nothing, so `:move` — whose `target_parent_id` is
   * required — cannot express it. That is what F2-04 added, and this is the client its pull request
   * named as the consumer.
   */
  async reorder(
    id: string,
    siblings: readonly Container[],
    position: number,
    idempotencyKey: string,
  ): Promise<Container> {
    return engine.mutate<Container>(
      'POST',
      `/containers/${id}:reorder`,
      { before_container_id: anchorFor(siblings, id, position) },
      { idempotencyKey, invalidates: TOUCHES },
    );
  }

  /** Moves a collection into another hub. `:move` rather than `:reorder` because the parent changes. */
  async move(
    id: string,
    targetHubId: string,
    beforeContainerId: string | null,
    idempotencyKey: string,
  ): Promise<Container> {
    return engine.mutate<Container>(
      'POST',
      `/containers/${id}:move`,
      { target_parent_id: targetHubId, before_container_id: beforeContainerId },
      { idempotencyKey, invalidates: TOUCHES },
    );
  }

  /**
   * Moves a container and everything under it to the trash.
   *
   * One deletion for the whole subtree: a hub takes its collections and every collection takes its
   * entries, under one `trash_batch_id` "so that restoring is atomic" (I-C2). Anything already in
   * the trash from an earlier deletion keeps that deletion rather than joining this one.
   */
  async trash(id: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', `/containers/${id}`, undefined, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /** Takes one container deletion back out of the trash, whole — the hub, its collections, its entries. */
  async restore(id: string, idempotencyKey: string): Promise<Container> {
    return engine.mutate<Container>('POST', `/containers/${id}:restore`, undefined, {
      idempotencyKey,
      invalidates: ['/containers', ENTRY_LISTS, ANY_ENTRY_DOCUMENT, '/trash'],
    });
  }

  /** Archives or unarchives. Read-only is a state, not a deletion (I-C3). */
  async setArchived(id: string, isArchived: boolean, idempotencyKey: string): Promise<Container> {
    return engine.mutate<Container>(
      'POST',
      `/containers/${id}:${isArchived ? 'archive' : 'unarchive'}`,
      undefined,
      { idempotencyKey, invalidates: TOUCHES },
    );
  }
}

export const containers = new Containers();
