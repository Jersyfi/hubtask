// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A dialog's own leave guard (page-system §5.2, *Leave with changes*): closed on ✕, Escape or the
 * backdrop while it holds changes, it stays open and asks *Discard changes?* - the one Dialog S
 * allowed over another. `Dialog`'s `onDismiss` asks this; a close by an action never does. The
 * page's leave guard (`frame/leave.ts`) is the same question for a move away from a page.
 */
export class DiscardModel {
  #isAsking = false;
  readonly #hasChanges: () => boolean;

  constructor(hasChanges: () => boolean) {
    this.#hasChanges = hasChanges;
  }

  /** Whether the confirmation is open. */
  get isAsking(): boolean {
    return this.#isAsking;
  }

  /** `Dialog`'s `onDismiss`: `true` lets it close; `false` keeps it open and asks. */
  dismiss(): boolean {
    if (!this.#hasChanges()) return true;
    this.#isAsking = true;
    return false;
  }

  /** The person keeps their changes: the confirmation closes, the dialog stays. */
  keep(): void {
    this.#isAsking = false;
  }

  /** The person drops them: the confirmation closes and the caller closes the dialog. */
  discard(close: () => void): void {
    this.#isAsking = false;
    close();
  }
}
