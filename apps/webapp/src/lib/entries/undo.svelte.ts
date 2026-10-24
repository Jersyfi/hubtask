// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Offers the way back from an act that took effect at once (UC-WRK-28 check 3), on the frame's
 * toast host: the message says what happened and carries *Undo*; pressing it sends the inverse
 * write. A way back the server refuses - the parent went to the trash meanwhile, another label took
 * the name - says so in a message of its own, and the trash, the archive and the labels dialog stay
 * the other ways back.
 */

import type { BulkOperation, WorkItem } from '@hubtask/sync-engine';

import { ancestorsFirst, sentenceOf, takenBackCode, type Act } from './undo.ts';

import { engine } from '../data/engine.ts';
import { items } from '../data/items.svelte.ts';
import { containers } from '../data/containers.svelte.ts';
import { jumble } from '../data/jumble.svelte.ts';
import { labels } from '../data/labels.svelte.ts';
import { toast } from '../frame/toasts.svelte.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';
import { renderProblem } from '../problem.ts';

/** The inverse write of one act. */
async function takeBack(act: Act): Promise<void> {
  const key = crypto.randomUUID();
  switch (act.kind) {
    case 'trash':
      await items.restore(act.itemId, key);
      return;
    case 'archive':
      await items.setArchived(act.itemId, false, key);
      return;
    case 'label-removed':
      await labels.setOnItem(act.itemId, act.labelId, true, key);
      return;
    case 'label-deleted':
      await labels.restore(act.collectionId, act.labelId, key);
      return;
    case 'jumbled':
      await jumble.dismiss(act.entryId);
      return;
    case 'place-archived':
      await containers.setArchived(act.containerId, false, key);
      return;
    case 'captured':
      if (act.itemIds.length === 1) {
        // Read again first, so the trash states the version the server holds now: a rule that
        // reacted to the new entry must not make the way back fail.
        const id = act.itemIds[0]!;
        const state = await engine.refresh<WorkItem>({ path: `/items/${id}` });
        if (state.status === 'ready') await items.trash(id, state.data.version);
        return;
      }
    // falls through: several entries go back in one bulk
    case 'bulk-trash':
    case 'bulk-label-removed': {
      // All or nothing, as the act was one decision: a selection half back from the trash is a
      // state nobody asked for.
      const operations = act.itemIds.map((itemId) =>
        act.kind === 'bulk-trash'
          ? ({ op: 'RESTORE_ITEM', item_id: itemId, payload: {} } as BulkOperation)
          : act.kind === 'captured'
            ? ({ op: 'TRASH_ITEM', item_id: itemId } as BulkOperation)
            : ({ op: 'ADD_LABEL', item_id: itemId, payload: { label_id: act.labelId } } as BulkOperation),
      );
      const results = await items.bulk(operations, true, key);
      const refused = results.find((result) => result.problem);
      if (refused?.problem) throw refused.problem;
    }
  }
}

/**
 * Says what happened, with *Undo*. `returnTo` is the control that did it, for the focus; `link`
 * leads to what was made, where there is something to open.
 */
export function offerUndo(act: Act, returnTo?: string, link?: { readonly label: string; readonly href: string }): void {
  const sentence = sentenceOf(act);
  toast.show({
    text: t(sentence.code, sentence.params),
    returnTo,
    link,
    undo: async () => {
      try {
        await takeBack(act);
        toast.show({ text: t(takenBackCode(act)), tone: 'success' });
      } catch (error) {
        toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      }
    },
  });
}

/** The ids of a selection about to go to the trash, in the order they come back. */
export function restoreOrder(itemIds: readonly string[]): string[] {
  return ancestorsFirst(itemIds.map((id) => ({ id, depth: items.find(id)?.depth })));
}
