// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The sidebar's tree drag as arithmetic (the navigation concept's §5.4, design-system.md §11.9).
//
// Where a carried row lands, which key press moves a row where, which position is announced: each
// is a question about a list of rows, answered here as functions so `navtree.test.js` checks them
// under `node --test`, as `reorder.ts` does for a flat list. The components draw what these answer;
// the write is the caller's.
//
// A placement is the API's own shape - a parent (null is the top of the part) and the sibling it
// goes after (null is first) - so `movePage`, `moveWhiteboard` and a collection's `:move` take it
// as it is.

import { flattenTree, type NavNode, type NavRow } from './structure.ts';

/** Travel before a press becomes a drag; below it a click stays a click (concept §5.4). */
export const TREE_DRAG_THRESHOLD_PX = 4;

/** Within this distance of the scrolling region's edge a carried row scrolls it (§5.4). */
export const AUTO_SCROLL_EDGE_PX = 48;

/** The deepest level the indent still grows at; deeper levels stand at this one (§2.4). */
export const INDENT_LEVELS = 6;

/** The server's bound on page nesting (`Page.go`): a move past it is refused here first. */
export const MAX_DEPTH = 12;

/** A node of the sidebar's tree, with what the drag has to know about it. */
export interface TreeNode extends NavNode {
  readonly children?: readonly TreeNode[];
  /** A whiteboard takes nothing in (`whiteboards.no_children`). Absent means it may. */
  readonly canHoldChildren?: boolean;
  /** The part of the sidebar the row belongs to: a drag never leaves its part. */
  readonly part?: string;
  /** A collection row an entry from the content region may be dropped onto (UC-WRK-06). */
  readonly acceptsEntries?: boolean;
  /** What the filter's kind narrows by: `page`, `whiteboard`. */
  readonly kind?: string;
}

/** Where a row goes: under `parentId` (null is the part's top), after `afterId` (null is first). */
export interface Placement {
  readonly parentId: string | null;
  readonly afterId: string | null;
}

/** Why a target is refused; the caller words it on the drag image (no sentence here, ADR-0011). */
export type Refusal = 'self' | 'no-children' | 'other-part' | 'too-deep';

export type Zone = 'before' | 'inside' | 'after';

/** The drop as drawn: a line after a row at a level, or a row outlined for *inside*. */
export type DropTarget =
  | { readonly kind: 'line'; readonly afterIndex: number; readonly level: number; readonly placement: Placement }
  | { readonly kind: 'inside'; readonly index: number; readonly placement: Placement }
  | { readonly kind: 'refused'; readonly index: number; readonly refusal: Refusal }
  | { readonly kind: 'unchanged' };

/** The indent step a level is drawn at: it grows to the sixth level and no further. */
export function indentOf(level: number): number {
  return Math.max(0, Math.min(level, INDENT_LEVELS));
}

/**
 * The zone of a row the pointer is over: the upper quarter places before, the lower quarter after,
 * the middle inside - where the row cannot take anything in, the middle splits at its half.
 */
export function zoneAt(offset: number, height: number, canHoldChildren: boolean): Zone {
  const share = height > 0 ? offset / height : 0;
  if (share < 0.25) return 'before';
  if (share >= 0.75) return 'after';
  if (canHoldChildren) return 'inside';
  return share < 0.5 ? 'before' : 'after';
}

/** The level the pointer's inline position asks for, counted in indent steps from the tree's start. */
export function levelAtInline(inline: number, indentPx: number): number {
  return indentPx > 0 ? Math.max(0, Math.floor(inline / indentPx)) : 0;
}

/** Signed pixels to scroll this frame: negative up, positive down, 0 outside the edges. */
export function autoScrollStep(pointer: number, start: number, end: number, maxStep = 12): number {
  if (pointer < start + AUTO_SCROLL_EDGE_PX) {
    const depth = Math.min(AUTO_SCROLL_EDGE_PX, start + AUTO_SCROLL_EDGE_PX - pointer);
    return -Math.ceil((depth / AUTO_SCROLL_EDGE_PX) * maxStep);
  }
  if (pointer > end - AUTO_SCROLL_EDGE_PX) {
    const depth = Math.min(AUTO_SCROLL_EDGE_PX, pointer - (end - AUTO_SCROLL_EDGE_PX));
    return Math.ceil((depth / AUTO_SCROLL_EDGE_PX) * maxStep);
  }
  return 0;
}

