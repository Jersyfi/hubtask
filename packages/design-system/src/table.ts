// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic behind a sorted table - out where it can be tested, for the reason `structure.ts`
// records: "where does an empty cell go when the direction flips" is a question about a list, and
// a component that answered it inline could only be checked by opening a browser. Here they are
// functions over data, and `table.test.js` runs them under `node --test`.
//
// **There is no paging arithmetic here, and that is the point.** This product's API answers a page
// and an opaque cursor; it has no page numbers and cannot be asked for a fourth page, so no part
// of the client may grow a `pageOf` that would imply one. A list too long to read is narrowed or
// sorted, or it arrives through `LoadMore`. `apps/webapp/CLAUDE.md` states the rule.

/** Which way a column is sorted. `none` is a sortable column nobody has sorted by. */
export type SortDirection = 'ascending' | 'descending' | 'none';

/** Which column is sorted, and which way. `columnId` absent means the caller's own order. */
export interface Sort {
  readonly columnId?: string;
  readonly direction: SortDirection;
}

/**
 * The next sort when a heading is pressed, which is a three-state cycle and not a two-state one.
 *
 * Ascending, descending, and **back to none** - because the order a list arrives in is itself a
 * statement. `SessionsView` answers "this device first, then newest"; a reader who sorts by client
 * and wants that back has no way to ask for it if the cycle only flips. Atlassian's table has two
 * states and no way home, which is the one thing about it worth not copying.
 *
 * Pressing a *different* column starts that column at ascending rather than continuing this
 * column's phase: the direction belongs to the question, not to the press.
 */
export function nextSort(current: Sort, columnId: string): Sort {
  if (current.columnId !== columnId) return { columnId, direction: 'ascending' };
  if (current.direction === 'ascending') return { columnId, direction: 'descending' };
  if (current.direction === 'descending') return { direction: 'none' };
  return { columnId, direction: 'ascending' };
}

/**
 * A comparator for one column, given how to read the column out of a row.
 *
 * `undefined` and `null` sort **last in both directions**, which is deliberate and is the opposite
 * of what a naive comparator does. A missing due date is not "earliest" and not "latest" - it is
 * absent, and a reader sorting by due date is looking for the dated ones. Flipping the direction
 * must not promote every empty cell to the top of the screen.
 *
 * Text is compared with the reader's collation, which is `i18n-l10n.md` §5's requirement rather
 * than a nicety: `Ä` sorts beside `A` for a German reader and after `Z` for nobody. Numbers and
 * dates compare as themselves - `10` after `9`, which is the whole reason a key is not stringified
 * first.
 */
export function comparing<T>(
  read: (row: T) => string | number | Date | null | undefined,
  direction: SortDirection,
  locale?: string,
): (a: T, b: T) => number {
  if (direction === 'none') return () => 0;
  const sign = direction === 'ascending' ? 1 : -1;
  const collator = new Intl.Collator(locale, { numeric: true, sensitivity: 'base' });

  return (a, b) => {
    const left = read(a);
    const right = read(b);
    const isLeftEmpty = left === null || left === undefined || left === '';
    const isRightEmpty = right === null || right === undefined || right === '';
    if (isLeftEmpty || isRightEmpty) {
      if (isLeftEmpty && isRightEmpty) return 0;
      // Last whichever way round the rest is going, so the sign is not applied here.
      return isLeftEmpty ? 1 : -1;
    }
    if (left instanceof Date || right instanceof Date) {
      return sign * (Number(new Date(left as Date)) - Number(new Date(right as Date)));
    }
    if (typeof left === 'number' && typeof right === 'number') return sign * (left - right);
    return sign * collator.compare(String(left), String(right));
  };
}
