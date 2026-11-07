// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The queue, as the frame reads it: what `engine.queue()` publishes, bound to runes, and
 * each change spelled as what and where - the kind in the catalogue's words, the entry's title
 * from the copy. The engine's `QueueState` is the fact; this is how a person reads it.
 */

import type { ConflictRecord, QueuedKind, QueueState, RejectedMutation, StoredRecord, SyncMutation } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { ENTITY_PLACES, fieldRefusals } from './queueplaces.ts';
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
  if (mutation?.entity && (kind === 'ENTITY_UPSERT' || kind === 'ENTITY_PATCH' || kind === 'ENTITY_DELETE')) {
    return t(`app.sync.what.entity.${mutation.entity}.${kind === 'ENTITY_DELETE' ? 'delete' : 'write'}`);
  }
  return t(`app.sync.what.${kind.toLowerCase()}`);
}

/** The source entry a link mutation draws from, where it names one. */
const linkSourceOf = (mutation?: SyncMutation): string | undefined =>
  mutation?.entity === 'entry_link' && typeof mutation.payload?.item_id === 'string' ? mutation.payload.item_id : undefined;

/** What a change names, as the copy knows it: the entry it hangs off, or the container, label or bucket. */
interface Subject {
  readonly itemId: string;
  readonly kind: QueuedKind;
  readonly entityId?: string;
  readonly mutation?: SyncMutation;
}

/** An entry's title from the copy, or its identifier where the copy has none. */
async function titleOf(itemId: string): Promise<string> {
  const held = await engine.storage?.get<StoredRecord<{ title?: string }>>('items', itemId);
  return held?.document.title ?? itemId;
}

/** For the vocabulary, the name of what was written, or its key; a link names its source entry. */
async function vocabularyWhereOf(itemId: string, mutation: SyncMutation): Promise<string> {
  const vocabulary = mutation.entity ? ENTITY_PLACES[mutation.entity] : undefined;
  if (vocabulary?.collection) {
    const held = await engine.storage?.get<StoredRecord<{ name?: string }>>(vocabulary.collection, itemId);
    return held?.document.name ?? String(mutation.entity_key ?? itemId);
  }
  const source = linkSourceOf(mutation);
  if (mutation.entity === 'entry_link') return source ? titleOf(source) : String(mutation.entity_key ?? itemId);
  return titleOf(itemId);
}

/**
 * Where a change sits: the entry's title and page; for the vocabulary, what was written and its
 * screen; for a container, a label, a bucket or a template applied, the collection's name and page
 * (or the hub's), from the copy - its identifier where the copy has none.
 */
async function whereOf(subject: Subject): Promise<{ where: string; href: string }> {
  const storage = engine.storage;
  if (subject.mutation?.entity) {
    return { where: await vocabularyWhereOf(subject.itemId, subject.mutation), href: hrefOf(subject.itemId, subject.mutation) };
  }
  if (subject.itemId) {
    return { where: await titleOf(subject.itemId), href: `/items/${subject.itemId}` };
  }
  const payload = (subject.mutation?.payload ?? {}) as { collection_id?: string; parent_id?: string };
  const containerId = subject.kind.startsWith('CONTAINER_')
    ? (subject.entityId ?? subject.mutation?.entity_id ?? '')
    : (subject.mutation?.container_id ?? payload.collection_id ?? '');
  const held = await storage?.get<StoredRecord<{ name?: string; type?: string }>>('containers', String(containerId));
  const kind = held?.document.type === 'HUB' ? 'hubs' : 'collections';
  return { where: held?.document.name ?? String(containerId), href: `/${kind}/${containerId}` };
}

/** Where a queued change is opened: its entry, or the screen of the vocabulary it writes. */
function hrefOf(itemId: string, mutation?: SyncMutation): string {
  const vocabulary = mutation?.entity ? ENTITY_PLACES[mutation.entity] : undefined;
  if (vocabulary?.href) return vocabulary.href;
  return `/items/${linkSourceOf(mutation) ?? itemId}`;
}

export interface ListedChange {
  readonly id: string;
  readonly what: string;
  readonly where: string;
  readonly href: string;
}

export interface ListedRefusal extends ListedChange {
  readonly reason: string;
  /** The reason beside each draft field the server named, where the catalogue words it. */
  readonly fieldReasons: Readonly<Record<string, string>>;
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
      // The mutation it was, where the record keeps it: a vocabulary or timer refusal reads as
      // what it is and links to its screen (user-experience.md §5.11).
      refused.push({
        id: record.id,
        what: whatOf(record.kind, record.mutation),
        ...(await whereOf(record)),
        reason: reasonOf(record),
        fieldReasons: fieldReasonsOf(record),
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

  /** Sends a refused change again with what its draft now says. */
  async resend(id: string, values: Readonly<Record<string, unknown>>): Promise<boolean> {
    return engine.resendRejected(id, values);
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
    return t('app.sync.refused.named', { reason: t(record.messageCode, record.params ?? {}) });
  }
  return t('app.sync.refused.other');
}

/** Each named field's reason in words; a code the catalogue lacks stays with the refusal's own. */
function fieldReasonsOf(record: RejectedMutation): Record<string, string> {
  const reasons: Record<string, string> = {};
  for (const [field, refusal] of Object.entries(fieldRefusals(record.fieldErrors))) {
    if (messages.has(refusal.code)) reasons[field] = t(refusal.code, refusal.params);
  }
  return reasons;
}

export const queue = new QueueView();
