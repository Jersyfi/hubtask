// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where the page stands after a move (the navigation concept's review, R-15): a new screen starts
 * at its top, *back* and *forward* return to where the reader was on that entry, and a change of
 * the query alone - the detail pane's `?item=`, a layout - keeps the place, because the screen is
 * the same one. The window is what scrolls; the sidebar and a filled page's panels scroll on their
 * own and are never touched. Focus is not moved here: the frame's announcer says a new screen.
 *
 * The browser's own restoration is switched off (`history.scrollRestoration = 'manual'`): it
 * restores before the screen has drawn, onto a page too short to hold the position.
 */

import type { Move } from '../router.ts';

export type Landing = 'top' | 'restore' | 'keep';

/** What a move does to the scroll position: pure, the tests' subject. */
export function landingOf(from: { readonly path: string } | undefined, to: { readonly path: string }, move: Move): Landing {
  if (move === 'pop') return 'restore';
  if (!from || move === 'replace' || from.path === to.path) return 'keep';
  return 'top';
}

/** What the memory needs of a window - the real one, or a test's. */
export interface Scroller {
  readonly scrollY: number;
  scrollTo(x: number, y: number): void;
  /** How far down the page can scroll now. */
  reach(): number;
  /** The next frame; `cancel` drops it. */
  frame(run: () => void): () => void;
}

/** How many frames a restore waits for a screen to grow tall enough. */
const RESTORE_FRAMES = 90;

/**
 * The positions of the session's entries, keyed by the router's entry key. One instance for the
 * application; `left` is called before the screen changes, `arrived` once it has.
 */
export class ScrollMemory {
  readonly #at = new Map<string, number>();
  readonly #scroller: Scroller;
  #pending: (() => void) | undefined;

  constructor(scroller: Scroller) {
    this.#scroller = scroller;
  }

  /** The entry being left: its position is kept for a return. */
  left(entry: string): void {
    this.#pending?.();
    this.#pending = undefined;
    this.#at.set(entry, this.#scroller.scrollY);
  }

  /** The entry arrived at, once its screen is drawn. */
  arrived(entry: string, landing: Landing): void {
    if (landing === 'keep') return;
    if (landing === 'top') {
      this.#scroller.scrollTo(0, 0);
      return;
    }
    const target = this.#at.get(entry) ?? 0;
    // A screen that reads its content draws it later: the position is tried again each frame until
    // the page is tall enough, or the frames run out and it stands as far down as it can.
    let frames = 0;
    const attempt = () => {
      this.#scroller.scrollTo(0, Math.min(target, this.#scroller.reach()));
      if (this.#scroller.reach() >= target || ++frames >= RESTORE_FRAMES) {
        this.#pending = undefined;
        return;
      }
      this.#pending = this.#scroller.frame(attempt);
    };
    attempt();
  }

  /** The reader scrolled while a restore waited: theirs wins. */
  interrupted(): void {
    this.#pending?.();
    this.#pending = undefined;
  }
}

/** The document's window as a `Scroller`. */
export function windowScroller(): Scroller {
  return {
    get scrollY() {
      return window.scrollY;
    },
    scrollTo: (x, y) => window.scrollTo(x, y),
    reach: () => Math.max(0, document.documentElement.scrollHeight - window.innerHeight),
    frame: (run) => {
      const id = requestAnimationFrame(run);
      return () => cancelAnimationFrame(id);
    },
  };
}
