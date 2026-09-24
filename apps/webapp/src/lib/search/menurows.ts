// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the bar's menu offers, as one list (ADR-0066 decision 4).
 *
 * The rows are a list and not a tree, because the arrow keys are: a reader pressing ↓ walks the
 * hits, then the narrowings, then the ways on, and the index into *this* array is the whole of the
 * keyboard model. Building it in one pure function is what keeps the field's key handling and the
 * menu's markup from counting differently — the bug every combobox written twice has, where the
 * highlight and the thing Enter presses drift apart by one.
 *
 * Pure, and tested beside itself.
 */

import type { Quick } from '../data/searchquery.ts';

export type RowKind = 'hit' | 'quick' | 'all' | 'refine';

export interface Row {
  readonly kind: RowKind;
  /** The hit's identifier, the narrowing's line, or the row's own name. */
  readonly id: string;
}

/**
 * The rows, in the order the arrows walk them.
 *
 * `refine` — the same question with the filter open — is offered only where there is something to
 * refine. With an empty field the menu's one way on is the search screen itself, and a second row
 * saying almost the same thing would be a choice nobody has.
 */
export function rowsOf(
  term: string,
  hitIds: readonly string[],
  quick: readonly Quick[],
): readonly Row[] {
  const rows: Row[] = hitIds.map((id) => ({ kind: 'hit', id }));
  for (const each of quick) rows.push({ kind: 'quick', id: each.line });
  rows.push({ kind: 'all', id: 'all' });
  if (term.trim() !== '') rows.push({ kind: 'refine', id: 'refine' });
  return rows;
}

/**
 * Where ↓ and ↑ land, counted from "nothing highlighted".
 *
 * `-1` is that state and it is a real one rather than a placeholder: a field somebody is typing in
 * has no row selected, and Enter then means "search for what I typed" rather than "open whatever
 * happened to be first". Walking past either end returns to it, so the list cannot be left without
 * a way back to one's own words.
 */
export function step(active: number, count: number, by: 1 | -1): number {
  if (count === 0) return -1;
  const next = active + by;
  if (next >= count) return -1;
  if (next < -1) return count - 1;
  return next;
}
