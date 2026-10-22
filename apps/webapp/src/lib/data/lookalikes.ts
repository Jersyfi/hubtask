// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Entries that look like a title somebody is still typing (UC-AI-04 check 7): the seam every place
 * that creates an entry mounts - the add form today, the capture line and the turning of text into
 * entries next.
 *
 * A read, though a `POST` like the search: what somebody types is their content, and a body does
 * not travel through access logs. It carries no idempotency key and is never queued - the engine
 * queues only the writes the application maps, and this is not one - and the server answers it
 * `no-store`. A question superseded by more typing is stopped, not merely ignored, so the provider
 * call behind it ends too.
 */

/** One entry near the words. */
export interface Lookalike {
  readonly item_id: string;
  readonly title: string;
  readonly collection_id: string;
  readonly similarity: number;
}

/** How long the typing has to rest before the words are asked about. */
export const LOOKALIKE_PAUSE_MS = 400;
/** Fewer characters than this say too little to be like anything. */
export const LOOKALIKE_MIN_CHARACTERS = 3;
/** How many are shown under a field: a question whether something exists, not a search. */
export const LOOKALIKES_SHOWN = 3;

/** Whether words are worth asking about. */
export const worthAsking = (title: string): boolean => [...title.trim()].length >= LOOKALIKE_MIN_CHARACTERS;

/** The one call this needs, as the engine makes it. */
export type AskLookalikes = (title: string, signal: AbortSignal) => Promise<readonly Lookalike[]>;

/** The timer this needs; a test's runs at once. */
export interface Pause {
  set(run: () => void, ms: number): unknown;
  clear(handle: unknown): void;
}

const realPause: Pause = {
  set: (run, ms) => setTimeout(run, ms),
  clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
};

/**
 * Follows the words of one field: after a pause it asks, stops the question before when more is
 * typed, and hands what it found to `show` - at most `LOOKALIKES_SHOWN`, or none. Every failure is
 * none: a person creating an entry is never told that looking for its duplicates failed.
 */
export function followTitle(ask: AskLookalikes, show: (found: readonly Lookalike[]) => void, pause: Pause = realPause) {
  let timer: unknown;
  let inFlight: AbortController | undefined;

  const stop = () => {
    if (timer !== undefined) pause.clear(timer);
    timer = undefined;
    inFlight?.abort();
    inFlight = undefined;
  };

  return {
    /** The field holds these words now. */
    typed(title: string): void {
      stop();
      if (!worthAsking(title)) {
        show([]);
        return;
      }
      timer = pause.set(() => {
        timer = undefined;
        const controller = new AbortController();
        inFlight = controller;
        ask(title.trim(), controller.signal).then(
          (found) => {
            if (inFlight !== controller) return;
            inFlight = undefined;
            show(found.slice(0, LOOKALIKES_SHOWN));
          },
          () => {
            if (inFlight === controller) inFlight = undefined;
            if (!controller.signal.aborted) show([]);
          },
        );
      }, LOOKALIKE_PAUSE_MS);
    },
    /** The field is gone: nothing more is asked or shown. */
    stop,
  };
}
