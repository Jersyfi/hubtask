// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The comment margin beside a page (UC-DOC-11 check 4): each thread with a passage is drawn level
// with it, and a thread that would cover the one above is pushed down below it. Threads whose words
// are gone, and the page's own, follow under the last.

/** A thread to place: where its passage starts (none when its words are gone) and how tall it is. */
export interface MarginRow {
  readonly id: string;
  readonly top: number | undefined;
  readonly height: number;
}

/** Where each thread is drawn, from the margin's top, and how tall the margin is then. */
export interface MarginLayout {
  readonly tops: ReadonlyMap<string, number>;
  readonly height: number;
}

/** Places the threads in their passages' order, never overlapping, `gap` apart. */
export function levelThreads(rows: readonly MarginRow[], gap: number): MarginLayout {
  const placed = rows
    .map((row, order) => ({ row, order }))
    .sort((a, b) => {
      const at = a.row.top ?? Number.POSITIVE_INFINITY;
      const bt = b.row.top ?? Number.POSITIVE_INFINITY;
      return at === bt ? a.order - b.order : at - bt;
    });
  const tops = new Map<string, number>();
  let next = 0;
  for (const { row } of placed) {
    const top = Math.max(row.top ?? next, next, 0);
    tops.set(row.id, top);
    next = top + row.height + gap;
  }
  return { tops, height: Math.max(next - gap, 0) };
}
