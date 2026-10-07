// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Sorting one of the settings tables, in one place.
//
// The six of them - sessions, tokens, devices, grants, people, quotas - are the same shape: a list
// the client holds in full, drawn as a table, with a default order that means something. Each one
// was going to grow the same `$state`, the same `$derived` over `comparing`, and the same reading
// of the reader's locale, and the sixth copy is where one of them quietly disagrees with the other
// five.
//
// **There is no page here.** This product's API answers a page and an opaque cursor and has no
// page numbers, and a client-side pager over a list that happens to have arrived whole would give
// the product two different answers to "where am I in this list" (`design-system.md`
// §4). A long list is sorted or narrowed; a list with a cursor behind it uses
// `LoadMore`.
//
// It holds no data. It is handed the rows and gives back the rows in order, which keeps it a
// question about a list rather than a second place where a list lives.

import { comparing, type Sort } from '@hubtask/design-system/components';

import { messages } from './i18n/i18n.svelte.ts';

/** How to read the column a reader is sorting by, out of one row. */
export type ReadColumn<T> = (row: T, columnId: string) => string | number | Date | null | undefined;

interface Options<T> {
  /** Every row, in the order the view wants when nobody has sorted. */
  rows: () => readonly T[];
  read: ReadColumn<T>;
  /**
   * Which column is sorted before anybody touches it, and which way. Absent means the view's own
   * order - which is a statement in most of these: `SessionsView` answers "this device, then
   * newest" and that is not a column, which is why the third press of a heading matters.
   */
  initialSort?: Sort;
}

/**
 * The state one sortable table needs.
 *
 * Returned as getters rather than a reactive record: the caller reads `.sort` and `.rows` in its
 * own markup, and the one `$state` stays in here where nothing else can write to it.
 */
export function sorting<T>(options: Options<T>) {
  let sort = $state<Sort>(options.initialSort ?? { direction: 'none' });

  const ordered = $derived.by(() => {
    const rows = options.rows();
    if (sort.columnId === undefined || sort.direction === 'none') return rows;
    const columnId = sort.columnId;
    // The reader's collation, not the codepoint order: `Ä` sits beside `A` for a German reader
    // and after `Z` for nobody (`i18n-l10n.md` §5).
    return [...rows].sort(comparing((row: T) => options.read(row, columnId), sort.direction, messages.locale));
  });

  return {
    get sort(): Sort {
      return sort;
    },
    /** The rows to draw, in the order the reader asked for. */
    get rows(): readonly T[] {
      return ordered;
    },
    by(next: Sort): void {
      sort = next;
    },
  };
}
