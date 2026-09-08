// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The entries a reader has picked out, held once for the screen they are on.
 *
 * A module rather than a prop drilled through the list, the board and the bar: all three are
 * looking at one selection, and passing it down three trees would make "how many are selected" a
 * question with three answers. The arithmetic is in `selection.ts`, pure and tested; this is what
 * holds the result and the anchor a range is measured from.
 *
 * **It is cleared when the screen changes.** A selection is about what is in front of somebody, so
 * carrying it from one collection to the next would leave a bar acting on entries nobody can see.
 * The view that owns the screen calls `clear`; nothing here guesses at navigation.
 */

import { rangeTo, stillVisible, toggle, toggleAll } from './selection.ts';

class Selection {
  #ids = $state<readonly string[]>([]);
  /** Where a range starts: the last entry picked without one. */
  #anchor = $state<string | undefined>(undefined);
  /**
   * What is drawn, as the layout last reported it.
   *
   * Held so that a control *outside* the list — "select every entry on screen", in the bar above
   * it — can mean the same thing the list means by it. The list and the board are the only writers
   * and they write it on every render, which is what keeps it from going stale.
   */
  #visible = $state<readonly string[]>([]);

  get ids(): readonly string[] {
    return this.#ids;
  }

  get count(): number {
    return this.#ids.length;
  }

  has(id: string): boolean {
    return this.#ids.includes(id);
  }

  /**
   * One pick, however it was made.
   *
   * `range` is the shift a pointer holds and the shift a keyboard holds — one path, so the two can
   * never drift apart. A plain pick moves the anchor; a range does not, so a reader can extend the
   * same range twice.
   */
  pick(visible: readonly string[], id: string, options: { range?: boolean } = {}): void {
    if (options.range) {
      this.#ids = rangeTo(visible, this.#ids, this.#anchor, id);
      return;
    }
    this.#ids = toggle(this.#ids, id);
    this.#anchor = id;
  }

  /** Everything on screen, or nothing. */
  all(visible: readonly string[]): void {
    this.#ids = toggleAll(visible, this.#ids);
    this.#anchor = undefined;
  }

  /** The same, from a control that is not inside the list. */
  allVisible(): void {
    this.all(this.#visible);
  }

  /** Whether everything drawn is picked, which is what the select-all control shows. */
  get isAllVisible(): boolean {
    return this.#visible.length > 0 && this.#visible.every((id) => this.#ids.includes(id));
  }

  get visibleCount(): number {
    return this.#visible.length;
  }

  /** Drops what is no longer drawn, and keeps the drawn order. Safe to call on every render. */
  keepVisible(visible: readonly string[]): void {
    this.#visible = visible;
    const kept = stillVisible(visible, this.#ids);
    if (kept.length !== this.#ids.length) this.#ids = kept;
  }

  clear(): void {
    this.#ids = [];
    this.#anchor = undefined;
  }
}

export const selection = new Selection();
