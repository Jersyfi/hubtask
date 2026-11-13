// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The board read as a list (work surface §3): each frame with the objects whose middle stands in
 * it, then the objects in no frame, each in reading order - rows from the top, left to right in a
 * row. Arrows and strokes are not read; what they join is.
 */

import type { Element } from '@hubtask/whiteboard';

export interface ListGroup {
  frame?: Element;
  items: Element[];
}

const READ: ReadonlySet<string> = new Set(['STICKY', 'SHAPE', 'TEXT', 'CARD', 'LINK', 'IMAGE', 'FILE']);

/** Reading order: a row is objects whose tops lie within half the first one's height. */
export function readingOrder(items: readonly Element[]): Element[] {
  const byTop = [...items].sort((a, b) => a.y - b.y || a.x - b.x);
  const rows: Element[][] = [];
  for (const e of byTop) {
    const row = rows[rows.length - 1];
    const first = row?.[0];
    if (row && first && e.y - first.y < first.height / 2) row.push(e);
    else rows.push([e]);
  }
  return rows.flatMap((row) => row.sort((a, b) => a.x - b.x));
}

export function listOfBoard(elements: readonly Element[], frames: readonly Element[]): ListGroup[] {
  const ordered = readingOrder(frames);
  const placed = new Set<string>();
  const groups: ListGroup[] = ordered.map((frame) => {
    const items = elements.filter((e) => {
      if (!READ.has(e.kind) || placed.has(e.id)) return false;
      const cx = e.x + e.width / 2;
      const cy = e.y + e.height / 2;
      return cx >= frame.x && cx <= frame.x + frame.width && cy >= frame.y && cy <= frame.y + frame.height;
    });
    for (const e of items) placed.add(e.id);
    return { frame, items: readingOrder(items) };
  });
  const loose = elements.filter((e) => READ.has(e.kind) && !placed.has(e.id));
  if (loose.length > 0) groups.push({ items: readingOrder(loose) });
  return groups;
}