type Row = NavRow<TreeNode>;

function parentIdOf(rows: readonly Row[], index: number): string | null {
  const depth = rows[index]?.depth ?? 0;
  for (let above = index - 1; above >= 0; above--) {
    if (rows[above]!.depth < depth) return rows[above]!.node.id;
  }
  return null;
}

/** The previous sibling's id of the row at `index`, or null when it is first. */
function previousSiblingId(rows: readonly Row[], index: number): string | null {
  const depth = rows[index]!.depth;
  for (let above = index - 1; above >= 0; above--) {
    const row = rows[above]!;
    if (row.depth < depth) return null;
    if (row.depth === depth) return row.node.id;
  }
  return null;
}

/** The index just past the subtree of the row at `index`. */
function subtreeEnd(rows: readonly Row[], index: number): number {
  const depth = rows[index]!.depth;
  let end = index + 1;
  while (end < rows.length && rows[end]!.depth > depth) end++;
  return end;
}

/** The deepest level below the row a placement under it may reach (its own subtree's depth). */
function subtreeHeight(node: TreeNode): number {
  return 1 + Math.max(0, ...(node.children ?? []).map(subtreeHeight));
}

function isSamePlace(rows: readonly Row[], dragIndex: number, placement: Placement): boolean {
  return parentIdOf(rows, dragIndex) === placement.parentId && previousSiblingId(rows, dragIndex) === placement.afterId;
}

/**
 * Where a row carried over the row at `overIndex` would land.
 *
 * `rows` is the tree as drawn (`flattenTree`); `dragged` the carried node, which need not be a row
 * of it (an entry from the content region). `pointerLevel` is `levelAtInline`'s answer and only
 * matters below the last row of a branch, where moving to the start outdents.
 */
export function dropTarget(
  rows: readonly Row[],
  dragged: TreeNode,
  overIndex: number,
  zone: Zone,
  pointerLevel = Number.POSITIVE_INFINITY,
): DropTarget {
  const over = rows[overIndex];
  if (!over) return { kind: 'unchanged' };

  // An entry from the content region lands only on a collection row, and always inside it.
  if (dragged.part === 'entries') {
    return over.node.acceptsEntries
      ? { kind: 'inside', index: overIndex, placement: { parentId: over.node.id, afterId: null } }
      : { kind: 'refused', index: overIndex, refusal: 'other-part' };
  }
  if ((over.node.part ?? '') !== (dragged.part ?? '')) return { kind: 'refused', index: overIndex, refusal: 'other-part' };

  const dragIndex = rows.findIndex((row) => row.node.id === dragged.id);
  if (dragIndex >= 0 && overIndex >= dragIndex && overIndex < subtreeEnd(rows, dragIndex)) {
    return overIndex === dragIndex ? { kind: 'unchanged' } : { kind: 'refused', index: overIndex, refusal: 'self' };
  }

  const height = subtreeHeight(dragged);
  const canHold = over.node.canHoldChildren ?? true;

  if (zone === 'inside') {
    if (!canHold) return { kind: 'refused', index: overIndex, refusal: 'no-children' };
    if (over.depth + 1 + height > MAX_DEPTH) return { kind: 'refused', index: overIndex, refusal: 'too-deep' };
    return { kind: 'inside', index: overIndex, placement: { parentId: over.node.id, afterId: null } };
  }

  let placement: Placement;
  let level: number;
  let afterIndex: number;

  if (zone === 'before') {
    placement = { parentId: parentIdOf(rows, overIndex), afterId: previousSiblingId(rows, overIndex) };
    level = over.depth;
    afterIndex = overIndex - 1;
  } else if (over.isExpanded) {
    // After an open branch is its first child: the line stands where the eye already is.
    if (!canHold) return { kind: 'refused', index: overIndex, refusal: 'no-children' };
    placement = { parentId: over.node.id, afterId: null };
    level = over.depth + 1;
    afterIndex = overIndex;
  } else {
    // Below the last row of a branch the pointer's level may climb to an ancestor's: outdent.
    const next = rows[overIndex + 1];
    const floor = next ? Math.min(next.depth, over.depth) : 0;
    level = Math.max(floor, Math.min(over.depth, pointerLevel));
    let anchor = overIndex;
    while (rows[anchor]!.depth > level) {
      let up = anchor - 1;
      while (up >= 0 && rows[up]!.depth >= rows[anchor]!.depth) up--;
      anchor = up;
    }
    placement = { parentId: parentIdOf(rows, anchor), afterId: rows[anchor]!.node.id };
    afterIndex = overIndex;
  }

  if (placement.afterId === dragged.id) return { kind: 'unchanged' };
  if (dragIndex >= 0 && isSamePlace(rows, dragIndex, placement)) return { kind: 'unchanged' };
  if (level + height > MAX_DEPTH) return { kind: 'refused', index: overIndex, refusal: 'too-deep' };
  return { kind: 'line', afterIndex, level, placement };
}

