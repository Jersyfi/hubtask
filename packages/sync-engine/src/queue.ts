// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The mutation queue (offline-sync.md §3.2, §9): what a device did while the server could not be
// reached, in the order it did it, held in the store until the server has answered each one.
//
// Three things are decided here and nowhere else. **The prediction**: a queued mutation is applied
// to the replica at once, marked `pending`, so that the screen shows what the person did - and it
// is a prediction, not a merge: the server's answer overwrites it whole (§9.5). **The answer**:
// `APPLIED` and `MERGED` write the server's state over the prediction; `CONFLICT` writes the
// server's state and keeps both values beside the entry for the resolver to show (§5);
// `REJECTED` takes the mutation out of the queue and keeps it, with its code, until the person
// dismisses it - `sync.gone` (§7) and `forbidden` (§6) by name. **The order**: what was made
// first is pushed first, and a push that fails to reach the server leaves the queue as it was.

import type { Storage } from './ports.ts';
import { collectionOf, Replica, META } from './replica.ts';
import type {
  ChangeRecord, ConflictRecord, MutationKind, PendingMutation, QueuedWrite, RejectedMutation, StoredRecord,
  SyncMutation, SyncMutationResult,
} from './schema.ts';

export const QUEUE = 'queue';
export const REJECTED = 'rejected';
export const CONFLICTS = 'conflicts';

/** The next sequence number, kept in the store so a reload continues the order. */
async function nextSeq(storage: Storage): Promise<number> {
  const held = await storage.get<{ seq: number }>(META, 'queue');
  const seq = (held?.seq ?? 0) + 1;
  await storage.put(META, 'queue', { seq });
  return seq;
}

/** The wire shape of a queued write, its fields stamped: one clock per field (§4.2). */
export function mutationOf(write: QueuedWrite, opId: string, stamp: () => string): SyncMutation {
  const mutation: Record<string, unknown> = { op_id: opId, kind: write.kind, item_id: write.itemId };
  if (write.baseVersion !== undefined) mutation.base_version = write.baseVersion;
  switch (write.kind) {
    case 'ITEM_PATCH': {
      const fields: Record<string, { value: unknown; hlc: string }> = {};
      for (const [field, value] of Object.entries(write.fields ?? {})) fields[field] = { value, hlc: stamp() };
      mutation.fields = fields;
      break;
    }
    case 'SET_ADD':
    case 'SET_REMOVE':
      mutation.set = write.set;
      mutation.element = write.element;
      mutation.hlc = stamp();
      break;
    case 'ITEM_CREATE':
    case 'COMMENT_ADD':
    case 'MOVE':
      mutation.payload = { ...(write.payload ?? {}) };
      mutation.hlc = stamp();
      break;
    case 'ITEM_DELETE':
      mutation.hlc = stamp();
      break;
  }
  return mutation as SyncMutation;
}

export class Queue {
  readonly #replica: Replica;
  readonly #storage: Storage;

  constructor(replica: Replica) {
    this.#replica = replica;
    this.#storage = replica.store;
  }

  /** Everything waiting, oldest first. */
  async pending(): Promise<PendingMutation[]> {
    const held = await this.#storage.all<PendingMutation>(QUEUE);
    return [...held].sort((a, b) => a.seq - b.seq);
  }

  async rejected(): Promise<RejectedMutation[]> {
    const held = await this.#storage.all<RejectedMutation>(REJECTED);
    return [...held].sort((a, b) => a.at - b.at);
  }

  async conflicts(): Promise<ConflictRecord[]> {
    const held = await this.#storage.all<ConflictRecord>(CONFLICTS);
    return [...held].sort((a, b) => a.at - b.at);
  }

