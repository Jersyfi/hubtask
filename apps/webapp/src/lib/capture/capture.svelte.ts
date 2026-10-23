// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether the capture field is open (UC-WRK-26 check 1). `Q`, the bar's *Capture* and the bottom
 * bar's ＋ call `summon`; the frame mounts the one `CaptureDialog` that reads this. One owner, so two
 * ways in cannot open two fields. Focus going back to where it was is the `Dialog`'s.
 */

class Capture {
  #isOpen = $state(false);

  get isOpen(): boolean {
    return this.#isOpen;
  }

  /** Opens it; pressed again while open, it closes - the key that opened it puts it away. */
  summon(): void {
    this.#isOpen = !this.#isOpen;
  }

  close(): void {
    this.#isOpen = false;
  }
}

export const capture = new Capture();
