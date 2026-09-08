// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * An entry's history, turned from a record into the parts of a sentence.
 *
 * The rule that shapes all of it is `domain-model.md` §3.5: **`verb` is a code**. `item.completed`
 * is stored, `activity.item_completed` is what a client renders, and a module that wrote
 * "Completed" would be the message catalogue growing a second copy — which ADR-0011 forbids and
 * F1-07 built the renderer to prevent. So nothing here produces text; it produces codes and
 * parameters, and `messages.t` turns them into words.
 *
 * Two model facts this must not smooth over.
 *
 * **The change set keeps field names always and values only where the product needs them.** A
 * rename carries both titles; a note carries `changed: true` and none of its text, because no user
 * content goes anywhere it is not needed (ADR-0017). `changesOf` reads `from`, `to` and `changed`
 * and **nothing else** — so a field the server one day carries more about does not leak through a
 * client that was written before it.
 *
 * **An activity's history is compact.** The verb, the actor and the time are the whole of the
 * step and the change set is empty, so there is nothing here that invents a detail for one.
 */

import type { ActivityEntry } from '@hubtask/sync-engine';

/** What the history kept about one field. */
export interface Change {
  readonly field: string;
  /** Where there was a value before. Absent when there was none — the contract's own wording. */
  readonly from?: string;
  readonly to?: string;
  /** True for a field whose values the history does not keep. A note is the worked example. */
  readonly isOpaque: boolean;
}

/** How a value is written when it is shown at all. Never an object, never JSON at a reader. */
function textOf(value: unknown): string | undefined {
  if (value === null || value === undefined) return undefined;
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  // Anything else is a shape this client was not written for. It is left out rather than printed:
  // `[object Object]` in a history is worse than a field with no detail beside it.
  return undefined;
}

/**
 * What moved, as the history recorded it.
 *
 * The order is the change set's own. A client that sorted would be claiming an order the record
 * does not have, and the record's is at least the server's.
 */
export function changesOf(changeSet: Record<string, unknown> | undefined): readonly Change[] {
  if (!changeSet) return [];

  const changes: Change[] = [];
  for (const [field, moved] of Object.entries(changeSet)) {
    if (moved === null || typeof moved !== 'object') continue;
    const detail = moved as { from?: unknown; to?: unknown; changed?: unknown };
    changes.push({
      field,
      from: textOf(detail.from),
      to: textOf(detail.to),
      isOpaque: detail.changed === true,
    });
  }
  return changes;
}

/**
 * The change-set fields whose values are people rather than text.
 *
 * The four verbs C-01 added — `item.assigned`, `item.unassigned`, `item.member_added`,
 * `item.member_removed` — carry an account identifier where every other change carries a value a
 * reader can read. Showing the identifier would be showing a UUID to somebody asking who took the
 * entry over, so the name is looked up and the field list says which ones to look up.
 *
 * A list rather than a guess at "anything ending in `_id`": a `bucket_id` is not a person, and a
 * client that resolved every identifier as an account would ask for accounts that do not exist.
 */
const PEOPLE_FIELDS = new Set(['assignee_id', 'member_id', 'owner_id', 'actor_id']);

export function namesPeople(field: string): boolean {
  return PEOPLE_FIELDS.has(field);
}

/**
 * The change-set fields whose values are files.
 *
 * `item.attachment_added` and `item.attachment_removed` carry `media_id`, and a UUID in front of
 * somebody asking which file was attached is the same non-answer an account identifier is. The
 * name is one request away — the media record the client already reads for a cover — so the field
 * list says which values to look up, exactly as it does for people.
 *
 * `cover` is deliberately not here: it carries the whole cover object rather than an identifier,
 * and `textOf` already refuses to print an object at a reader.
 */
const MEDIA_FIELDS = new Set(['media_id']);

export function namesMedia(field: string): boolean {
  return MEDIA_FIELDS.has(field);
}

/** Every media identifier a set of changes mentions, each once. */
export function mediaNamedBy(changes: readonly Change[]): readonly string[] {
  const ids = new Set<string>();
  for (const change of changes) {
    if (!namesMedia(change.field)) continue;
    if (change.from) ids.add(change.from);
    if (change.to) ids.add(change.to);
  }
  return [...ids];
}

/** Every account identifier a set of changes mentions, each once. What the name lookup is asked. */
export function accountsNamedBy(changes: readonly Change[]): readonly string[] {
  const ids = new Set<string>();
  for (const change of changes) {
    if (!namesPeople(change.field)) continue;
    if (change.from) ids.add(change.from);
    if (change.to) ids.add(change.to);
  }
  return [...ids];
}

/**
 * The message codes that name who did it, most specific first.
 *
 * A list rather than one code, for the reason `problem.ts` keeps one: the actor kinds are an enum
 * in the contract and `domain-model.md` §2's rule is that such sets grow with the installation, so
 * a kind this client has never heard of falls back to the one sentence that is true of every
 * actor — "somebody" — rather than rendering a key.
 *
 * **These are the fallbacks, not the whole answer.** The contract says of the actor that "the
 * label is not here: the account is one request away", and `GET /accounts/{accountId}` is that
 * request — `lib/data/accounts.svelte.ts` makes it and caches what comes back. What this function
 * answers is what to say when no name was resolved: for the reader, "You"; for anybody else, the
 * sentence true of their kind. It invents no names, and it never has.
 */
export function actorCodes(entry: ActivityEntry, selfAccountId: string | undefined): string[] {
  const kind = entry.actor?.type;
  const id = entry.actor?.id;

  if (kind === 'USER' && id && id === selfAccountId) return ['app.activity.actor_you'];
  return [`app.activity.actor_${kind ?? 'UNKNOWN'}`, 'app.activity.actor_someone'];
}
