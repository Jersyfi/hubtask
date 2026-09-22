// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a change record makes stale, in this application's own paths.
 *
 * **The engine does not learn what a hub is.** It reads `entity`, `entity_id`, `container_id` and
 * `op`, hands them here, and invalidates what comes back — so this function is the one place that
 * knows a comment lives under `/items/{id}/comments` and a label under a container. That split is
 * `packages/sync-engine/CLAUDE.md`'s: the seam is the network, the paths are the product.
 *
 * **A record is a signal to re-read, never data to apply.** Nothing here touches `payload`. Applying
 * it would be a merge, and merging is the server's (ADR-0021, `offline-sync.md` §4) — which is also
 * why a record for something this client does not draw returns nothing at all rather than an
 * invalidation of everything.
 *
 * **The names are `touches.ts`'s**, the same ones a write declares, so the write's answer and the
 * record for it agree on what is stale (issue 877 - `/items` here re-read an entry's thread, its
 * reminders and its series for a retitled entry, and each record of the write did it again).
 * `entity_id` is the record's own entity - a reminder's id on a reminder record, not its entry's -
 * which is why the reminders, the threads and the series are named with a star in place of the
 * entry: the record cannot name it, and only the open entry's are watched.
 *
 * **Two names for an entry, deliberately.** The change log records a work item as `item`
 * (`CreateWorkItem.recordChange`) and the purge records one as `work_item` (`Purge.go`). Both are
 * accepted here rather than one being picked: a client that guessed would go blind for whichever
 * writer it guessed against, and being tolerant of a name is cheaper than being wrong about one.
 */

import type { ChangeRecord } from '@hubtask/sync-engine';

import { ANY_ENTRY_DOCUMENT, ENTRY_LISTS, entryDocument, entryHistory } from './touches.ts';

/** The paths a change to an entry makes stale. */
function itemPaths(record: ChangeRecord): readonly string[] {
  const id = record.entity_id;
  return [
    // Every list and board, the entry's own document, and its history - a change made anywhere
    // is a step somebody may be reading. Not the thread, the reminders or the attachments under
    // it: each of those is an entity with a record of its own.
    ENTRY_LISTS,
    entryDocument(id),
    entryHistory(id),
    // An entry that left for the trash, or came back from it, is a row of that screen too.
    ...(record.op === 'DELETE' ? ['/trash'] : []),
  ];
}

/**
 * What this record makes stale. Empty means "this changes nothing this client is showing".
 *
 * Empty is a real answer and the common one: a record about an entity this version does not draw
 * should cost nothing, and invalidating everything "to be safe" would turn one person's edit into
 * a reload of every screen in the workspace.
 */
export function pathsFor(record: ChangeRecord): readonly string[] {
  const id = record.entity_id;
  const container = record.container_id;

  switch (record.entity) {
    case 'item':
    case 'work_item':
      return itemPaths(record);

    case 'container':
      // The tree - the hubs, and the children of each - and the container's own document, which
      // is a heading and a row in the sidebar. Not the entries under it: a renamed collection
      // changes no entry, and a trashed one takes its entries to the trash under records of
      // their own.
      return ['/containers$', `/containers/${id}$`, ...(record.op === 'DELETE' ? ['/trash'] : [])];

    case 'comment':
      // The thread it is in. `container_id` is the collection, so the entry is not in the record —
      // and a thread is read at `/items/{itemId}/comments`, which this record cannot name.
      // Every open thread is right and cheap: only the open entry's is watched.
      return ['/items/*/comments'];

    case 'label':
      // A label belongs to a collection and is drawn, expanded, on every entry that carries it:
      // the collection's labels, the lists, and every open document.
      return [...(container ? [`/containers/${container}/labels`] : ['/containers']), ENTRY_LISTS, ANY_ENTRY_DOCUMENT];

    case 'bucket':
      // The board's columns, and the lists that group by them.
      return [...(container ? [`/containers/${container}/buckets`] : ['/containers']), ENTRY_LISTS];

    case 'reminder':
      // The record names the reminder, not its entry.
      return ['/items/*/reminders'];

    case 'recurrence_rule':
      // The series, and the entry's own document, which carries `recurrence_rule_id`. The
      // occurrences a series makes arrive as entry records of their own.
      return ['/items/*/recurrence', ANY_ENTRY_DOCUMENT];

    case 'media_object':
      return [`/media/${id}`, '/items/*/attachments'];

    case 'membership':
    case 'group':
      return ['/memberships', '/groups'];

    case 'custom_field':
    case 'custom_field_definition':
      return ['/custom-fields', ENTRY_LISTS, ANY_ENTRY_DOCUMENT];

    case 'saved_view':
      return ['/views'];

    case 'template':
      return ['/templates'];

    case 'account':
      return [`/accounts/${id}`];

    default:
      // A newer server's entity. Nothing here draws it, so nothing here is stale — and a client
      // that reloaded everything on a name it had never met would punish the installation for
      // being ahead of it.
      return [];
  }
}

/**
 * The container a record says access was revoked to, or nothing when it is not that kind.
 *
 * `offline-sync.md` §6 and §9 rule 3 bind a client to **delete what it holds** for a container it
 * lost. This client holds a cache and nothing more, so "delete" is "drop it and stop showing it" —
 * but the obligation is the same one, and pretending a record we cannot act on is only a hint would
 * be reading the rule as advice.
 */
export function revokedContainerOf(record: ChangeRecord): string | undefined {
  if (record.op !== 'ACCESS_REVOKED') return undefined;
  // The container itself is the subject when the entity is a container; otherwise the record names
  // the container the lost thing lived in.
  if (record.entity === 'container') return record.entity_id;
  return record.container_id ?? undefined;
}
