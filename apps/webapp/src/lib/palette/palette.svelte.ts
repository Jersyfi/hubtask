// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether the palette is open, who opened it, and the rows other features contribute.
 *
 * One palette with two drawings (design-system.md §11.4): from `medium` up its field is the bar's
 * and the list opens under it; below, where the bar has no field, it opens in a dialog. The frame
 * mounts one of the two; this is what both read and what the shortcuts and the bottom bar's
 * *Search* call, so neither needs to know which drawing is up.
 *
 * **The opener is remembered only when something else opened it** - a shortcut, the bottom bar.
 * Escape returns focus there (UC-WRK-27 check 5). Somebody who clicked into the bar's field opened
 * it from the field, and that is where focus stays.
 */

import { untrack } from 'svelte';

/** One row a feature adds for the words typed - *Ask*, once it exists (UC-AI-08). */
export interface SourcedRow {
  /** Unique within its source. */
  readonly id: string;
  readonly label: string;
  readonly run: (words: string) => void;
}

/**
 * A feature that answers the words with rows of its own. It returns none where it does not apply
 * - *Ask* without consent to AI - and then nothing about it is drawn (UC-AI-08 check 1).
 */
export interface PaletteSource {
  rows(words: string): readonly SourcedRow[];
}

class Palette {
  #isOpen = $state(false);
  #opener: HTMLElement | null = null;
  #sources = $state<Record<string, PaletteSource>>({});

  get isOpen(): boolean {
    return this.#isOpen;
  }

  /**
   * Opens it from somewhere else - a shortcut, the bottom bar - and remembers where focus was, to
   * return it there. Opening it again while open closes it: the key that opened it is the key
   * that puts it away.
   */
  summon(): void {
    if (this.#isOpen) {
      this.close();
      return;
    }
    const active = document.activeElement;
    this.#opener = active instanceof HTMLElement && active !== document.body ? active : null;
    this.#isOpen = true;
  }

  /** Opened from its own field: nothing to return to but the field. */
  enter(): void {
    if (this.#isOpen) return;
    this.#opener = null;
    this.#isOpen = true;
  }

  /**
   * Closes it and, where something else opened it, returns focus there. What a row then does runs
   * after, on the next microtask (`perform`), so a dialog it opens records that element as its
   * opener and returns there in turn.
   */
  close(): void {
    this.#isOpen = false;
    const opener = this.#opener;
    this.#opener = null;
    if (opener?.isConnected) opener.focus();
  }

  /**
   * Closes it because focus went somewhere else - a press elsewhere on the page. Focus is not
   * pulled back to the opener: the reader has just put it where they want it.
   */
  dismiss(): void {
    this.#isOpen = false;
    this.#opener = null;
  }

  /** Closes, returns focus, then runs what was chosen. */
  perform(run: () => void): void {
    this.close();
    queueMicrotask(run);
  }

  /** Whether something else opened it - then Escape has a place to return focus to. */
  get hasOpener(): boolean {
    return this.#opener !== null;
  }

  /** Adds a feature's rows until the returned cleanup runs. */
  provide(id: string, source: PaletteSource): () => void {
    untrack(() => {
      this.#sources = { ...this.#sources, [id]: source };
    });
    return () =>
      untrack(() => {
        if (this.#sources[id] !== source) return;
        const { [id]: _gone, ...rest } = this.#sources;
        this.#sources = rest;
      });
  }

  /** Every contributed row for these words, keyed by source so two sources cannot collide. */
  sourced(words: string): readonly (SourcedRow & { readonly key: string })[] {
    return Object.entries(this.#sources).flatMap(([id, source]) =>
      source.rows(words).map((row) => ({ ...row, key: `${id}:${row.id}` })),
    );
  }
}

export const palette = new Palette();
