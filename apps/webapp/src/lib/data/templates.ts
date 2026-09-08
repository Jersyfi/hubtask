// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What this client decides about a template, pure so each decision is tested.
 *
 * **The tree is nested, and stays nested.** The document is "the shape the template stamps out
 * rather than a flat list with parent pointers", and it travels whole on an update — half a shape
 * is a different shape. So the editor works on the tree itself, and what is flat here is only ever
 * a *view* of it: a walk that pairs each node with its path, which is what a node-level field error
 * is addressed by.
 *
 * **Two permissions, not one.** Defining a template is shaping the workspace and asks `STRUCTURE`
 * at the template's own scope; instantiating one is ordinary work and asks `WRITE_ITEMS` in the
 * collection it lands in. A reader who may use every template and define none is the normal case
 * rather than an edge one.
 *
 * **The shape is the manifest's.** Which types may sit under which, and how deep, come from
 * `item_types[]` — so an activity is never offered children, and an installation with a fourth type
 * is edited correctly by a client that has never heard of it.
 */

import type { Capabilities, TemplateNode } from '@hubtask/sync-engine';

import { allowedChildTypes, childVerdict, type Verdict } from './capability.ts';

/** How many nodes a template may carry, as the installation reports it. */
export function nodeCapOf(limits: Record<string, unknown> | undefined): number | undefined {
  const declared = limits?.['max_template_nodes'];
  if (typeof declared !== 'number' || !Number.isFinite(declared) || declared <= 0) return undefined;
  return declared;
}

/** Every node in the tree, the root included. What the cap is counted against. */
export function countNodes(nodes: readonly TemplateNode[]): number {
  return nodes.reduce((total, node) => total + 1 + countNodes(node.children ?? []), 0);
}

/** Whether one more node fits. No cap known means yes, and the server decides. */
export function fitsInCap(nodes: readonly TemplateNode[], cap: number | undefined): boolean {
  return cap === undefined || countNodes(nodes) < cap;
}

/** One node, with where it sits and how deep. The flat view the editor draws from. */
export interface WalkedNode {
  readonly node: TemplateNode;
  /** The indices from the root down — `[0, 1]` is the second child of the first root. */
  readonly path: readonly number[];
  readonly depth: number;
  readonly parentType?: string;
}

/**
 * The tree, walked depth first, in the order it is drawn.
 *
 * The path travels with each node because that is how a node is addressed — both by the editor,
 * which has to replace one node inside a nested document, and by the server, whose field errors
 * name `/nodes/0/children/1/title`.
 */
export function walk(
  nodes: readonly TemplateNode[],
  path: readonly number[] = [],
  depth = 0,
  parentType?: string,
): readonly WalkedNode[] {
  return nodes.flatMap((node, index) => {
    const here = [...path, index];
    return [
      { node, path: here, depth, parentType },
      ...walk(node.children ?? [], here, depth + 1, node.type as string),
    ];
  });
}

/** The field path the server would name for one of this node's fields. */
export function fieldPathOf(path: readonly number[], field: string): string {
  return `/nodes/${path.join('/children/')}/${field}`;
}

/**
 * A node replaced inside the tree, without touching the rest of the shape.
 *
 * The three tree functions return a **mutable** array, which is the generated type's doing rather
 * than a preference: `TemplateNode.children` is `TemplateNode[]`, so a `readonly` result could not
 * be put back into a node. They still copy rather than mutate — every one of them builds a new
 * tree, which is what makes an editor's undo a matter of keeping the old one.
 */
export function replaceAt(
  nodes: readonly TemplateNode[],
  path: readonly number[],
  next: TemplateNode,
): TemplateNode[] {
  const [index, ...rest] = path;
  if (index === undefined) return [...nodes];
  return nodes.map((node, at) => {
    if (at !== index) return node;
    if (rest.length === 0) return next;
    return { ...node, children: replaceAt(node.children ?? [], rest, next) };
  });
}

/** A node removed, and everything under it with it. */
export function removeAt(
  nodes: readonly TemplateNode[],
  path: readonly number[],
): TemplateNode[] {
  const [index, ...rest] = path;
  if (index === undefined) return [...nodes];
  if (rest.length === 0) return nodes.filter((_, at) => at !== index);
  return nodes.map((node, at) =>
    at === index ? { ...node, children: removeAt(node.children ?? [], rest) } : node,
  );
}

/** A child added under a node, or at the root when the path is empty. */
export function addUnder(
  nodes: readonly TemplateNode[],
  path: readonly number[],
  child: TemplateNode,
): TemplateNode[] {
  if (path.length === 0) return [...nodes, child];
  const [index, ...rest] = path;
  if (index === undefined) return [...nodes];
  return nodes.map((node, at) => {
    if (at !== index) return node;
    const children = node.children ?? [];
    return {
      ...node,
      children: rest.length === 0 ? [...children, child] : addUnder(children, rest, child),
    };
  });
}

/**
 * The types that may be added under a node, as the manifest permits them at that depth.
 *
 * Empty is a complete answer: an activity takes no children, and a node already at `max_depth`
 * takes none either. The editor offers nothing there rather than offering a refusal.
 */
export function childTypesAt(
  manifest: Capabilities | undefined,
  parentType: string,
  parentDepth: number,
): readonly string[] {
  return allowedChildTypes(manifest, parentType).filter(
    (type) => childVerdict(manifest, parentType, type, parentDepth).status === 'permitted',
  );
}

/** Whether a child of that type may go under that parent, with the reason when it may not. */
export function childVerdictAt(
  manifest: Capabilities | undefined,
  parentType: string,
  childType: string,
  parentDepth: number,
): Verdict {
  return childVerdict(manifest, parentType, childType, parentDepth);
}

/**
 * Where a template applies, as one word for a reader.
 *
 * A collection's list carries its own, its hub's and the workspace-wide ones together — "exactly
 * the set a person picking one in that collection may choose from" — so each row has to say which
 * it is, or the list is three lists that look like one.
 */
export function scopeCodeOf(scopeType: string): string {
  return `app.templates.scope_${scopeType}`;
}
