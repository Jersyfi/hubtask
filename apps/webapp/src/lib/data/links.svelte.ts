// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's link types, an entry's links, and the writes that draw and remove them
 * (UC-PLN-01). The reads are `resource()`s at their call sites; this holds the paths and the
 * writes, so every screen invalidates the same set.
 *
 * **A link write touches both ends.** The far end lists the link from its side, so drawing or
 * removing one makes both entries' links stale, and every view's dependencies (`/links:query`) -
 * nothing else: the entries themselves do not change.
 */

import type { EntryLink, LinkType, LinkTypeChange, LinkTypeCreate, LinkTypeList, Reference, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

export * from './links.ts';

const TYPES = '/link-types';

/** Where an entry's links and backlinks are read. */
export const entryLinksPath = (itemId: string) => `/items/${itemId}/links`;

/** The read of the links among a view's entries, a `POST` keyed on its document. */
export const LINKS_AMONG = '/links:query';

/** The read that resolves references for the reader, a `POST` keyed on its document. */
export const RESOLVE = '/references:resolve';

/** The most references one resolve asks for, as the contract bounds it. */
export const RESOLVE_MAX = 100;

/** The most entries one `/links:query` names, as the contract bounds it. */
export const AMONG_MAX = 500;

/** The request a set of references is resolved with, in a stable order so equal sets share a read. */
export function resolveRequest(references: readonly Reference[]) {
  return { path: RESOLVE, body: { references: references.slice(0, RESOLVE_MAX) } };
}

/** The request a view's dependencies are read with; the ids sorted so the same rows share a read. */
export function amongRequest(itemIds: readonly string[]) {
  return { path: LINKS_AMONG, body: { item_ids: [...itemIds].sort().slice(0, AMONG_MAX) } };
}

const touched = (...itemIds: string[]) => [...itemIds.map(entryLinksPath), '/links'];

class LinkTypes {
  #state = $state<ResourceState<LinkTypeList> | undefined>(undefined);

  /** The read itself, for a screen that draws its waiting and its refusal. */
  get state(): ResourceState<LinkTypeList> | undefined {
    return this.#state;
  }

  /** Every type, archived ones included, empty until read. */
  get all(): readonly LinkType[] {
    return this.#state?.status === 'ready' ? this.#state.data.data : [];
  }

  /** Starts the read. **From `untrack`**, for the reason the other stores record. */
  open(): () => void {
    return engine.subscribe<LinkTypeList>({ path: TYPES }, (next) => {
      this.#state = next;
    });
  }

  /** Adds a type of the workspace's own (check 5). */
  create(body: LinkTypeCreate): Promise<LinkType> {
    return engine.mutate<LinkType>('POST', TYPES, body, {
      invalidates: [TYPES],
    });
  }

  /** Renames, reorders, archives or restores one of the workspace's own types. */
  change(type: Pick<LinkType, 'key' | 'version'>, body: LinkTypeChange): Promise<LinkType> {
    return engine.mutate<LinkType>('PATCH', `${TYPES}/${encodeURIComponent(type.key)}`, body, {
      ...(type.version > 0 ? { ifMatch: etagFor(type.version) } : {}),
      invalidates: [TYPES],
    });
  }

  /** Deletes a type no link names; one in use is refused and can only be archived (check 5). */
  async remove(type: Pick<LinkType, 'key'>): Promise<void> {
    await engine.mutate<void>('DELETE', `${TYPES}/${encodeURIComponent(type.key)}`, undefined, {
      invalidates: [TYPES],
    });
  }
}

export const linkTypes = new LinkTypes();

class Links {
  /** Draws a link of a type from one entry to another (check 1); the server judges loops and limits. */
  draw(sourceId: string, targetId: string, type: string): Promise<EntryLink> {
    return engine.mutate<EntryLink>('POST', entryLinksPath(sourceId), { target_id: targetId, type }, {
      invalidates: touched(sourceId, targetId),
    });
  }

  /** Removes a link; whoever may change either end may (check 1). */
  async remove(link: Pick<EntryLink, 'id' | 'source_id' | 'target_id'>): Promise<void> {
    await engine.mutate<void>('DELETE', `/links/${link.id}`, undefined, {
      invalidates: touched(link.source_id, link.target_id),
    });
  }
}

export const links = new Links();
