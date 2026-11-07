// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Moving in the sidebar (the navigation concept's §5, UC-WRK-38): what a placement becomes as a
 * write, where a row stands now so *Undo* can put it back, which places *Move to…* offers and why
 * the others are greyed, and a flat list ordered anew. Pure; `WorkspaceNav` draws and writes.
 */

import type { Refusal, TreePlacement as Placement, TreeNode } from '@hubtask/design-system/components';

/** Where a node stands now: its parent and the sibling before it - what *Undo* places it back at. */
export function placementOf(nodes: readonly TreeNode[], id: string, parentId: string | null = null): Placement | undefined {
  for (let at = 0; at < nodes.length; at++) {
    const node = nodes[at]!;
    if (node.id === id) return { parentId, afterId: at > 0 ? nodes[at - 1]!.id : null };
    const inner = placementOf(node.children ?? [], id, node.id);
    if (inner) return inner;
  }
  return undefined;
}

/** `movePage`'s body: absent fields are the top of the space and the first place (`PageMove`). */
export function pageMoveBody(placement: Placement): { parent_id?: string; after_id?: string } {
  return {
    ...(placement.parentId ? { parent_id: placement.parentId } : {}),
    ...(placement.afterId ? { after_id: placement.afterId } : {}),
  };
}

/** `moveWhiteboard`'s body: null is the top of the space and the first place (`WhiteboardMove`). */
export function boardMoveBody(placement: Placement): { parent_page_id: string | null; after_id: string | null } {
  return { parent_page_id: placement.parentId, after_id: placement.afterId };
}

/** One place *Move to…* lists: the top of *Contents* (`id` null) or a page, greyed with its reason. */
export interface PickerPlace {
  readonly id: string | null;
  readonly label: string;
  readonly depth: number;
  readonly refusal?: Refusal;
  /** Where the row lands there: after the place's last child. */
  readonly placement: Placement;
}

const heightOf = (node: TreeNode): number => 1 + Math.max(0, ...(node.children ?? []).map(heightOf));

/**
 * The places a content row may be moved to (§5.4, *Move to…*): the top, then every node in tree
 * order. A whiteboard takes nothing in, the row itself and what is under it are refused, and so is
 * a place that would nest it deeper than `maxDepth`. Whiteboards stand greyed rather than absent,
 * so the reason is read where the eye looks for them.
 */
export function pickerPlaces(nodes: readonly TreeNode[], movingId: string, topLabel: string, maxDepth = 12): PickerPlace[] {
  const moving = findNode(nodes, movingId);
  const height = moving ? heightOf(moving) : 1;
  const lastOf = (list: readonly TreeNode[]) => {
    const others = list.filter((each) => each.id !== movingId);
    return others.length > 0 ? others[others.length - 1]!.id : null;
  };
  const places: PickerPlace[] = [{ id: null, label: topLabel, depth: 0, placement: { parentId: null, afterId: lastOf(nodes) } }];
  const walk = (list: readonly TreeNode[], depth: number, isUnderMoving: boolean) => {
    for (const node of list) {
      const isSelf = isUnderMoving || node.id === movingId;
      const refusal: Refusal | undefined = isSelf
        ? 'self'
        : node.canHoldChildren === false
          ? 'no-children'
          : depth + 1 + height > maxDepth
            ? 'too-deep'
            : undefined;
      places.push({ id: node.id, label: node.label, depth, ...(refusal ? { refusal } : {}), placement: { parentId: node.id, afterId: lastOf(node.children ?? []) } });
      walk(node.children ?? [], depth + 1, isSelf);
    }
  };
  walk(nodes, 0, false);
  return places;
}

function findNode(nodes: readonly TreeNode[], id: string): TreeNode | undefined {
  for (const node of nodes) {
    if (node.id === id) return node;
    const inner = findNode(node.children ?? [], id);
    if (inner) return inner;
  }
  return undefined;
}

/** A flat list with one id moved after `afterId` (null is first): the starred list, the products. */
export function reordered<T extends string>(ids: readonly T[], id: T, afterId: string | null): T[] {
  const without = ids.filter((each) => each !== id);
  if (without.length === ids.length) return [...ids];
  const at = afterId === null ? 0 : without.indexOf(afterId as T) + 1;
  return [...without.slice(0, at), id, ...without.slice(at)];
}
