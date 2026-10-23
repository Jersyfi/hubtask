// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What Enter writes (UC-WRK-26 check 4; record D7). Pure over a
 * `Writer`, which the field binds to the stores: the tests drive it with a fake and see every
 * request it would make.
 *
 * **One key per Enter.** The idempotency key belongs to the intent - this Enter - and a retry of
 * it reuses the key, so a create that reached the server once is never made twice.
 *
 * *Undo* is not here: a capture offers the way back every act offers (`entries/undo`), which trashes
 * what one Enter made, or dismisses a jumble entry (record D9).
 */

import type { WorkItem } from '@hubtask/sync-engine';

import type { JumbleEntry } from '../data/jumble.svelte.ts';

import { dueInputFor } from '../data/due.ts';
import { dueOf, type Reading } from './parse.ts';

/** How many entries one Enter makes at most; the rest stays in the field. */
export const MAX_LINES = 100;

/** The stores, as far as a capture writes to them. */
export interface Writer {
  createItem(body: CreateBody, idempotencyKey: string): Promise<WorkItem>;
  submitJumble(subject: string, idempotencyKey: string): Promise<JumbleEntry>;
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
