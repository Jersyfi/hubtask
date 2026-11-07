// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The class hierarchy as the assets' settings draw it (user-experience.md §5.4): the root and its
 * classes open, each deeper branch folded until somebody opens it, so a shipped hierarchy of a few
 * dozen classes reads as its families rather than as one long column.
 */

export interface ClassRow {
  readonly key: string;
  /** The class and its ancestors, nearest first, up to the root (`CiClass.lineage`). */
  readonly lineage: readonly string[];
}

/** The rows drawn: the root's level and the next, and below them only what every open branch holds. */
export function shownClasses<T extends ClassRow>(tree: readonly T[], opened: ReadonlySet<string>): T[] {
  return tree.filter((each) => each.lineage.slice(1, -1).every((ancestor) => opened.has(ancestor)));
}

/** How many classes sit below each class, at any depth: a row with some can be opened. */
export function classesBelow(tree: readonly ClassRow[]): Map<string, number> {
  const below = new Map<string, number>();
  for (const each of tree) {
    for (const ancestor of each.lineage.slice(1)) below.set(ancestor, (below.get(ancestor) ?? 0) + 1);
  }
  return below;
}
