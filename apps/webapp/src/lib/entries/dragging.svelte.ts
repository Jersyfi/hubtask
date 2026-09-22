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
 * **What starts a drag is the caller's to name** (`handle`), and the two surfaces name different
 * things. A list row is a link in a column of links, and a drag that began anywhere on it would
 * have to guess afterwards whether the reader meant to open it — so the list keeps its grip. A
 * **board card is the thing you move**: that is what every board is, and a 14 × 22 px grip beside
 * the card is an affordance nobody finds (ADR-0063 decision 11). The guess is answered rather than
 * avoided: a press that travels past the threshold is a drag, a press that does not is the click
 * it always was, and the click that ends a drag is swallowed so that letting go does not also open
 * the entry.
 *
 * **A coarse pointer starts on a hold, not on movement.** A finger that moves is scrolling the
 * board, so a drag that began on movement would take every scroll. 300 ms of stillness is what
 * says "this one", and it is the same hold the selection mode waits for, deliberately: one
 * gesture, one meaning.
 *
 * The single-pointer alternative SC 2.5.7 asks for is the card's own menu, and it is a real one.
 */

import { boundariesOf, dropIndex, hasLeftTheHandle } from '@hubtask/design-system/components';

/** The list a dragged element could land in: what it is, and the elements drawn in it. */
export interface DragLevel {
  /** What the level is, for a surface that has more than one — a board's column. */
  readonly key: string;
  /** Its elements in drawn order, the dragged one included while it is in this level. */
  readonly elements: readonly HTMLElement[];
}

/** How long a coarse pointer is held still before it is carrying something rather than scrolling. */
export const HOLD_MS = 300;

export interface DragOptions {
  /**
   * What a drag may start on, as a selector. The default is the grip a list draws; a board names
   * the card itself, because a card *is* the handle.
   */
  readonly handle?: string;
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
  /** How far the pointer has travelled on the block axis, as a CSS length for the transform. */
  readonly offset: string;
  /** The same on the inline axis, for a surface where a thing is carried in both (a board). */
  readonly offsetInline: string;
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
  let travelledInline = $state(0);
  let levelKey = $state<string | null>(null);
  let position = $state<number | null>(null);

  function forget() {
    id = null;
    travelled = 0;
    travelledInline = 0;
    levelKey = null;
    position = null;
  }

  /**
   * Whether the gesture that just ended was a drag, so that the click behind it can be swallowed.
   *
   * A card is a link now, and letting go of one that was carried across the board would otherwise
   * open the entry the reader had just filed. One flag, read once by the click that follows
   * immediately, and cleared by it.
   */
  let hasJustDragged = false;

  function begin(event: PointerEvent, grip: HTMLElement) {
    const target = options.start(grip);
    if (!target) return;

    const from = { x: event.clientX, y: event.clientY };
    const isCoarse = event.pointerType !== 'mouse';
    let hasBegun = false;
    /** A coarse pointer may not begin until it has been held: until then a move is a scroll. */
    let mayBegin = !isCoarse;
    let held: ReturnType<typeof setTimeout> | undefined = isCoarse
      ? setTimeout(() => {
          mayBegin = true;
        }, HOLD_MS)
      : undefined;
    let landed = { key: target.level.key, to: target.index };

    // The moves are listened for on the window rather than through `setPointerCapture`, and that
    // is the card being the handle rather than a preference. A capture taken on `pointerdown`
    // retargets the `click` that follows to the captured element, so the `<a>` inside a card never
    // receives it and a plain click opens nothing — which is what it did. The window keeps the
    // gesture alive when the pointer leaves the card, which is what the capture was for.

    const onMove = (move: PointerEvent) => {
      if (!hasBegun) {
        if (!hasLeftTheHandle(from, { x: move.clientX, y: move.clientY })) return;
        // A finger that moved before the hold was up is scrolling, and the board is what it is
        // scrolling. Let go of the gesture entirely rather than taking it.
        if (!mayBegin) {
          finish();
          return;
        }
        hasBegun = true;
        id = target.id;
      }
      travelled = move.clientY - from.y;
      travelledInline = move.clientX - from.x;

      const level = options.levelAt?.({ x: move.clientX, y: move.clientY }) ?? target.level;
      landed = {
        key: level.key,
        to: dropIndex(move.clientY, boundariesOf(level.elements.map(extentOf))),
      };
      levelKey = landed.key;
      position = landed.to;
    };

    const finish = () => {
      if (held !== undefined) {
        clearTimeout(held);
        held = undefined;
      }
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', finish);
      window.removeEventListener('pointercancel', finish);

      const moved = hasBegun && (landed.key !== target.level.key || landed.to !== target.index);
      hasJustDragged = hasBegun;
      forget();
      // A drag that ends where it began writes nothing. It is the ordinary outcome of thinking
      // better of it, and a request for the position an entry already holds is a request that
      // changes nothing and still costs a round trip.
      if (moved) options.ondrop({ id: target.id, to: landed.to, levelKey: landed.key });
    };

    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', finish);
    window.addEventListener('pointercancel', finish);
  }

  return {
    get id() {
      return id;
    },
    get offset() {
      return `${travelled}px`;
    },
    get offsetInline() {
      return `${travelledInline}px`;
    },
    get levelKey() {
      return levelKey;
    },
    get position() {
      return position;
    },
    attach(node: HTMLElement) {
      const handle = options.handle ?? '[data-grip]';
      const onPointerDown = (event: PointerEvent) => {
        // The primary button only. A secondary press is a context menu, and a middle one is a
        // paste on X11 — neither is somebody asking to move a row.
        if (event.button !== 0) return;
        const from = (event.target as Element | null)?.closest?.(handle);
        if (from instanceof HTMLElement) begin(event, from);
      };

      // The click that ends a drag, swallowed in the capture phase — before the link under it and
      // before the router. A press that never became a drag leaves the flag false and the click
      // is the one it always was.
      const onClick = (event: MouseEvent) => {
        if (!hasJustDragged) return;
        hasJustDragged = false;
        event.preventDefault();
        event.stopPropagation();
      };

      node.addEventListener('pointerdown', onPointerDown);
      node.addEventListener('click', onClick, true);
      return () => {
        node.removeEventListener('pointerdown', onPointerDown);
        node.removeEventListener('click', onClick, true);
        forget();
      };
    },
  };
}
