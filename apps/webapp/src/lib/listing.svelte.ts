// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Sorting and paging one of the settings tables, in one place.
//
// The seven of them - sessions, tokens, devices, grants, people, notifications, quotas - are the
// same shape: a list the client holds **in full**, drawn as a table, with a default order that
// means something. Each one was going to grow the same twenty lines of `$state`, the same reset
// of the page when the sort changes, and the same six calls into the message catalogue, and the
// seventh copy is where one of them quietly disagrees with the other six.
//
// It holds no data. It is handed the rows and gives back the page to draw, which keeps it a
// question about a list rather than a second place where a list lives.

import { comparing, nextSort, pageOf, type Page, type Sort } from '@hubtask/design-system/components';

import { messages, t } from './i18n/i18n.svelte.ts';

/** How to read the column a reader is sorting by, out of one row. */
export type ReadColumn<T> = (row: T, columnId: string) => string | number | Date | null | undefined;

interface Options<T> {
  /** Every row, in the order the view wants when nobody has sorted. */
  rows: () => readonly T[];
  read: ReadColumn<T>;
  /**
   * Which column is sorted before anybody touches it, and which way. Absent means the view's own
   * order - which is a statement in most of these: `SessionsView` answers "this device, then
   * newest" and that is not a column.
   */
  initialSort?: Sort;
  /**
   * How many rows a page holds, or `undefined` for a list that is never paged. A table of five
   * quotas with a pager under it is a pager that says "1-5 of 5" and costs a line for nothing.
   */
  pageSize?: number;
  /** What the list is, for the pager's name: "sessions", already resolved. */
  what: string;
}

/**
 * The state one settings table needs.
 *
 * Returned as an object of getters rather than a reactive record: the caller reads `.sort` and
 * `.page` in its own `$derived`, and the two `$state`s stay inside here where nothing else can
 * write to them.
 */
export function listing<T>(options: Options<T>) {
  let sort = $state<Sort>(options.initialSort ?? { direction: 'none' });
  let page = $state(1);

  const ordered = $derived.by(() => {
    const rows = options.rows();
    if (sort.columnId === undefined || sort.direction === 'none') return rows;
    const columnId = sort.columnId;
    return [...rows].sort(comparing((row: T) => options.read(row, columnId), sort.direction, messages.locale));
  });

  const shown = $derived.by((): Page<T> =>
    pageOf(ordered, page, options.pageSize ?? Math.max(1, ordered.length)),
  );

  return {
    get sort(): Sort {
      return sort;
    },
    /** The page to draw: its rows, its number, and the numbers the range sentence needs. */
    get page(): Page<T> {
      return shown;
    },
    /** Whether a pager is worth drawing at all. */
    get hasPages(): boolean {
      return options.pageSize !== undefined && shown.pageCount > 1;
    },
    /**
     * Everything the pager takes, resolved: spread into `<Pagination {...list.pager} />` and the
     * view never writes the page size, the total or any of the six words twice.
     */
    get pager() {
      return {
        total: shown.total,
        pageSize: options.pageSize ?? Math.max(1, shown.total),
        page: shown.page,
        label: t('app.list.pages', { what: options.what }),
        rangeLabel: t('app.list.range', {
          first: shown.firstRow,
          last: shown.lastRow,
          total: shown.total,
        }),
        previousLabel: t('app.list.previous_page'),
        nextLabel: t('app.list.next_page'),
        pageLabel: (number: number) => t('app.list.page', { number }),
        gapLabel: (from: number, to: number) => t('app.list.pages_between', { from, to }),
      };
    },
    /**
     * A heading was pressed. The page goes back to the first: every row under the reader has just
     * changed, and page 4 of the new order is not the page they were looking at.
     */
    sortBy(next: Sort): void {
      sort = next;
      page = 1;
    },
    /** The same, for a caller that has the column rather than the sort. */
    press(columnId: string): void {
      this.sortBy(nextSort(sort, columnId));
    },
    goTo(number: number): void {
      page = number;
    },
  };
}
