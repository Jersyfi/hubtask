// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** The *new* marks of this device (`newmark.ts`), kept where storage can be reached. */

import { MARKS_KEY, marked, readMarks, unmarked } from './newmark.ts';

function storage() {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

class NewMarks {
  #marks = $state<string[]>(readMarks(read()));

  has(module: string): boolean {
    return this.#marks.includes(module);
  }

  /** The module was switched on here: its row says *new* until its home is visited. */
  add(module: string): void {
    this.#write(marked(this.#marks, module));
  }

  /** Its home was visited. */
  seen(module: string): void {
    if (this.#marks.includes(module)) this.#write(unmarked(this.#marks, module));
  }

  #write(next: string[]) {
    this.#marks = next;
    try {
      storage()?.setItem(MARKS_KEY, JSON.stringify(next));
    } catch {
      // A device that keeps nothing keeps the mark for this tab only.
    }
  }
}

function read(): string | null {
  try {
    return storage()?.getItem(MARKS_KEY) ?? null;
  } catch {
    return null;
  }
}

export const newMarks = new NewMarks();