/** What a key press asks of the focused row: a placement, a refusal, or nothing (not a move key). */
export type MoveIntent = { readonly placement: Placement } | { readonly refusal: Refusal } | null;

export interface MoveKey {
  readonly key: string;
  readonly altKey: boolean;
  readonly ctrlKey?: boolean;
  readonly metaKey?: boolean;
}

function findPath(nodes: readonly TreeNode[], id: string, trail: TreeNode[] = []): TreeNode[] | null {
  for (const node of nodes) {
    if (node.id === id) return [...trail, node];
    const found = findPath(node.children ?? [], id, [...trail, node]);
    if (found) return found;
  }
  return null;
}

/**
 * `Alt+↑/↓` orders among siblings; `Alt+→/←` indents under the previous sibling (after its last
 * child) and outdents after the parent - the mirrored pair in a right-to-left page. Answered on the
 * whole tree, not the drawn rows: a folded sibling still has children to go after.
 */
export function moveIntent(event: MoveKey, nodes: readonly TreeNode[], id: string, dir: 'ltr' | 'rtl' = 'ltr'): MoveIntent {
  if (!event.altKey || event.ctrlKey || event.metaKey) return null;
  const path = findPath(nodes, id);
  if (!path) return null;
  const node = path[path.length - 1]!;
  const parent = path.length > 1 ? path[path.length - 2]! : null;
  const siblings = parent ? (parent.children ?? []) : nodes;
  const at = siblings.findIndex((sibling) => sibling.id === id);
  const parentId = parent?.id ?? null;
  const indent = dir === 'rtl' ? 'ArrowLeft' : 'ArrowRight';
  const outdent = dir === 'rtl' ? 'ArrowRight' : 'ArrowLeft';

  if (event.key === 'ArrowUp') {
    if (at <= 0) return null;
    return { placement: { parentId, afterId: at >= 2 ? siblings[at - 2]!.id : null } };
  }
  if (event.key === 'ArrowDown') {
    if (at >= siblings.length - 1) return null;
    return { placement: { parentId, afterId: siblings[at + 1]!.id } };
  }
  if (event.key === indent) {
    const previous = at > 0 ? siblings[at - 1]! : null;
    if (!previous) return null;
    if (previous.canHoldChildren === false) return { refusal: 'no-children' };
    if (path.length + subtreeHeight(node) > MAX_DEPTH) return { refusal: 'too-deep' };
    const children = previous.children ?? [];
    return { placement: { parentId: previous.id, afterId: children.length ? children[children.length - 1]!.id : null } };
  }
  if (event.key === outdent) {
    if (!parent) return null;
    const grand = path.length > 2 ? path[path.length - 3]! : null;
    return { placement: { parentId: grand?.id ?? null, afterId: parent.id } };
  }
  return null;
}

