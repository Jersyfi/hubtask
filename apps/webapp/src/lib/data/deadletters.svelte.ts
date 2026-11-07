// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's dead letters, as its operator reads them and runs one again (UC-INS-12,
 * alert A-07): the jobs that ran out of attempts, newest first, a page at a time.
 *
 * **Codes and moments only.** The answer never carries what a job was given; the screen shows the
 * kind, the workspace, the attempts and the code of the last failure, and nothing more is there
 * to show.
 */

import type { DeadLetter, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const DEAD_LETTERS = '/admin/dead-letters';
/** The installation's journal records every requeue. */
const JOURNAL = '/admin/journal';

type Answer = { readonly data: readonly DeadLetter[]; readonly page?: { readonly next_cursor?: string | null } };

function pathOf(cursor?: string): string {
  return cursor ? `${DEAD_LETTERS}?cursor=${encodeURIComponent(cursor)}` : DEAD_LETTERS;
}

class DeadLetters {
  #first = $state<ResourceState<Answer>>({ status: 'idle' });
  #more = $state<readonly DeadLetter[]>([]);
  #cursor = $state<string | undefined>(undefined);
  /** The ones run again in this session, gone from the list before the next read says so. */
  #requeued = $state<ReadonlySet<string>>(new Set());

  get state(): ResourceState<Answer> {
    return this.#first;
  }

  get letters(): readonly DeadLetter[] {
    const first = this.#first.status === 'ready' ? this.#first.data.data : [];
    return [...first, ...this.#more].filter((letter) => !this.#requeued.has(letter.id));
  }

  get moreAfter(): string | undefined {
    return this.#cursor;
  }

  /** Forgets what was read, with the session it was read in. */
  forget(): void {
    this.#first = { status: 'idle' };
    this.#more = [];
    this.#cursor = undefined;
    this.#requeued = new Set();
  }

  /** The first page, read and kept fresh. **From `untrack`.** */
  open(): () => void {
    return engine.subscribe<Answer>({ path: DEAD_LETTERS }, (next) => {
      this.#first = next;
      if (next.status === 'ready') {
        this.#more = [];
        this.#cursor = next.data.page?.next_cursor ?? undefined;
        this.#requeued = new Set();
      }
    });
  }

  /** The next page, appended. Cursor pagination, never page numbers. */
  async more(): Promise<void> {
    const cursor = this.#cursor;
    if (!cursor) return;
    const next = await engine.refresh<Answer>({ path: pathOf(cursor) });
    if (next.status === 'failed') throw next.error;
    if (next.status !== 'ready') return;
    this.#more = [...this.#more, ...next.data.data];
    this.#cursor = next.data.page?.next_cursor ?? undefined;
  }

  /** Runs one again: due now, its attempts from zero. */
  async requeue(id: string): Promise<void> {
    await engine.mutate<DeadLetter>('POST', `${DEAD_LETTERS}/${id}:requeue`, undefined, {
      invalidates: [DEAD_LETTERS, JOURNAL],
    });
    this.#requeued = new Set([...this.#requeued, id]);
  }
}

export const deadLetters = new DeadLetters();
