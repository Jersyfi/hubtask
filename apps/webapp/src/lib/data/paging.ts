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