/** "Position 2 of 5 under Team": the numbers of a row among its siblings, for the announcement. */
export function positionOf(nodes: readonly TreeNode[], id: string): { position: number; of: number; parent: TreeNode | null } | null {
  const path = findPath(nodes, id);
  if (!path) return null;
  const parent = path.length > 1 ? path[path.length - 2]! : null;
  const siblings = parent ? (parent.children ?? []) : nodes;
  return { position: siblings.findIndex((sibling) => sibling.id === id) + 1, of: siblings.length, parent };
}

/**
 * The tree with one node moved to a placement - what a caller shows at once, marked *waiting*,
 * before the server answers (P-11), and what it springs back from on a refusal.
 */
export function applyPlacement<T extends TreeNode>(nodes: readonly T[], id: string, placement: Placement): T[] {
  let moving: T | null = null;
  const without = (list: readonly T[]): T[] =>
    list.flatMap((node) => {
      if (node.id === id) {
        moving = node;
        return [];
      }
      return node.children ? [{ ...node, children: without(node.children as readonly T[]) }] : [node];
    });
  const pruned = without(nodes);
  if (!moving) return [...nodes];
  const insert = (list: readonly T[]): T[] => {
    const at = placement.afterId === null ? 0 : list.findIndex((node) => node.id === placement.afterId) + 1;
    return [...list.slice(0, at), moving!, ...list.slice(at)];
  };
  if (placement.parentId === null) return insert(pruned);
  const into = (list: readonly T[]): T[] =>
    list.map((node) =>
      node.id === placement.parentId
        ? { ...node, children: insert((node.children ?? []) as readonly T[]) }
        : node.children
          ? { ...node, children: into(node.children as readonly T[]) }
          : node,
    );
  return into(pruned);
}

/** The visible rows of a tree, re-exported so a caller of this module needs one import. */
export function rowsOf(nodes: readonly TreeNode[], expanded: readonly string[]): Row[] {
  return flattenTree(nodes, expanded);
}

/** The ids to open so `id` is visible: its ancestors, outermost first (the current row's branch). */
export function ancestorsOf(nodes: readonly TreeNode[], id: string): string[] {
  return (findPath(nodes, id) ?? []).slice(0, -1).map((node) => node.id);
}

/**
 * The rail's peek: open after the pointer rests `delay` on the rail, close when it leaves both the
 * rail and the peek. Timers are injected, so the test runs it without a clock.
 */
export function createPeek(
  delay: number,
  onchange: (isOpen: boolean) => void,
  timers: { set: (fn: () => void, ms: number) => unknown; clear: (handle: unknown) => void } = {
    set: (fn, ms) => setTimeout(fn, ms),
    clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
  },
) {
  let pending: unknown = null;
  let isOpen = false;
  const set = (open: boolean) => {
    if (open !== isOpen) {
      isOpen = open;
      onchange(open);
    }
  };
  return {
    enter() {
      if (isOpen || pending !== null) return;
      pending = timers.set(() => {
        pending = null;
        set(true);
      }, delay);
    },
    leave() {
      if (pending !== null) timers.clear(pending);
      pending = null;
      set(false);
    },
    get isOpen() {
      return isOpen;
    },
  };
}

/**
 * The tree narrowed by the filter (§2.4): a node whose name holds the words, of the kind chosen
 * (`all` passes every kind), shown with its ancestors; `open` names the branches to draw open so
 * each match is visible. Without words or kind the tree is answered as it is.
 */
export function filterTree(
  nodes: readonly TreeNode[],
  words: string,
  kind = 'all',
): { nodes: TreeNode[]; open: string[]; count: number } {
  const needle = words.trim().toLocaleLowerCase();
  if (!needle && kind === 'all') return { nodes: [...nodes], open: [], count: -1 };
  const open: string[] = [];
  let count = 0;
  const walk = (list: readonly TreeNode[]): TreeNode[] =>
    list.flatMap((node) => {
      const children = walk(node.children ?? []);
      const matches = (!needle || node.label.toLocaleLowerCase().includes(needle)) && (kind === 'all' || node.kind === kind);
      if (matches) count++;
      if (!matches && children.length === 0) return [];
      if (children.length > 0) open.push(node.id);
      return [{ ...node, children }];
    });
  return { nodes: walk(nodes), open, count };
}
