// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The pointer path, built against the commands rather than beside them.
 *
 * It reaches the same call: a drag ends by naming a **position**, which is the number
 * `rankTarget` already answers with, so a menu item and a gesture cannot come to disagree about
 * where the fourth row lands. Everything about the position that is arithmetic lives in
 * `@hubtask/design-system`'s `reorder.ts` and is tested under `node --test`; what is left here is
 * the part that genuinely needs a browser — the events, the capture, and the measuring.
 *
 * **Pointer Events, not the HTML drag-and-drop API.** Three reasons, in the order they matter.
 * `dragstart` fires only for a mouse in several engines and never for a touch, so the API that
 * looks like it was made for this leaves a phone with no gesture at all. Its drag image cannot be
 * styled from a design system, which means the thing the reader is moving would be the one element
 * in the product drawn by the browser. And it carries a data transfer built for dragging *between*
 * documents, which is not what a rank change is. Pointer Events are in every engine
 * [ADR-0044](../../../../docs/adr/ADR-0044-browser-support-row.md)'s row names, and `setPointerCapture`
 * is what keeps a drag alive when the pointer leaves the row it started on.
 *
 * **A drag starts on a grip and nowhere else.** A row is a link and a card is a link, and a drag
 * that began on the whole element would have to guess afterwards whether the reader meant to open
 * it — which is the guess that makes a list of links unopenable with a trackpad. The grip is a
 * picture rather than a control: the single-pointer alternative SC 2.5.7 asks for is the row's
 * menu, and it is a real one, so a second focusable element that does nothing for the keyboard
 * would be noise in the tab order rather than access.
 */

import { boundariesOf, dropIndex, hasLeftTheHandle } from '@hubtask/design-system/components';

/** The list a dragged element could land in: what it is, and the elements drawn in it. */
export interface DragLevel {
  /** What the level is, for a surface that has more than one — a board's column. */
  readonly key: string;
  /** Its elements in drawn order, the dragged one included while it is in this level. */
  readonly elements: readonly HTMLElement[];
}

export interface DragOptions {
  /**
   * What the grip belongs to: the entry, where it currently sits, and the level it sits in.
   *
   * The caller answers it because the caller already knows — it drew the list. Reading the answer
   * back out of the DOM would be this module inventing a second source for something the
   * component holds.
   */
  readonly start: (grip: HTMLElement) => { id: string; index: number; level: DragLevel } | null;
  /**
   * The level under the pointer, for a surface where a drag can leave the list it started in.
   * Absent means it cannot — a level's rows have nowhere else to go.
   */
  readonly levelAt?: (point: { x: number; y: number }) => DragLevel | null;
  /** Where it ended up. Called once, and only when the position actually changed. */
  readonly ondrop: (drop: { id: string; to: number; levelKey: string }) => void;
}

/** What a drag looks like from the outside: enough for a template, and nothing about the DOM. */
export interface Drag {
  /** The entry being dragged, or `null`. */
  readonly id: string | null;
  /** How far the pointer has travelled, as a CSS length for the transform. */
  readonly offset: string;
  /** The level the pointer is over, and the position in it the entry would take. */
  readonly levelKey: string | null;
  readonly position: number | null;
  /** Installs the one listener a drag needs. Call it from an `$effect`. */
  attach(node: HTMLElement): () => void;
}

/**
 * One level member's extent, measured now rather than remembered.
 *
 * The boundary between two members is the middle of the **gap** between them
 * (`boundariesOf`), so a position changes when the pointer has passed a row rather than when it
 * has touched it. In a list with children shown, the gap between two members of one level holds
 * that subtree — so dragging a parent past its own children is what crosses the boundary, which
 * is the honest reading of "past the row" when the row is three rows tall.
 */
const extentOf = (element: HTMLElement) => {
  const box = element.getBoundingClientRect();
  return { start: box.top, end: box.bottom };
};

export function createDrag(options: DragOptions): Drag {
  let id = $state<string | null>(null);
  let travelled = $state(0);
  let levelKey = $state<string | null>(null);
  let position = $state<number | null>(null);

  function forget() {
    id = null;
    travelled = 0;
    levelKey = null;
    position = null;
  }

  function begin(event: PointerEvent, grip: HTMLElement) {
    const target = options.start(grip);
    if (!target) return;

    const from = { x: event.clientX, y: event.clientY };
    let hasBegun = false;
    let landed = { key: target.level.key, to: target.index };

    // The capture is what keeps the drag alive when the pointer leaves the grip, which it does
    // within the first few pixels. Without it the move events stop arriving at the element that
    // started the gesture and the drag ends wherever the pointer happened to cross a boundary.
    grip.setPointerCapture(event.pointerId);

    const onMove = (move: PointerEvent) => {
      if (!hasBegun) {
        if (!hasLeftTheHandle(from, { x: move.clientX, y: move.clientY })) return;
        hasBegun = true;
        id = target.id;
      }
      travelled = move.clientY - from.y;

      const level = options.levelAt?.({ x: move.clientX, y: move.clientY }) ?? target.level;
      landed = {
        key: level.key,
        to: dropIndex(move.clientY, boundariesOf(level.elements.map(extentOf))),
      };
      levelKey = landed.key;
      position = landed.to;
    };

    const finish = () => {
      grip.removeEventListener('pointermove', onMove);
      grip.removeEventListener('pointerup', finish);
      grip.removeEventListener('pointercancel', finish);
      if (grip.hasPointerCapture(event.pointerId)) grip.releasePointerCapture(event.pointerId);

      const moved = hasBegun && (landed.key !== target.level.key || landed.to !== target.index);
      forget();
      // A drag that ends where it began writes nothing. It is the ordinary outcome of thinking
      // better of it, and a request for the position an entry already holds is a request that
      // changes nothing and still costs a round trip.
      if (moved) options.ondrop({ id: target.id, to: landed.to, levelKey: landed.key });
    };

    grip.addEventListener('pointermove', onMove);
    grip.addEventListener('pointerup', finish);
    grip.addEventListener('pointercancel', finish);
  }

  return {
    get id() {
      return id;
    },
    get offset() {
      return `${travelled}px`;
    },
    get levelKey() {
      return levelKey;
    },
    get position() {
      return position;
    },
    attach(node: HTMLElement) {
      const onPointerDown = (event: PointerEvent) => {
        // The primary button only. A secondary press is a context menu, and a middle one is a
        // paste on X11 — neither is somebody asking to move a row.
        if (event.button !== 0) return;
        const grip = (event.target as Element | null)?.closest?.('[data-grip]');
        if (grip instanceof HTMLElement) begin(event, grip);
      };

      node.addEventListener('pointerdown', onPointerDown);
      return () => {
        node.removeEventListener('pointerdown', onPointerDown);
        forget();
      };
    },
  };
}
