// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether the reader may leave a screen that holds unsaved changes (user-experience.md §5.4): a
 * screen holds its count while it has changes, the router asks before it moves, and a held move
 * waits as `pending` until the reader stays or leaves. The rules are here and carry the tests;
 * `leave.svelte.ts` is the reactive half.
 */
export class LeaveModel {
  readonly #holders = new Set<() => number>();
  #pending: string | undefined;
  #released = false;

  /** A screen's count of unsaved changes, asked whenever a move is; the returned call lets go. */
  hold(count: () => number): () => void {
    this.#holders.add(count);
    return () => {
      this.#holders.delete(count);
    };
  }

  /** How many changes every holding screen has unsaved. */
  get unsaved(): number {
    let sum = 0;
    for (const count of this.#holders) sum += Math.max(0, count());
    return sum;
  }

  /** The move that waits for the reader's answer, if one does. */
  get pending(): string | undefined {
    return this.#pending;
  }

  /**
   * The router's question before it moves: true lets the move go; false holds it as `pending`.
   * A move the reader already chose to make (`leave`) is let through once.
   */
  admits(path: string): boolean {
    if (this.#released || this.unsaved === 0) return true;
    this.#pending = path;
    return false;
  }

  /** The reader stays: the held move is dropped and the changes stay where they are. */
  stay(): void {
    this.#pending = undefined;
  }

  /**
   * The reader leaves: the held move is made through `go`, past this guard, and the changes are
   * dropped with the screen that held them.
   */
  leave(go: (path: string) => void): void {
    const path = this.#pending;
    this.#pending = undefined;
    if (path === undefined) return;
    this.#released = true;
    try {
      go(path);
    } finally {
      this.#released = false;
    }
  }
}
