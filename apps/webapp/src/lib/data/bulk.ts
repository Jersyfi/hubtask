// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The bulk document, and reading what came back.
 *
 * **A bulk is the single-entry operations, sent together.** The contract says so in its own words —
 * "what a bulk may do is what a caller may do one entry at a time, never more" — so the payload of
 * each operation is the body its own route takes, by the field names it takes there. Nothing here
 * invents a shape: a field the operation does not know is refused by name rather than ignored.
 *
 * **`CREATE_ITEM` is the one that is not about a selection**, and the contract says why: it is
 * "required by every operation except CREATE_ITEM, which has no entry yet". Applying it to picked
 * entries is not a thing that can be meant. What it *is* good for is the other bulk a person
 * actually performs — adding several entries at once, one per line — so that is what the bar
 * offers under it, and `createOperations` is the shape of it.
 *
 * **A result per operation, and 200 is not "it worked".** "A bulk that half succeeded is not a
 * failed request", so the status lives in each result. Three outcomes, not two: applied, refused
 * with its problem, and *not applied* — an operation of an atomic bulk that never ran because
 * another one failed. The third carries neither an item nor a problem, which is exactly how it is
 * told apart from a genuine `409`.
 */

import type { BulkOperation, BulkResult } from '@hubtask/sync-engine';

/** The nine the contract's enum declares, in the order a bar offers them. */
export const BULK_OPERATIONS = [
  'COMPLETE_ITEM',
  'REOPEN_ITEM',
  'ADD_LABEL',
  'REMOVE_LABEL',
  'ASSIGN',
  'MOVE_ITEM',
  'UPDATE_ITEM',
  'CREATE_ITEM',
  'TRASH_ITEM',
] as const;

export type BulkOp = (typeof BULK_OPERATIONS)[number];

/** The one operation that carries no entry, because it has none yet. */
export function needsItem(op: string): boolean {
  return op !== 'CREATE_ITEM';
}

/** The one that asks first. Reversible, and still a subtree somebody may not know is there. */
export function isDestructive(op: string): boolean {
  return op === 'TRASH_ITEM';
}

/**
 * One operation per selected entry, with the same payload on each.
 *
 * The order is the selection's, which is the order the reader sees — so the results come back in
 * the order the rows are drawn and each one lands where it belongs without a lookup by identifier.
 */
export function operationsFor(
  op: BulkOp,
  itemIds: readonly string[],
  payload: Record<string, unknown> = {},
): readonly BulkOperation[] {
  return itemIds.map((itemId) => ({ op, item_id: itemId, payload }) as BulkOperation);
}

/**
 * One `CREATE_ITEM` per title.
 *
 * The titles are what a person typed, one per line, blanks dropped: an entry with no title is
 * refused by the server and is not something anybody meant to ask for.
 */
export function createOperations(
  titles: string,
  base: { collection_id: string; type: string; parent_id?: string | null },
): readonly BulkOperation[] {
  return titles
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')
    .map((title) => ({ op: 'CREATE_ITEM', payload: { ...base, title } }) as BulkOperation);
}

/** What one operation did, as three words rather than a status code. */
export type Outcome = 'applied' | 'refused' | 'not_applied';

/**
 * Which of the three a result is.
 *
 * **Two shapes of "not applied", because the contract and the server describe it differently.**
 * The schema says a rolled-back operation "carries neither and says so by its status"; the server
 * sends `409` *with* a problem whose detail code is `bulk.rolled_back` — and that one is the better
 * answer, because it names the operation that failed. Both are recognised here rather than one:
 * reading the status alone would call a genuine version conflict a rollback, and reading only the
 * contract's shape would call every rollback a refusal.
 */
const ROLLED_BACK = 'bulk.rolled_back';

export function outcomeOf(result: BulkResult): Outcome {
  const status = result.status ?? 0;
  if (status >= 200 && status < 300) return 'applied';
  if ((result.problem as { detail_code?: string } | undefined)?.detail_code === ROLLED_BACK) {
    return 'not_applied';
  }
  if (!result.problem && !result.item && status === 409) return 'not_applied';
  return 'refused';
}

/** Whether the whole bulk landed. What decides between a quiet close and a report on screen. */
export function allApplied(results: readonly BulkResult[]): boolean {
  return results.length > 0 && results.every((result) => outcomeOf(result) === 'applied');
}

/**
 * Each result against the entry its operation was about.
 *
 * By index rather than by the answered item: a refused operation carries no item, and that is
 * precisely the one whose entry a reader most needs named.
 */
export function byItem(
  operations: readonly BulkOperation[],
  results: readonly BulkResult[],
): ReadonlyMap<string, BulkResult> {
  const found = new Map<string, BulkResult>();
  for (const result of results) {
    const itemId = operations[result.index ?? -1]?.item_id;
    if (itemId) found.set(itemId, result);
  }
  return found;
}

/** How many of each outcome, for the one sentence that says how a bulk went. */
export function tallyOf(results: readonly BulkResult[]): Record<Outcome, number> {
  const tally: Record<Outcome, number> = { applied: 0, refused: 0, not_applied: 0 };
  for (const result of results) tally[outcomeOf(result)] += 1;
  return tally;
}
