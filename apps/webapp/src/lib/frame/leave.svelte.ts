// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The leave guard every settings screen shares (user-experience.md §5.4): `leaveGuard.hold(() =>
 * count)` while a screen is open, the router's `admits` before it moves, and `LeaveDialog` asking
 * the reader about a held move. The rules are `leave.ts`'s; this is the reactive half and the
 * browser's own question on a reload or a closed tab.
 */

import { LeaveModel } from './leave.ts';

class LeaveGuard {
  #model = new LeaveModel();
  #pending = $state<string | undefined>(undefined);
  #go: ((path: string) => void) | undefined;

  get pending(): string | undefined {
    return this.#pending;
  }

  get unsaved(): number {
    return this.#model.unsaved;
  }

  hold(count: () => number): () => void {
    return this.#model.hold(count);
  }

  /** The router's answer to give, and how it moves once the reader chose to leave. */
  connect(go: (path: string) => void): () => void {
    this.#go = go;
    // A reload or a closed tab cannot wait for a dialog of ours: the browser asks its own question.
    const onUnload = (event: BeforeUnloadEvent) => {
      if (this.#model.unsaved === 0) return;
      event.preventDefault();
    };
    window.addEventListener('beforeunload', onUnload);
    return () => {
      window.removeEventListener('beforeunload', onUnload);
      this.#go = undefined;
    };
  }

  admits(path: string): boolean {
    const admitted = this.#model.admits(path);
    this.#pending = this.#model.pending;
    return admitted;
  }

  stay(): void {
    this.#model.stay();
    this.#pending = undefined;
  }

  leave(): void {
    const go = this.#go;
    this.#pending = undefined;
    if (go) this.#model.leave(go);
    else this.#model.stay();
  }
}

export const leaveGuard = new LeaveGuard();
