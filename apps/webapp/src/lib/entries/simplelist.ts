// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The simple list's sections (UC-WRK-25 checks 3, 4, 5): what is still to get, grouped by the
 * collection's categories in their order; what was ticked in the last 24 hours, under *Done*; and
 * what was ticked before that, under *Recently*, most used first.
 *
 * Pure and over the one level the list already reads (`items.svelte.ts`'s level question), which is
 * the read the local copy answers offline: a filtered or re-sorted query is refused offline by
 * design (`replica.ts`). So the sections are drawn the same way online and off, from the same
 * rows, and a second device draws them from the records the server sent it.
 *
 * The 24 hours need no stored state and mean "this trip". `completed_at` is the server's moment:
 * an entry ticked offline shows the device's prediction until the pull, then the server's time.
 */

/** How long a ticked entry stays under *Done* before it moves to *Recently*. */
export const DONE_WINDOW_MS = 24 * 60 * 60 * 1000;

/** What the sections read off an entry: the contract's own field names. */
export interface ListEntry {
  readonly id: string;
  readonly title: string;
  readonly completion?: { readonly is_completed?: boolean; readonly completed_at?: string | null };
  readonly completion_count?: number;
  readonly label_ids?: readonly string[] | null;
  readonly archived_at?: string | null;
}

export interface Group<T extends ListEntry> {
  /** The category's label, or `null` for the entries that carry none. */
  readonly category: string | null;
  readonly entries: readonly T[];
}

export interface Sections<T extends ListEntry> {
  readonly open: readonly Group<T>[];
  readonly done: readonly T[];
  readonly recently: readonly T[];
  /** When the oldest entry under *Done* moves to *Recently*, so a timer can re-sort then. */
  readonly nextBoundary?: number;
}

const completedAt = (entry: ListEntry) => {
  const at = entry.completion?.completed_at;
  return at ? Date.parse(at) : Number.NaN;
};

/**
 * The open entries under their categories: an entry sits under the first category, in the
 * categories' order, that it carries; the uncategorised come last. A category whose label no
 * longer exists (`known`) is skipped, and one with nothing under it is not drawn.
 */
export function groupByCategory<T extends ListEntry>(
  open: readonly T[],
  categories: readonly string[],
  known: ReadonlySet<string>,
): readonly Group<T>[] {
  const live = categories.filter((category) => known.has(category));
  if (live.length === 0) return open.length > 0 ? [{ category: null, entries: open }] : [];
  const groups = new Map<string | null, T[]>(live.map((category) => [category, []]));
  groups.set(null, []);
  for (const entry of open) {
    const labels = entry.label_ids ?? [];
    const category = live.find((each) => labels.includes(each)) ?? null;
    groups.get(category)?.push(entry);
  }
  return [...groups.entries()]
    .filter(([, entries]) => entries.length > 0)
    .map(([category, entries]) => ({ category, entries }));
}

/** The three sections at `now`, the open entries in the order they arrived (the manual order). */
export function partition<T extends ListEntry>(
  entries: readonly T[],
  now: number,
  categories: readonly string[] = [],
  known: ReadonlySet<string> = new Set(),
): Sections<T> {
  const open: T[] = [];
  const done: T[] = [];
  const recently: T[] = [];
  for (const entry of entries) {
    if (!entry.completion?.is_completed) {
      open.push(entry);
      continue;
    }
    const at = completedAt(entry);
    if (Number.isFinite(at) && now - at < DONE_WINDOW_MS) done.push(entry);
    // Put away is not offered back: an archived entry is read-only and stays where it is.
    else if (!entry.archived_at) recently.push(entry);
  }
  done.sort((a, b) => completedAt(b) - completedAt(a));
  recently.sort(
    (a, b) => (b.completion_count ?? 0) - (a.completion_count ?? 0) || (completedAt(b) || 0) - (completedAt(a) || 0),
  );
  const boundaries = done.map((entry) => completedAt(entry) + DONE_WINDOW_MS);
  return {
    open: groupByCategory(open, categories, known),
    done,
    recently,
    ...(boundaries.length > 0 ? { nextBoundary: Math.min(...boundaries) } : {}),
  };
}

/** Folded for comparing what a person typed with a title: one form, one case. */
const fold = (text: string) => text.normalize('NFC').toLocaleLowerCase().trim();

/** *Recently* narrowed to what the capture line's title asks for: every entry whose title holds it. */
export function narrow<T extends ListEntry>(recently: readonly T[], typed: string): readonly T[] {
  const wanted = fold(typed);
  if (wanted === '') return recently;
  return recently.filter((entry) => fold(entry.title).includes(wanted));
}
