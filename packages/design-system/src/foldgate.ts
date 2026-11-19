// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether a folding input (the empty Composer) stands open. It folds once the focus has left it,
 * but a press elsewhere holds it open until that press has clicked: folding at the blur moves what
 * stands below, and the release then lands where the pressed control no longer is.
 */
export class FoldGate {
  #isFocused = false;
  #isPressed = false;
  #isWaiting = false;

  get isOpen(): boolean {
    return this.#isFocused || this.#isWaiting;
  }

  focusIn(): void {
    this.#isFocused = true;
    this.#isWaiting = false;
  }

  focusOut(): void {
    this.#isFocused = false;
    if (this.#isPressed) this.#isWaiting = true;
  }

  /** A pointer went down anywhere, before the focus moves. */
  press(): void {
    this.#isPressed = true;
  }

  /** The press has clicked, or ended without one: what waited for it folds now. */
  settle(): void {
    this.#isPressed = false;
    this.#isWaiting = false;
  }
}
