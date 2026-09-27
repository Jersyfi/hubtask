// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic behind a sorted, paged table - out where it can be tested, for the reason
// `structure.ts` records: "which page numbers does a reader of page 7 of 40 see" is a question
// about a list, and a component that answered it inline could only be checked by opening a
// browser. Here they are functions over data, and `table.test.js` runs them under `node --test`.
//
// **Nothing here reaches a server.** Every function takes a list somebody already holds and
// answers a question about it. That is not an implementation detail - it is the boundary that
// keeps `Pagination` honest about what this product's API can do (see its own note).

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

/** Which slice of a list a page is, and what the reader should be told about it. */
export interface Page<T> {
  readonly rows: readonly T[];
  /** One-based, clamped into range: a page number past the end lands on the last page. */
  readonly page: number;
  readonly pageCount: number;
  /** One-based, inclusive, for the "17-32 of 91" sentence. Both 0 on an empty list. */
  readonly firstRow: number;
  readonly lastRow: number;
  readonly total: number;
}

/**
 * One page of a list the caller **already holds in full**.
 *
 * The clamping is the point. A reader on page 9 who deletes the last row of it must not be left
 * looking at an empty page with a pager that says 9 of 8 - so a page past the end resolves to the
 * last one, and a list that empties resolves to page 1 with nothing on it.
 */
export function pageOf<T>(rows: readonly T[], page: number, pageSize: number): Page<T> {
  const total = rows.length;
  const size = Math.max(1, Math.floor(pageSize));
  const pageCount = Math.max(1, Math.ceil(total / size));
  const clamped = Math.min(Math.max(1, Math.floor(page) || 1), pageCount);
  const start = (clamped - 1) * size;
  const slice = rows.slice(start, start + size);
  return {
    rows: slice,
    page: clamped,
    pageCount,
    firstRow: total === 0 ? 0 : start + 1,
    lastRow: total === 0 ? 0 : start + slice.length,
    total,
  };
}

/** One position in a pager: a page to go to, or a gap standing for the pages between two of them. */
export type Step =
  | { readonly kind: 'page'; readonly page: number }
  | { readonly kind: 'gap'; readonly from: number; readonly to: number };

/**
 * The `1 2 3 … 9` of a pager, with the gaps knowing which pages they stand for.
 *
 * A gap that knows its range is what lets it be a **control** rather than a dead ellipsis: it is
 * announced as the pages it hides and pressing it goes to the middle of them. Atlassian names the
 * range and then leaves the ellipsis inert, which tells a screen-reader reader about a door they
 * cannot open.
 *
 * The window is symmetric around the current page and the first and last are always present, so
 * the count of steps does not move as the reader walks through - a pager whose controls slide
 * under the pointer is one where the next press lands on the wrong page.
 *
 * A gap is never drawn in place of a **single** page: hiding one page behind a control that costs
 * the same width saves nothing and adds a press.
 */
export function pageSteps(page: number, pageCount: number, window = 1): readonly Step[] {
  if (pageCount <= 1) return [{ kind: 'page', page: 1 }];

  const shown = new Set<number>([1, pageCount]);
  for (let n = page - window; n <= page + window; n += 1) {
    if (n >= 1 && n <= pageCount) shown.add(n);
  }

  // The window is clipped at the ends, so page 1 would draw fewer controls than page 5 and the
  // pager would change width as the reader walks through it. Growing it back inwards fixes that -
  // but the thing to hold constant is the number of **controls**, not the number of pages: in the
  // middle two gaps take two slots and at either end only one does. So the count is measured on
  // the built steps rather than on the set, which is what the width test caught.
  const slots = Math.min(pageCount, window * 2 + 5);
  let steps = buildSteps(shown);
  while (steps.length < slots && shown.size < pageCount) {
    // Extend away from the edge the window is pressed against.
    const isAtStart = page - window <= 2;
    const inner = [...shown].filter((n) => n !== 1 && n !== pageCount);
    const next = isAtStart
      ? Math.max(1, ...inner) + 1
      : Math.min(pageCount, ...inner) - 1;
    if (next <= 1 || next >= pageCount || shown.has(next)) break;
    shown.add(next);
    steps = buildSteps(shown);
  }
  return steps;
}

/** The pages, in order, with each run of missing ones between them closed by a gap. */
function buildSteps(shown: ReadonlySet<number>): Step[] {
  const pages = [...shown].sort((a, b) => a - b);
  const steps: Step[] = [];
  for (const [index, n] of pages.entries()) {
    const previous = pages[index - 1];
    if (previous !== undefined && n - previous > 1) {
      // A gap is never drawn in place of a single page: a control hiding one page is the same
      // width as the page it hides and adds a press.
      if (n - previous === 2) steps.push({ kind: 'page', page: previous + 1 });
      else steps.push({ kind: 'gap', from: previous + 1, to: n - 1 });
    }
    steps.push({ kind: 'page', page: n });
  }
  return steps;
}

/** Where a gap goes when it is pressed: the middle of what it hides. */
export function gapTarget(gap: { readonly from: number; readonly to: number }): number {
  return Math.floor((gap.from + gap.to) / 2);
}
