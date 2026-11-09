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

/** A class's record page. */
export const classPath = (key: string): string => `/assets/classes/${encodeURIComponent(key)}`;

interface TreeClass extends ClassRow {
  readonly parent?: string | null;
}

/** The hierarchy in reading order: each class after its parent, siblings in the order listed. */
export function classesInOrder<T extends TreeClass>(classes: readonly T[]): T[] {
  const out: T[] = [];
  const visit = (parent: string) => {
    for (const each of classes.filter((one) => one.parent === parent)) {
      out.push(each);
      visit(each.key);
    }
  };
  for (const root of classes.filter((each) => !each.parent)) {
    out.push(root);
    visit(root.key);
  }
  return out;
}

/** The classes directly beneath one. */
export const classesUnder = <T extends TreeClass>(classes: readonly T[], key: string): T[] => classes.filter((each) => each.parent === key);

interface Joining {
  readonly key: string;
  readonly pairs: readonly { readonly from: string; readonly to: string }[];
}

/**
 * Where a class is used among the relation types: each type with a pair that names the class or a
 * class above it at either end, a pair naming an ancestor applying to it too.
 */
export function typesJoining<T extends Joining>(types: readonly T[], lineage: readonly string[]): T[] {
  return types.filter((type) => type.pairs.some((pair) => lineage.includes(pair.from) || lineage.includes(pair.to)));
}
