// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The queue, as the frame reads it (F6-06): what `engine.queue()` publishes, bound to runes, and
 * each change spelled as what and where - the kind in the catalogue's words, the entry's title
 * from the copy. The engine's `QueueState` is the fact; this is how a person reads it.
 */

import type { ConflictRecord, MutationKind, QueueState, RejectedMutation, StoredRecord, SyncMutation } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { t } from '../i18n/i18n.svelte.ts';

/** The message code for a mutation kind, and for a patch the field that moved. */
export function whatOf(kind: MutationKind, mutation?: SyncMutation): string {
  if (kind === 'ITEM_PATCH' && mutation?.fields) {
    const fields = Object.keys(mutation.fields);
    const field = fields[0];
    if (fields.length === 1 && field) {
      const known = ['title', 'notes', 'completion', 'assignee_id', 'due_at', 'bucket_id', 'order_key'];
      const key = known.includes(field) ? field : field.startsWith('custom_fields.') ? 'custom_field' : 'field';
      return t(`app.sync.what.patch.${key}`);
    }
    return t('app.sync.what.patch.fields', { count: fields.length });
  }
  return t(`app.sync.what.${kind.toLowerCase()}`);
}

/** The entry's title from the copy, or its identifier where the copy has none. */
async function whereOf(itemId: string): Promise<string> {
  const held = await engine.storage?.get<StoredRecord<{ title?: string }>>('items', itemId);
  return held?.document.title ?? itemId;
}

export interface ListedChange {
  readonly id: string;
  readonly what: string;
  readonly where: string;
  readonly href: string;
}

export interface ListedRefusal extends ListedChange {
  readonly reason: string;
  readonly record: RejectedMutation;
}

class QueueView {
  #state = $state<QueueState>({ count: 0, pushing: false, rejected: [], conflicts: [] });
  #queued = $state<readonly ListedChange[]>([]);
  #refused = $state<readonly ListedRefusal[]>([]);
  /** The entries a queued change names: what a row marks as on its way. */
  #pendingItems = $state<ReadonlySet<string>>(new Set());

  get count(): number {
    return this.#state.count;
  }

  get oldestAt(): number | undefined {
    return this.#state.oldestAt;
  }

  get isPushing(): boolean {
    return this.#state.pushing;
  }

  get queued(): readonly ListedChange[] {
    return this.#queued;
  }

  get refused(): readonly ListedRefusal[] {
    return this.#refused;
  }

  get conflicts(): readonly ConflictRecord[] {
    return this.#state.conflicts;
  }

  /** Whether a change to this entry is waiting to be sent (F6-06): the row's `pending` mark. */
  isPending(itemId: string): boolean {
    return this.#pendingItems.has(itemId);
  }

  /** The conflict on one entry's notes, where there is one - what the entry's strip shows. */
  conflictOn(itemId: string): ConflictRecord | undefined {
    return this.#state.conflicts.find((conflict) => conflict.itemId === itemId && conflict.field === 'notes');
  }

  start(): () => void {
    return engine.queue((state) => {
      this.#state = state;
      void this.#spell(state);
    });
  }

  async #spell(state: QueueState): Promise<void> {
    const storage = engine.storage;
    const pending = storage ? await storage.all<{ id: string; seq: number; kind: MutationKind; itemId: string; mutation: SyncMutation }>('queue') : [];
    const listed: ListedChange[] = [];
    for (const change of [...pending].sort((a, b) => a.seq - b.seq)) {
      listed.push({ id: change.id, what: whatOf(change.kind, change.mutation), where: await whereOf(change.itemId), href: `/items/${change.itemId}` });
    }
    const refused: ListedRefusal[] = [];
    for (const record of state.rejected) {
      refused.push({
        id: record.id,
        what: whatOf(record.kind),
        where: await whereOf(record.itemId),
        href: `/items/${record.itemId}`,
        reason: t(reasonCodeOf(record.code)),
        record,
      });
    }
    this.#queued = listed;
    this.#refused = refused;
    this.#pendingItems = new Set(pending.map((change) => change.itemId));
  }

  async dismiss(id: string): Promise<void> {
    await engine.dismissRejected(id);
  }

  async dismissConflict(id: string): Promise<void> {
    await engine.dismissConflict(id);
  }
}

/** The two codes §6 and §7 name are rendered by name; any other through the catalogue's floor. */
function reasonCodeOf(code: string): string {
  if (code === 'sync.gone') return 'app.sync.refused.gone';
  if (code === 'forbidden') return 'app.sync.refused.forbidden';
  return 'app.sync.refused.other';
}

export const queue = new QueueView();
