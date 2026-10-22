// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What Enter writes, and what *Undo* writes back (UC-WRK-26 check 4; record D7, D9). Pure over a
 * `Writer`, which the field binds to the stores: the tests drive it with a fake and see every
 * request it would make.
 *
 * **One key per Enter.** The idempotency key belongs to the intent - this Enter - and a retry of
 * it reuses the key, so a create that reached the server once is never made twice.
 *
 * **Undo is the inverse write, never a rollback.** An entry goes to the trash, read again first so
 * the trash states the version the server holds *now*: a rule that reacted to the new entry must
 * not make the way back fail. Several entries go in one bulk, all or nothing. A jumble capture is
 * dismissed - an entry there is never deleted by hand and never undismissed (UC-JUM-06), so the
 * message after it says *Dismissed*, not that it is gone. A notification the assignment already
 * sent is not called back.
 */

import type { BulkOperation, WorkItem } from '@hubtask/sync-engine';

import type { JumbleEntry } from '../data/jumble.svelte.ts';

import { dueInputFor } from '../data/due.ts';
import { dueOf, type Reading } from './parse.ts';

/** How many entries one Enter makes at most; the rest stays in the field. */
export const MAX_LINES = 100;

/** The stores, as far as a capture writes to them. */
export interface Writer {
  createItem(body: CreateBody, idempotencyKey: string): Promise<WorkItem>;
  submitJumble(subject: string, idempotencyKey: string): Promise<JumbleEntry>;
  /** The version the server holds now, or `undefined` where the entry is gone. */
  versionOf(itemId: string): Promise<number | undefined>;
  trash(itemId: string, version: number): Promise<void>;
  /** All or nothing. */
  trashAll(itemIds: readonly string[], idempotencyKey: string): Promise<void>;
  dismiss(entryId: string): Promise<void>;
}

export interface CreateBody {
  readonly type: string;
  readonly collection_id: string;
  readonly title: string;
  readonly due_at?: string;
  readonly due_date_only?: boolean;
  readonly due_time_zone?: string;
  readonly assignee_id?: string;
  readonly label_ids?: readonly string[];
}

/** The lines one Enter makes into entries: each its own where lines split, else one line. */
export function linesOf(text: string, splitsLines: boolean): { readonly lines: readonly string[]; readonly rest: string } {
  const typed = text.split(/\r?\n/u);
  if (!splitsLines) {
    const line = typed.join(' ').replace(/\s+/gu, ' ').trim();
    return { lines: line === '' ? [] : [line], rest: '' };
  }
  const lines: string[] = [];
  let index = 0;
  for (; index < typed.length && lines.length < MAX_LINES; index += 1) {
    const line = (typed[index] ?? '').trim();
    if (line !== '') lines.push(line);
  }
  return { lines, rest: typed.slice(index).filter((line) => line.trim() !== '').join('\n') };
}

/** The create request for one read line. Only what the chips say is sent; nothing is guessed. */
export function bodyOf(
  reading: Reading,
  place: { readonly collectionId: string; readonly type: string },
  clock: { readonly now: number; readonly zone: string },
): CreateBody {
  const due = dueOf(reading.chips, clock);
  const wire = due ? dueInputFor(due, clock.zone) : undefined;
  const assignee = reading.chips.find((chip) => chip.kind === 'person')?.id;
  const labels = reading.chips.filter((chip) => chip.kind === 'label').map((chip) => chip.id ?? '').filter((id) => id !== '');
  return {
    type: place.type,
    collection_id: place.collectionId,
    title: reading.title,
    ...(wire ?? {}),
    ...(assignee ? { assignee_id: assignee } : {}),
    ...(labels.length > 0 ? { label_ids: labels } : {}),
  };
}

export type Outcome =
  | { readonly kind: 'items'; readonly created: readonly WorkItem[]; readonly failure?: unknown; readonly rest: string }
  | { readonly kind: 'jumble'; readonly entry: JumbleEntry };

/**
 * Creates the entries in order. A refusal part-way stops there: what was created stays, named in
 * the message, and the lines not yet made go back into the field with whatever did not fit.
 */
export async function createAll(
  writer: Writer,
  bodies: readonly CreateBody[],
  keys: readonly string[],
  lines: readonly string[],
  rest: string,
): Promise<Outcome> {
  const created: WorkItem[] = [];
  for (const [index, body] of bodies.entries()) {
    try {
      created.push(await writer.createItem(body, keys[index]!));
    } catch (failure) {
      const left = [...lines.slice(index), ...(rest === '' ? [] : [rest])].join('\n');
      return { kind: 'items', created, failure, rest: left };
    }
  }
  return { kind: 'items', created, rest };
}

/** The way back from what one Enter made. */
export function undoOf(writer: Writer, outcome: Outcome, newKey: () => string): () => Promise<void> {
  if (outcome.kind === 'jumble') return () => writer.dismiss(outcome.entry.id);
  const ids = outcome.created.map((item) => item.id);
  if (ids.length === 1) {
    return async () => {
      const version = await writer.versionOf(ids[0]!);
      if (version !== undefined) await writer.trash(ids[0]!, version);
    };
  }
  return () => writer.trashAll(ids, newKey());
}

/** The bulk the way back sends for several entries. */
export function trashOperations(itemIds: readonly string[]): readonly BulkOperation[] {
  return itemIds.map((id) => ({ op: 'TRASH_ITEM', item_id: id }) as BulkOperation);
}
