// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Copy and paste - within a board and between boards - and groups. What is copied is the native
 * document's shape (`hubtask.whiteboard`, the one ExportWhiteboard writes), so a selection copied
 * on one board pastes into another, and a file exported pastes as well. Pasted objects take new
 * identifiers; connectors and groups are mapped to them.
 */

import { bounds, type Point } from './geometry.ts';
import { MAX_ELEMENTS, type Element, type ElementKind } from './scene.ts';

export const CLIPBOARD_FORMAT = 'hubtask.whiteboard';

export interface Copied {
  format: typeof CLIPBOARD_FORMAT;
  version: 1;
  elements: Element[];
}

/** The chosen objects together with their groups' other members. */
export function withGroups(elements: readonly Element[], ids: readonly string[]): string[] {
  const chosen = new Set(ids);
  const groups = new Set(elements.filter((e) => chosen.has(e.id) && e.group).map((e) => e.group as string));
  if (groups.size === 0) return [...ids];
  return elements.filter((e) => chosen.has(e.id) || (e.group && groups.has(e.group))).map((e) => e.id);
}

/** The chosen objects as copied: themselves and the connectors between two of them, in drawing order. */
export function copy(elements: readonly Element[], ids: readonly string[]): Copied {
  const chosen = new Set(withGroups(elements, ids));
  const out = elements.filter((e) =>
    e.kind === 'CONNECTOR' ? chosen.has(e.id) || (chosen.has(e.from ?? '') && chosen.has(e.to ?? '')) : chosen.has(e.id),
  );
  return { format: CLIPBOARD_FORMAT, version: 1, elements: out.map((e) => structuredClone(e)) };
}

const KINDS: readonly ElementKind[] = ['STICKY', 'SHAPE', 'TEXT', 'CONNECTOR', 'FRAME', 'CARD', 'STROKE', 'LINK'];

/** Reads copied objects from the clipboard's words; undefined for anything else. */
export function readCopied(text: string): Copied | undefined {
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    return undefined;
  }
  const raw = value as { format?: unknown; elements?: unknown };
  if (!raw || raw.format !== CLIPBOARD_FORMAT || !Array.isArray(raw.elements)) return undefined;
  const elements = (raw.elements as unknown[]).filter((e): e is Element => {
    const element = e as Partial<Element>;
    return typeof element?.id === 'string' && KINDS.includes(element.kind as ElementKind) &&
      [element.x ?? 0, element.y ?? 0, element.width ?? 0, element.height ?? 0].every((n) => typeof n === 'number' && Number.isFinite(n));
  });
  return { format: CLIPBOARD_FORMAT, version: 1, elements: elements.slice(0, MAX_ELEMENTS) };
}

/**
 * The copied objects as new objects centred at a point: new identifiers, a connector attached to
 * the copies of its ends (an end not copied is left a free point where it was), a group made new
 * so the copy does not join the original's.
 */
export function paste(copied: Copied, at: Point, newId: () => string): Element[] {
  const area = bounds(copied.elements);
  if (!area) return [];
  const dx = Math.round(at.x - (area.left + area.right) / 2);
  const dy = Math.round(at.y - (area.top + area.bottom) / 2);
  const ids = new Map(copied.elements.map((e) => [e.id, newId()]));
  const groups = new Map<string, string>();
  const groupOf = (group: string) => {
    if (!groups.has(group)) groups.set(group, newId());
    return groups.get(group) as string;
  };
  return copied.elements.map((e) => {
    const next: Element = { ...structuredClone(e), id: ids.get(e.id) as string, x: e.x + dx, y: e.y + dy };
    if (e.group) next.group = groupOf(e.group);
    if (e.kind === 'CONNECTOR') {
      next.from = e.from && ids.has(e.from) ? ids.get(e.from) : null;
      next.to = e.to && ids.has(e.to) ? ids.get(e.to) : null;
    }
    return next;
  });
}
