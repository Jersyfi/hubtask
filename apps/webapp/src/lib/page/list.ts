// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a `ListPage` narrows by (page-system PS-7): the search and the chips, over the rows the
 * client holds. Where an endpoint has no filter of its own yet, the screen narrows what is loaded
 * with these and says so where it calls them; the count then counts what is loaded.
 */

/** Text as a search compares it: case and accents do not matter. */
export function folded(text: string): string {
  return text.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLocaleLowerCase();
}

/** Whether any of the fields holds every word of the query. An empty query matches everything. */
export function matchesSearch(query: string, fields: readonly (string | undefined)[]): boolean {
  const words = folded(query).split(/\s+/u).filter((word) => word !== '');
  if (words.length === 0) return true;
  const haystack = fields.filter((field): field is string => field !== undefined).map(folded).join('\n');
  return words.every((word) => haystack.includes(word));
}

/** A chip's choice: nothing chosen narrows nothing. */
export function matchesChoice<T>(chosen: T | undefined, value: T | undefined): boolean {
  return chosen === undefined || chosen === value;
}

/** How many rows a long list shows before its *Show more* (page-system D-1): 25 · 50 · 100. */
export const ROWS_PER_PAGE = [25, 50, 100] as const;
export type RowsPerPage = (typeof ROWS_PER_PAGE)[number];

/** A kept choice, or the first where nothing valid was kept. */
export function rowsPerPageOf(kept: string | null | undefined): RowsPerPage {
  const value = Number(kept);
  return (ROWS_PER_PAGE as readonly number[]).includes(value) ? (value as RowsPerPage) : ROWS_PER_PAGE[0];
}
