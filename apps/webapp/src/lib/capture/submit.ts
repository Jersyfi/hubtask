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
import { capture, parseLine, type CaptureRules } from '../entries/capture.ts';
import { dueOf, type Reading } from './parse.ts';

/** The stores, as far as a capture writes to them. */
export interface Writer {
  createItem(body: CreateBody, idempotencyKey: string): Promise<WorkItem>;
  submitJumble(subject: string, body: string, idempotencyKey: string): Promise<JumbleEntry>;
}

export interface CreateBody {
  readonly type: string;
  /** Where the entry goes: its collection, or the entry it goes under, which names the collection. */
  readonly collection_id?: string;
  readonly parent_id?: string;
  readonly bucket_id?: string;
  readonly title: string;
  readonly due_at?: string;
  readonly due_date_only?: boolean;
  readonly due_time_zone?: string;
  readonly assignee_id?: string;
  readonly label_ids?: readonly string[];
  readonly amount?: string;
  /** The key of the entry's type, absent for its level's own (UC-FLW-01 check 3). */
  readonly entry_type?: string;
}

/** One line of a capture, as the target reads it: its chips, its title, its leading amount. */
export interface ReadLine {
  /** The line as it goes back into the field when it is not created. */
  readonly line: string;
  readonly reading: Reading;
  readonly amount?: string;
}

/**
 * The lines one Enter makes into entries, by the one rule both capture fields read a collection by
 * (`captureRulesOf`): each its own where lines split, else one; each read by `read` - the chips -
 * and then its leading amount off the title that is left, so "tomorrow 2 kg apples" and "2 kg
 * apples tomorrow" both carry the amount.
 */
export function readLinesOf(
  text: string,
  rules: CaptureRules,
  read: (line: string) => Reading,
): { readonly lines: readonly ReadLine[]; readonly rest: string } {
  const { lines, rest } = capture(text, { ...rules, readsAmount: false });
  return {
    lines: lines.map(({ title: line }) => {
      const reading = read(line);
      const { title, amount } = parseLine(reading.title, rules);
      return { line, reading: { ...reading, title }, ...(amount ? { amount } : {}) };
    }),
    rest,
  };
}

/** The create request for one read line. Only what the chips say is sent; nothing is guessed. */
export function bodyOf(
  reading: Reading,
  place: {
    readonly collectionId: string;
    readonly type: string;
    readonly parentId?: string;
    readonly bucketId?: string;
    readonly entryTypeKey?: string;
  },
  clock: { readonly now: number; readonly zone: string },
  amount?: string,
): CreateBody {
  const due = dueOf(reading.chips, clock);
  const wire = due ? dueInputFor(due, clock.zone) : undefined;
  const assignee = reading.chips.find((chip) => chip.kind === 'person')?.id;
  const labels = reading.chips.filter((chip) => chip.kind === 'label').map((chip) => chip.id ?? '').filter((id) => id !== '');
  return {
    type: place.type,
    // Under an entry the parent is the place, as the list's own form sends it.
    ...(place.parentId ? { parent_id: place.parentId } : { collection_id: place.collectionId }),
    ...(place.bucketId ? { bucket_id: place.bucketId } : {}),
    title: reading.title,
    ...(wire ?? {}),
    ...(assignee ? { assignee_id: assignee } : {}),
    ...(labels.length > 0 ? { label_ids: labels } : {}),
    ...(amount ? { amount } : {}),
    ...(place.entryTypeKey ? { entry_type: place.entryTypeKey } : {}),
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
