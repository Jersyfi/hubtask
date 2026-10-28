// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The queue, as the frame reads it: what `engine.queue()` publishes, bound to runes, and
 * each change spelled as what and where - the kind in the catalogue's words, the entry's title
 * from the copy. The engine's `QueueState` is the fact; this is how a person reads it.
 */

import type { ConflictRecord, QueuedKind, QueueState, RejectedMutation, StoredRecord, SyncMutation } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';

/** The message code for a mutation kind, and for a patch the field that moved. */
export function whatOf(kind: QueuedKind, mutation?: SyncMutation): string {
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

/** What a change names, as the copy knows it: the entry it hangs off, or the container, label or bucket. */
interface Subject {
  readonly itemId: string;
  readonly kind: QueuedKind;
  readonly entityId?: string;
  readonly mutation?: SyncMutation;
}

/**
 * Where a change sits: the entry's title and page; for a container, a label, a bucket or a template
 * applied, the collection's name and page (or the hub's), from the copy - its identifier where the
 * copy has none.
 */
async function whereOf(subject: Subject): Promise<{ where: string; href: string }> {
  const storage = engine.storage;
  if (subject.itemId) {
    const held = await storage?.get<StoredRecord<{ title?: string }>>('items', subject.itemId);
    return { where: held?.document.title ?? subject.itemId, href: `/items/${subject.itemId}` };
  }
  const payload = (subject.mutation?.payload ?? {}) as { collection_id?: string; parent_id?: string };
  const containerId = subject.kind.startsWith('CONTAINER_')
    ? (subject.entityId ?? subject.mutation?.entity_id ?? '')
    : (subject.mutation?.container_id ?? payload.collection_id ?? '');
  const held = await storage?.get<StoredRecord<{ name?: string; type?: string }>>('containers', String(containerId));
  const kind = held?.document.type === 'HUB' ? 'hubs' : 'collections';
  return { where: held?.document.name ?? String(containerId), href: `/${kind}/${containerId}` };
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

  /** Whether a change to this entry is waiting to be sent: the row's `pending` mark. */
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
    const pending = storage ? await storage.all<{ id: string; seq: number; kind: QueuedKind; itemId: string; entityId?: string; mutation: SyncMutation }>('queue') : [];
    const listed: ListedChange[] = [];
    for (const change of [...pending].sort((a, b) => a.seq - b.seq)) {
      listed.push({ id: change.id, what: whatOf(change.kind, change.mutation), ...(await whereOf(change)) });
    }
    const refused: ListedRefusal[] = [];
    for (const record of state.rejected) {
      refused.push({
        id: record.id,
        what: whatOf(record.kind),
        ...(await whereOf(record)),
        reason: reasonOf(record),
        record,
      });
    }
    this.#queued = listed;
    this.#refused = refused;
    this.#pendingItems = new Set(pending.map((change) => change.itemId).filter((id) => id !== ''));
  }

  async dismiss(id: string): Promise<void> {
    await engine.dismissRejected(id);
  }

  async dismissConflict(id: string): Promise<void> {
    await engine.dismissConflict(id);
  }
}

/**
 * The sentence for a refusal. The two codes §6 and §7 name are rendered by name; any other
 * through the server's own message code where the catalogue has it - a validation the applier
 * answered (`items.collection_or_parent_required`) reads as what it is rather than as "not
 * accepted" - and through the floor where it does not.
 */
function reasonOf(record: RejectedMutation): string {
  const code = record.messageCode ?? record.code;
  if (code === 'sync.gone') return t('app.sync.refused.gone');
  if (code === 'access.forbidden' || record.code === 'forbidden') return t('app.sync.refused.forbidden');
  if (code === 'sync.device_revoked') return t('app.sync.refused.device_revoked');
  if (record.messageCode && messages.has(record.messageCode)) {
    return t('app.sync.refused.named', { reason: t(record.messageCode) });
  }
  return t('app.sync.refused.other');
}

export const queue = new QueueView();
