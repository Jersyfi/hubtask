// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What can be taken back, and how (UC-WRK-28 check 3): moving to the trash, archiving, taking a
 * label off an entry and deleting a label from a collection take effect at once and offer *Undo*;
 * so does a capture (UC-WRK-26 check 4), whose way back is the trash, or, for a jumble entry, its
 * dismissal - an entry there is never deleted by hand and never undismissed (UC-JUM-06).
 *
 * Pure: which sentence the message says, and in which order a selection comes back. How long the
 * message stays, that a newer one does not push it out and that *Undo* waits for the network are
 * the frame's toast host's (`lib/frame/toasts.ts`); the writes are `undo.svelte.ts`'s, through the
 * same calls every other write makes - the way back is an ordinary write, never a client-side
 * rollback (offline-sync.md §1).
 */

/** One act that can be taken back. */
export type Act =
  | { readonly kind: 'trash'; readonly itemId: string; readonly title: string }
  | { readonly kind: 'archive'; readonly itemId: string; readonly title: string }
  | { readonly kind: 'label-removed'; readonly itemId: string; readonly labelId: string; readonly title: string }
  | { readonly kind: 'label-deleted'; readonly collectionId: string; readonly labelId: string; readonly name: string }
  | { readonly kind: 'bulk-trash'; readonly itemIds: readonly string[] }
  | { readonly kind: 'bulk-label-removed'; readonly itemIds: readonly string[]; readonly labelId: string }
  | { readonly kind: 'captured'; readonly itemIds: readonly string[]; readonly place: string }
  | { readonly kind: 'jumbled'; readonly entryId: string }
  | { readonly kind: 'place-archived'; readonly containerId: string; readonly name: string };

/** What the message says about the act, as a code and its parameters. */
export function sentenceOf(act: Act): { readonly code: string; readonly params: Record<string, string | number> } {
  switch (act.kind) {
    case 'trash':
      return { code: 'app.undo.trashed', params: { title: act.title } };
    case 'archive':
      return { code: 'app.undo.archived', params: { title: act.title } };
    case 'label-removed':
      return { code: 'app.undo.label_removed', params: { title: act.title } };
    case 'label-deleted':
      return { code: 'app.undo.label_deleted', params: { name: act.name } };
    case 'bulk-trash':
      return { code: 'app.undo.bulk_trashed', params: { count: act.itemIds.length } };
    case 'bulk-label-removed':
      return { code: 'app.undo.bulk_label_removed', params: { count: act.itemIds.length } };
    case 'captured':
      return { code: 'app.capture.added', params: { count: String(act.itemIds.length), place: act.place } };
    case 'jumbled':
      return { code: 'app.capture.jumbled', params: {} };
    case 'place-archived':
      return { code: 'app.undo.archived', params: { title: act.name } };
  }
}

/** What the message says once the way back went through: a dismissed jumble entry is not gone. */
export function takenBackCode(act: Act): string {
  return act.kind === 'jumbled' ? 'app.capture.dismissed' : 'app.undo.done';
}

/**
 * A selection taken back from the trash, ancestors before descendants: a child restored while its
 * parent is still in the trash is refused (`items.parent_trashed`), and in an atomic bulk that would
 * roll the whole way back.
 */
export function ancestorsFirst<T extends { readonly id: string; readonly depth?: number }>(entries: readonly T[]): string[] {
  return [...entries]
    .map((entry, index) => ({ entry, index }))
    .sort((a, b) => (a.entry.depth ?? 1) - (b.entry.depth ?? 1) || a.index - b.index)
    .map(({ entry }) => entry.id);
}
