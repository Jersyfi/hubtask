// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** How far a paged read has come: the rows held, the total where the answer counted them, and whether more exist. */
export interface PageReach {
  readonly shown: number;
  readonly total: number | undefined;
  readonly hasMore: boolean;
}

interface PagedAnswer {
  readonly data?: readonly unknown[];
  readonly total?: number | null;
  readonly page?: { readonly next_cursor?: string | null; readonly has_more?: boolean } | null;
}

/**
 * reachOf reads a paged answer. A next page exists where the answer says so, or, from an answer
 * that does not say, where it handed a cursor: a list that stops at its first page without saying
 * so hides every entry after it.
 */
export function reachOf(answer: PagedAnswer | undefined): PageReach {
  if (!answer) return { shown: 0, total: undefined, hasMore: false };
  const shown = answer.data?.length ?? 0;
  const total = typeof answer.total === 'number' ? answer.total : undefined;
  const hasMore = answer.page?.has_more ?? Boolean(answer.page?.next_cursor);
  return { shown, total, hasMore };
}

/** How many more cards one continuation of a board column reads. */
export const COLUMN_PAGE = 50;

/**
 * columnRequest is the question that continues one column of a grouped board read: the same
 * document without the grouping, the column's key as one more filter, the group's cursor as the
 * cursor (api-guidelines.md, `group_by`). A grouped answer has no single walk, so a column is never
 * a later page of the board itself.
 */
export function columnRequest(
  board: Readonly<Record<string, unknown>>, key: string | null, cursor: string,
): Record<string, unknown> {
  const { group_by: grouping, filter, page: _page, count: _count, ...rest } = board as {
    group_by?: { field?: string };
    filter?: unknown;
    page?: unknown;
    count?: unknown;
  };
  const field = grouping?.field ?? 'bucket_id';
  const column = key === null ? { field, op: 'IS_NULL' } : { field, op: 'EQ', value: key };
  return {
    ...rest,
    filter: filter ? { op: 'AND', nodes: [filter, column] } : column,
    page: { cursor, size: COLUMN_PAGE },
    count: 'none',
  };
}

/**
 * columnCards joins a column's first cards and the ones read after them, each card once: the board
 * and its continuation are two reads, and a card moved between them can stand in both.
 */
export function columnCards<T extends { readonly id: string }>(first: readonly T[], more: readonly T[]): readonly T[] {
  if (more.length === 0) return first;
  const seen = new Set(first.map((card) => card.id));
  return [...first, ...more.filter((card) => !seen.has(card.id))];
}
