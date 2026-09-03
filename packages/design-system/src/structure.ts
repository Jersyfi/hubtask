// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic behind wave 2's two structural components, out where it can be tested.
//
// The same reasoning `focus.ts` and `layers.ts` record: "which crumb is in the middle" and "which
// row is this one's parent when three of five branches are collapsed" are questions about a list,
// and a component that answered them inline could only be checked by opening a browser. Here they
// are functions over data, and `structure.test.js` runs them under `node --test`.

/** One level of a breadcrumb trail. The last one is where the reader is. */
export interface Crumb {
  readonly id: string;
  /** Resolved text (ADR-0011), never a code. */
  readonly label: string;
  /** Where it goes. Absent on the current level, which is not a link. */
  readonly href?: string;
}

/** What a breadcrumb renders: the crumbs to show, and how many are hidden between them. */
export interface CollapsedTrail<T> {
  readonly shown: readonly { readonly crumb: T; readonly index: number }[];
  readonly hidden: number;
}

/**
 * The `Hub / … / Parent / Current` of design-system.md §4.
 *
 * Three shown and the rest hidden, and *which* three is the part that follows from the domain
 * rather than from a rule of thumb: the first answers "which workspace", the last two answer "what
 * am I looking at and what holds it", and the levels between are the ones a reader of
 * domain-model.md §3.4's hierarchy can reconstruct.
 *
 * Four is the shortest trail with a middle to hide. A trail of three collapses to itself, so the
 * ellipsis never appears in place of a single level - which would cost a control to save nothing.
 */
export function collapseTrail<T>(trail: readonly T[], isExpanded = false): CollapsedTrail<T> {
  const all = trail.map((crumb, index) => ({ crumb, index }));
  if (isExpanded || trail.length <= 3) return { shown: all, hidden: 0 };

  return {
    shown: [all[0]!, all[trail.length - 2]!, all[trail.length - 1]!],
    hidden: trail.length - 3,
  };
}

/** One node of a navigation tree. Children make it a branch, and there is no third case. */
export interface NavNode {
  readonly id: string;
  readonly label: string;
  readonly icon?: string;
  readonly href?: string;
  readonly children?: readonly NavNode[];
}

/**
 * One visible row of the tree, as the eye and the arrow keys both travel it.
 *
 * Generic in the node, so a component that narrows `icon` to the names its icon set actually has
 * keeps that narrowing through the flattening. A fixed `NavNode` here would hand every row back
 * with `icon: string`, and the component would need a cast to render one — which is the point at
 * which a type stops being checked.
 */
export interface NavRow<T extends NavNode = NavNode> {
  readonly node: T;
  readonly depth: number;
  readonly isBranch: boolean;
  readonly isExpanded: boolean;
}

/**
 * The visible nodes, flattened in reading order.
 *
 * Flattened rather than walked at each key press, because every question the keyboard asks is
 * about the *visible* list - the next row, the previous one, the last - and a collapsed branch's
 * children are not in it. A component that walked the tree would answer "the next node" with one
 * nobody can see.
 */
export function flattenTree<T extends NavNode>(
  nodes: readonly T[],
  expanded: readonly string[],
  depth = 0,
): NavRow<T>[] {
  const rows: NavRow<T>[] = [];
  for (const node of nodes) {
    const isBranch = (node.children?.length ?? 0) > 0;
    const isExpanded = isBranch && expanded.includes(node.id);
    rows.push({ node, depth, isBranch, isExpanded });
    if (isExpanded) rows.push(...flattenTree((node.children ?? []) as readonly T[], expanded, depth + 1));
  }
  return rows;
}

/**
 * The row that holds this one, or `null` at the top level.
 *
 * The nearest row above with a smaller depth. That is what the direction arrow means on a node
 * that is a leaf or already closed, and it is why the rows carry their depth: a parent is a
 * position in the flattened list, not a reference in the tree.
 */
export function parentRow(rows: readonly NavRow<NavNode>[], index: number): number | null {
  const row = rows[index];
  if (!row) return null;
  for (let above = index - 1; above >= 0; above--) {
    if ((rows[above]?.depth ?? 0) < row.depth) return above;
  }
  return null;
}

/**
 * Where a direction key moves or what it opens, for one row of a tree.
 *
 * Returned as an intention rather than performed, so the decision can be tested without a DOM and
 * so RTL is settled in one place: which physical arrow points *towards the children* is a
 * direction question, and a component that asked it twice would answer it differently once.
 */
export type TreeIntent =
  | { readonly kind: 'expand' }
  | { readonly kind: 'collapse' }
  | { readonly kind: 'focus'; readonly index: number }
  | null;

export function treeIntent(
  key: string,
  rows: readonly NavRow<NavNode>[],
  index: number,
  dir: 'ltr' | 'rtl' = 'ltr',
): TreeIntent {
  const row = rows[index];
  if (!row) return null;

  const towardsChildren = dir === 'rtl' ? 'ArrowLeft' : 'ArrowRight';
  const towardsParent = dir === 'rtl' ? 'ArrowRight' : 'ArrowLeft';

  if (key === towardsChildren) {
    if (row.isBranch && !row.isExpanded) return { kind: 'expand' };
    if (row.isExpanded) return { kind: 'focus', index: index + 1 };
    return null;
  }
  if (key === towardsParent) {
    if (row.isExpanded) return { kind: 'collapse' };
    const parent = parentRow(rows, index);
    return parent === null ? null : { kind: 'focus', index: parent };
  }
  // A tree does not wrap, unlike a menu: a list has a shape, and running off the end of it loses
  // the reader's place in that shape.
  if (key === 'ArrowDown') {
    return index + 1 < rows.length ? { kind: 'focus', index: index + 1 } : null;
  }
  if (key === 'ArrowUp') return index > 0 ? { kind: 'focus', index: index - 1 } : null;
  if (key === 'Home') return rows.length > 0 ? { kind: 'focus', index: 0 } : null;
  if (key === 'End') return rows.length > 0 ? { kind: 'focus', index: rows.length - 1 } : null;
  return null;
}
