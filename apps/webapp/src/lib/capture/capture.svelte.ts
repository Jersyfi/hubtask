// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether the capture field is open, and where it was opened (UC-WRK-26 check 1, design-system.md
 * §11.8). Capture is the one way an entry is added: `Q`, the bar's *Capture* and the bottom bar's ＋
 * call `summon`; a collection's *Add an entry*, the end of a list, a row's ＋ and a column's foot call
 * `open` with the door they are - the collection, the entry it goes under, the level, the column -
 * so every way in is one press and there is one field to keep. The frame mounts the one
 * `CaptureDialog` that reads this. Focus going back to where it was is the `Dialog`'s.
 */

/** Where an add control opened capture: what it already knows about the entry to be. */
export interface Door {
  /** The collection the entry goes into. */
  readonly collectionId?: string;
  /** The entry it goes under, and that entry's title for the chip that says so. */
  readonly parentId?: string;
  readonly parentTitle?: string;
  /** The levels the control may add, in the manifest's order; the first is chosen. */
  readonly types?: readonly string[];
  /** The board column it lands in. */
  readonly bucketId?: string;
  /** The mode the dialog opens in. */
  readonly mode?: Mode;
  /** Whether the field stays open after Enter, for a list typed one line after another. */
  readonly keepsOpen?: boolean;
}

/** One entry, several lines as several entries, from a template, or a file imported. */
export type Mode = 'one' | 'lines' | 'template' | 'import';

class Capture {
  #isOpen = $state(false);
  #door = $state<Door | undefined>(undefined);

  get isOpen(): boolean {
    return this.#isOpen;
  }

  /** The door it was opened from; none for `Q`, the bar and the bottom bar. */
  get door(): Door | undefined {
    return this.#door;
  }

  /** Opens it; pressed again while open, it closes - the key that opened it puts it away. */
  summon(): void {
    if (this.#isOpen) {
      this.close();
      return;
    }
    this.#door = undefined;
    this.#isOpen = true;
  }

  /** Opens it pointed at where the control stands. */
  open(door: Door): void {
    this.#door = door;
    this.#isOpen = true;
  }

  close(): void {
    this.#isOpen = false;
    this.#door = undefined;
  }
}

export const capture = new Capture();
