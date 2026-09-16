// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One entry's proposals: reading what stands, asking for more, and deciding each.
 *
 * **Asking is a `202` with no body.** The contract answers every AI ask that way on purpose - the
 * provider has not been asked yet, so there is nothing to answer with - and the change stream
 * carries no suggestion record. So the strip cannot follow a job; it follows the *listing*,
 * re-read on the job watcher's schedule (`followArrival`) until a proposal made after the ask
 * stands in it, and gives up after ten reads rather than polling a tab forever. `duplicates` is
 * the exception the contract makes: it is a query, answered now with the suggestion or with
 * nothing near.
 *
 * **A decision is the server's write.** `:accept` performs the use case as the accepting person;
 * this module sends the identifier and, where the person changed something first, the overrides.
 * Nothing here writes a field of the entry. The entry's own reads are invalidated after an
 * acceptance, because the entry is what changed.
 *
 * **No payload leaves in a URL.** The listing is by identifier; an override travels in a body.
 */

import type { Suggestion, SuggestionPage } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { followArrival, suggestionsPath, type Operation } from './suggestions.ts';

export { suggestionsPath } from './suggestions.ts';
export type { SuggestionPage };

/** What the strip shows while an ask is being followed, or after it stopped. */
export interface Asking {
  readonly operation: Operation;
  readonly askedAt: string;
  /** `following` while the listing is re-read; the other three are how it ended. */
  readonly outcome: 'following' | 'arrived' | 'gave_up' | 'nothing_near';
}

/** How long an ask may take to be accepted. It queues and returns; it does not wait for the model. */
const ASK_TIMEOUT_MS = 15_000;

const touches = (itemId: string) => [suggestionsPath(itemId)];

class Suggestions {
  #asking = $state<Record<string, Asking>>({});
  /** Which follow is current per entry, outside `$state`: a token, not something a template reads. */
  readonly #follows = new Map<string, symbol>();

  /** What is being asked about one entry, or nothing. */
  askingOf(itemId: string): Asking | undefined {
    return this.#asking[itemId];
  }

  /**
   * Asks one of the six operations, and follows the listing until the answer stands in it.
   *
   * `duplicates` answers now: `200` with the suggestion - which the invalidation puts into the
   * listing - or `204` with nothing near, which the strip says rather than waiting for.
   */
  async ask(itemId: string, operation: Operation): Promise<void> {
    const askedAt = new Date().toISOString();
    this.#put(itemId, { operation, askedAt, outcome: 'following' });

    if (operation === 'duplicates') {
      const answer = await engine.mutate<Suggestion | undefined>('POST', `/items/${itemId}:duplicates`, undefined, {
        idempotencyKey: crypto.randomUUID(),
        timeoutMs: ASK_TIMEOUT_MS,
        invalidates: touches(itemId),
      });
      this.#put(itemId, { operation, askedAt, outcome: answer ? 'arrived' : 'nothing_near' });
      return;
    }

    await engine.mutate<void>('POST', `/items/${itemId}:${operation}`, {}, {
      idempotencyKey: crypto.randomUUID(),
      timeoutMs: ASK_TIMEOUT_MS,
      // Nothing to invalidate: the answer is not there yet, and the follow reads the listing itself.
      invalidates: [],
    });

    const token = Symbol(operation);
    this.#follows.set(itemId, token);
    const outcome = await followArrival(
      engine,
      itemId,
      askedAt,
      (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
      () => this.#follows.get(itemId) === token,
    );
    if (outcome === 'left') return;
    this.#follows.delete(itemId);
    this.#put(itemId, { operation, askedAt, outcome });
  }

  /**
   * Accepts, as the person's own write. The overrides are what they changed before accepting,
   * laid over the proposal by the server (`SuggestionAcceptance`).
   */
  async accept(suggestion: Suggestion, overrides?: Readonly<Record<string, unknown>>): Promise<Suggestion> {
    return engine.mutate<Suggestion>(
      'POST',
      `/suggestions/${suggestion.id}:accept`,
      overrides ? { overrides } : {},
      {
        idempotencyKey: crypto.randomUUID(),
        // The entry changed - its fields, or the children under it - so every read of it is stale,
        // and the engine matches by prefix: `/items/{id}`, its activity, its children.
        invalidates: [...touches(suggestion.target_id), '/items'],
      },
    );
  }

  /** Dismisses. A state, not a deletion; the listing narrows to what still stands. */
  async dismiss(suggestion: Suggestion): Promise<Suggestion> {
    return engine.mutate<Suggestion>('POST', `/suggestions/${suggestion.id}:dismiss`, {}, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: touches(suggestion.target_id),
    });
  }

  /** Stops following and forgets what was asked. What the strip calls when it is destroyed. */
  forget(itemId: string): void {
    this.#follows.delete(itemId);
    const { [itemId]: _removed, ...rest } = this.#asking;
    this.#asking = rest;
  }

  #put(itemId: string, asking: Asking): void {
    this.#asking = { ...this.#asking, [itemId]: asking };
  }
}

export const suggestions = new Suggestions();