  /** Queues a write: the mutation into the store, and its prediction into the replica. */
  async enqueue(write: QueuedWrite, mutation: SyncMutation, at: number): Promise<PendingMutation> {
    const pending: PendingMutation = {
      id: String(mutation.op_id),
      seq: await nextSeq(this.#storage),
      kind: write.kind,
      itemId: write.itemId,
      mutation,
      ...(write.invalidates ? { invalidates: write.invalidates } : {}),
      queuedAt: at,
      attempts: 0,
    };
    await this.#storage.put(QUEUE, pending.id, pending);
    await this.predict(write, mutation, at);
    return pending;
  }

  /**
   * The prediction: what the replica would hold if the server applied this as sent. Marked
   * `pending` on the record, and overwritten whole by the server's answer.
   */
  async predict(write: QueuedWrite, mutation: SyncMutation, at: number): Promise<void> {
    const mark = async (collection: string, id: string) => {
      const held = await this.#storage.get<StoredRecord<Record<string, unknown>>>(collection, id);
      if (held) await this.#storage.put(collection, id, { ...held, pending: true, seenAt: at });
    };
    switch (write.kind) {
      case 'ITEM_CREATE':
        await this.#replica.apply(record('UPSERT', 'item', write.itemId, {
          ...(write.payload ?? {}), id: write.itemId, version: 0,
          completion: { is_completed: false, completed_at: null, completed_by: null },
        }));
        return mark('items', write.itemId);
      case 'ITEM_PATCH': {
        const fields: Record<string, unknown> = {};
        for (const [field, value] of Object.entries(write.fields ?? {})) {
          if (field === 'completion') {
            const done = typeof value === 'boolean' ? value : (value as { is_completed?: boolean })?.is_completed === true;
            fields.completion = { is_completed: done, completed_at: done ? new Date(at).toISOString() : null, completed_by: null };
          } else if (field.startsWith('custom_fields.')) {
            const held = await this.#storage.get<StoredRecord<Record<string, unknown>>>('items', write.itemId);
            const custom = { ...((held?.document.custom_fields as Record<string, unknown> | undefined) ?? {}) };
            const key = field.slice('custom_fields.'.length);
            if (value === null || value === undefined) delete custom[key];
            else custom[key] = value;
            fields.custom_fields = custom;
          } else {
            fields[field] = value;
          }
        }
        await this.#replica.apply(record('UPSERT', 'item', write.itemId, fields));
        return mark('items', write.itemId);
      }
      case 'SET_ADD':
      case 'SET_REMOVE':
        await this.#replica.apply(record('UPSERT', 'item', write.itemId, {
          set: write.set, element_id: write.element, op: write.kind === 'SET_ADD' ? 'add' : 'remove',
        }));
        return mark('items', write.itemId);
      case 'MOVE':
        await this.#replica.apply(record('UPSERT', 'item', write.itemId, { ...(write.payload ?? {}) }));
        return mark('items', write.itemId);
      case 'ITEM_DELETE':
        return this.#replica.apply(record('DELETE', 'item', write.itemId));
      case 'COMMENT_ADD': {
        const payload = write.payload ?? {};
        const id = String(payload.id ?? mutation.op_id);
        await this.#replica.apply(record('UPSERT', 'comment', id, {
          ...payload, id, item_id: write.itemId, created_at: new Date(at).toISOString(), version: 0,
        }));
        return mark('comments', id);
      }
    }
  }

  /**
   * One result, applied as the server's word (§9.5). Answers the entry the result concerned, so
   * the caller can invalidate what shows it.
   */
  async settle(pending: PendingMutation, result: SyncMutationResult, at: number): Promise<void> {
    await this.#storage.delete(QUEUE, pending.id);
    const entity = pending.kind === 'COMMENT_ADD' ? 'comment' : 'item';
    const entityId = result.entity_id ?? (pending.kind === 'COMMENT_ADD' ? String((pending.mutation.payload as { id?: string } | undefined)?.id ?? pending.id) : pending.itemId);

    switch (result.result) {
      case 'APPLIED':
      case 'MERGED':
        await this.#adopt(entity, entityId, result.server_state ?? null, pending);
        return;
      case 'CONFLICT':
        await this.#adopt(entity, entityId, result.server_state ?? null, pending);
        if (result.conflict?.field) {
          await this.#storage.put<ConflictRecord>(CONFLICTS, pending.id, {
            id: pending.id, itemId: pending.itemId, field: result.conflict.field,
            mine: result.conflict.mine, theirs: result.conflict.theirs,
            ...(result.conflict.preserved_comment_id ? { preservedCommentId: result.conflict.preserved_comment_id } : {}),
            at,
          });
        }
        return;
      case 'REJECTED': {
        const code = result.error?.code ?? 'rejected';
        await this.#storage.put<RejectedMutation>(REJECTED, pending.id, {
          id: pending.id, kind: pending.kind, itemId: pending.itemId, code,
          ...(result.error?.message_code ? { messageCode: result.error.message_code } : {}),
          local: localOf(pending.mutation),
          at,
        });
        if (code === 'sync.gone') {
          // The object was purged (§7): the copy goes with it, and what the person wrote is in
          // the rejected record for safekeeping.
          await this.#replica.apply(record('DELETE', entity, entityId));
          return;
        }
        // Refused for another reason: the prediction was wrong. The server's state, where it
        // named one, replaces it; otherwise the copy is left marked until the next read.
        if (result.server_state) await this.#adopt(entity, entityId, result.server_state, pending);
        else await this.#unmark(entity, entityId);
        return;
      }
      default:
        return;
    }
  }

  /** The server's state over the prediction, and the mark cleared. */
  async #adopt(entity: string, entityId: string, state: Record<string, unknown> | null, pending: PendingMutation): Promise<void> {
    if (state) {
      await this.#replica.apply(record('UPSERT', entity, entityId, { ...state, id: entityId }));
    } else if (pending.kind === 'ITEM_DELETE') {
      await this.#replica.apply(record('DELETE', entity, entityId));
      return;
    }
    await this.#unmark(entity, entityId);
  }

  async #unmark(entity: string, entityId: string): Promise<void> {
    const collection = collectionOf(entity);
    const held = await this.#storage.get<StoredRecord<Record<string, unknown>>>(collection, entityId);
    if (!held?.pending) return;
    const { pending: _pending, ...rest } = held;
    await this.#storage.put(collection, entityId, rest);
  }

  /** A push that never reached the server: the mutations stay, one attempt older. */
  async failed(mutations: readonly PendingMutation[]): Promise<void> {
    for (const pending of mutations) {
      await this.#storage.put(QUEUE, pending.id, { ...pending, attempts: pending.attempts + 1 });
    }
  }

  async dismiss(rejectedId: string): Promise<void> {
    await this.#storage.delete(REJECTED, rejectedId);
  }

  async dismissConflict(conflictId: string): Promise<void> {
    await this.#storage.delete(CONFLICTS, conflictId);
  }
}

/** A change record shaped for the replica's apply: what the server would have sent. */
function record(op: 'UPSERT' | 'DELETE', entity: string, id: string, payload?: Record<string, unknown>): ChangeRecord {
  return { op, entity, entity_id: id, container_id: null, ...(payload ? { payload } : {}) } as ChangeRecord;
}

/** What a mutation carried, for a rejected one to offer back: its fields or its payload. */
function localOf(mutation: SyncMutation): unknown {
  if (mutation.fields) {
    const values: Record<string, unknown> = {};
    for (const [field, change] of Object.entries(mutation.fields)) values[field] = (change as { value?: unknown })?.value;
    return values;
  }
  if (mutation.payload) return mutation.payload;
  if (mutation.set) return { set: mutation.set, element: mutation.element };
  return undefined;
}

export type { MutationKind };
