// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One entry's proposals: reading what stands, asking for more, and deciding each.
 *
 * **Asking is a `202` with the job.** The provider has not been asked yet, so the job is all there
 * is to answer with, and the change stream carries no suggestion record. So the strip follows the
 * job on the job watcher's schedule (`followArrival`) until it ends - refused, with a proposal the
 * listing then shows, or with an outcome that is said as a note - and says after about a minute
 * that the AI takes longer than usual, while it keeps following. The follow pauses while the page
 * is hidden and stops when the strip is left (`forget`). A refused ask ends its pending state at
 * once. `duplicates` is the exception the contract makes: it is a query, answered now with the
 * suggestion or with nothing near.
 *
 * **A decision is the server's write.** `:accept` performs the use case as the accepting person;
 * this module sends the identifier and, where the person changed something first, the overrides.
 * Nothing here writes a field of the entry. The entry's own reads are invalidated after an
 * acceptance, because the entry is what changed.
 *
 * **No payload leaves in a URL.** The listing is by identifier; an override travels in a body.
 */

import type { AcceptedSuggestion, AiTranslation, Suggestion, SuggestionPage } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import {
  arrivalEndedAs,
  budgetSpentOf,
  endedAs,
  followArrival,
  suggestionsPath,
  targetOf,
  waitWhileVisible,
  whilePending,
  type AskingState,
  type Operation,
  type Said,
  type Target,
} from './suggestions.ts';
import { ANY_ENTRY_DOCUMENT, ENTRY_LISTS } from './touches.ts';

export { suggestionsPath } from './suggestions.ts';
export type { SuggestionPage };

/**
 * What the strip shows while an ask is being followed, or after it stopped: `following`, `slow`
 * after about a minute without an end, or how it ended (`noteOf` says it).
 */
export interface Asking extends AskingState {
  readonly operation: Operation | 'jumble' | 'container' | 'template' | 'rule';
  readonly askedAt: string;
}

/** The pointer a `202` hands back; only its identifier is read here. */
interface Accepted {
  readonly job_id?: string;
}

/** The follow's wait: paused while the page is hidden, so a tab left open does not read for nothing. */
const wait = waitWhileVisible(typeof document === 'undefined' ? undefined : document);

/** How long an ask may take to be accepted. It queues and returns; it does not wait for the model. */
const ASK_TIMEOUT_MS = 15_000;
/** How long a translation may take: a person is waiting, and the server bounds it the same way. */
const TRANSLATE_TIMEOUT_MS = 30_000;

const touches = (targetId: string, target: Target = 'WORK_ITEM') => [suggestionsPath(targetId, target)];

class Suggestions {
  #asking = $state<Record<string, Asking>>({});
  /** A spent budget a proposal prepared on arrival ended on: said once for the jumble, not per card. */
  #arrivalBudget = $state<Said | undefined>(undefined);
  /** Which follow is current per entry, outside `$state`: a token, not something a template reads. */
  readonly #follows = new Map<string, symbol>();

  /** The spent budget proposals prepared on arrival met, where one did. */
  get arrivalBudgetSpent(): Said | undefined {
    return this.#arrivalBudget;
  }

  /**
   * Follows the proposal the workspace prepares about a jumble entry on its arrival (UC-JUM-07
   * check 10), which nobody here asked for: the card says *Suggesting…* while the job runs and
   * shows the proposal when it stands. Not while the entry is already followed - an ask a person
   * made says itself - and never said as slow: nobody is waiting for it.
   */
  followArrivalJob(entryId: string, jobId: string, receivedAt: string): void {
    if (this.#asking[entryId]) return;
    this.#put(entryId, { operation: 'jumble', askedAt: receivedAt, outcome: 'following' });
    const token = Symbol('arrival');
    this.#follows.set(entryId, token);
    const isLive = () => this.#follows.get(entryId) === token;
    void followArrival(engine, entryId, receivedAt, wait, isLive, 'JUMBLE_ENTRY', jobId).then((outcome) => {
      if (outcome === 'left' || !isLive()) return;
      this.#follows.delete(entryId);
      const spent = budgetSpentOf(outcome);
      if (spent) this.#arrivalBudget = spent;
      this.#put(entryId, { operation: 'jumble', askedAt: receivedAt, ...arrivalEndedAs(outcome) });
    });
  }

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
    const begin = () => this.#put(itemId, { operation, askedAt, outcome: 'following' });
    const withdraw = () => this.#withdraw(itemId, askedAt);

    if (operation === 'duplicates') {
      const answer = await whilePending(begin, withdraw, () =>
        engine.mutate<Suggestion | undefined>('POST', `/items/${itemId}:duplicates`, undefined, {
          idempotencyKey: crypto.randomUUID(),
          timeoutMs: ASK_TIMEOUT_MS,
          invalidates: touches(itemId),
        }),
      );
      this.#put(itemId, { operation, askedAt, outcome: answer ? 'arrived' : 'nothing_near' });
      return;
    }

    const accepted = await whilePending(begin, withdraw, () =>
      engine.mutate<Accepted | undefined>('POST', `/items/${itemId}:${operation}`, {}, {
        idempotencyKey: crypto.randomUUID(),
        timeoutMs: ASK_TIMEOUT_MS,
        // Nothing to invalidate: the answer is not there yet, and the follow reads the listing itself.
        invalidates: [],
      }),
    );

    await this.#follow(itemId, operation, askedAt, 'WORK_ITEM', accepted?.job_id);
  }

  /**
   * Asks what a jumble entry should become, and follows the listing the same way.
   *
   * One operation rather than six: an arrival has no fields yet, so the one question is what it
   * would be as work - a title, notes, a date, and the subtasks the material implies.
   */
  async askJumble(entryId: string): Promise<void> {
    await (await this.#askJumble(entryId))();
  }

  /**
   * The same ask, answered once it is accepted rather than once it ended: the follow goes on in
   * the background, on the entry's card. What asking about a selection waits for, so its next ask
   * is not held up by a model's answer to the last one.
   */
  async queueJumble(entryId: string): Promise<void> {
    void (await this.#askJumble(entryId))();
  }

  /** Sends the ask, and answers the follow that is left to run. */
  async #askJumble(entryId: string): Promise<() => Promise<void>> {
    const askedAt = new Date().toISOString();
    const accepted = await whilePending(
      () => this.#put(entryId, { operation: 'jumble', askedAt, outcome: 'following' }),
      () => this.#withdraw(entryId, askedAt),
      () =>
        engine.mutate<Accepted | undefined>('POST', `/jumble/entries/${entryId}:suggest`, undefined, {
          idempotencyKey: crypto.randomUUID(),
          timeoutMs: ASK_TIMEOUT_MS,
          invalidates: [],
        }),
    );
    return () => this.#follow(entryId, 'jumble', askedAt, 'JUMBLE_ENTRY', accepted?.job_id);
  }

  /**
   * Asks how a collection stands - what is open, what moved, what is overdue - and follows
   * the listing. Nothing accepts the answer: a collection has nowhere to put a status summary, so
   * it is read and dismissed.
   */
  async askContainer(containerId: string): Promise<void> {
    const askedAt = new Date().toISOString();
    const accepted = await whilePending(
      () => this.#put(containerId, { operation: 'container', askedAt, outcome: 'following' }),
      () => this.#withdraw(containerId, askedAt),
      () =>
        engine.mutate<Accepted | undefined>('POST', `/containers/${containerId}:summarize`, undefined, {
          idempotencyKey: crypto.randomUUID(),
          timeoutMs: ASK_TIMEOUT_MS,
          invalidates: [],
        }),
    );
    await this.#follow(containerId, 'container', askedAt, 'CONTAINER', accepted?.job_id);
  }

  /**
   * Asks for a template drafted from a description, and follows the collection's
   * listing the way the status summary is followed: the draft appears there as a `TEMPLATE`
   * suggestion with the collection as its target, and accepting it is `CreateTemplate`.
   */
  async generateTemplate(collectionId: string, description: string): Promise<void> {
    const askedAt = new Date().toISOString();
    const accepted = await engine.mutate<Accepted | undefined>('POST', '/templates:generate', { collection_id: collectionId, description }, {
      idempotencyKey: crypto.randomUUID(),
      timeoutMs: ASK_TIMEOUT_MS,
      invalidates: [],
    });
    // Answered once the ask is accepted, not once the draft lands: the dialog that asked closes,
    // and the follow's outcome is the strip's to show.
    this.#put(collectionId, { operation: 'template', askedAt, outcome: 'following' });
    void this.#follow(collectionId, 'template', askedAt, 'CONTAINER', accepted?.job_id);
  }

  /**
   * Asks for a rule drafted from a sentence (UC-AUT-10), and follows the person's own listing: the
   * draft appears there as a `RULE` suggestion about their account, which only they read. The
   * follow is keyed on the account, so the editor that asked reads `askingOf(accountId)`.
   */
  async describeRule(accountId: string, scope: { type: string; id?: string }, description: string): Promise<void> {
    const askedAt = new Date().toISOString();
    const accepted = await whilePending(
      () => this.#put(accountId, { operation: 'rule', askedAt, outcome: 'following' }),
      () => this.#withdraw(accountId, askedAt),
      () =>
        engine.mutate<Accepted | undefined>('POST', '/automation/rules:describe', { scope, description }, {
          idempotencyKey: crypto.randomUUID(),
          timeoutMs: ASK_TIMEOUT_MS,
          invalidates: [],
        }),
    );
    void this.#follow(accountId, 'rule', askedAt, 'ACCOUNT', accepted?.job_id);
  }

  /**
   * Reads the entry in another language. Synchronous and stored nowhere: the answer is
   * handed back and held by nothing here - it is the caller's to show and to drop.
   */
  async translate(itemId: string, targetLocale: string): Promise<AiTranslation> {
    return engine.mutate<AiTranslation>('POST', `/items/${itemId}:translate`, { target_locale: targetLocale }, {
      idempotencyKey: crypto.randomUUID(),
      timeoutMs: TRANSLATE_TIMEOUT_MS,
      // A read: nothing it answers makes anything the client holds stale.
      invalidates: [],
    });
  }

  /**
   * Accepts, as the person's own write. The overrides are what they changed before accepting,
   * laid over the proposal by the server (`SuggestionAcceptance`) - for a jumble entry the
   * destination collection, which a model never chooses.
   */
  async accept(suggestion: Suggestion, overrides?: Readonly<Record<string, unknown>>): Promise<AcceptedSuggestion> {
    const target = targetOf(suggestion);
    return engine.mutate<AcceptedSuggestion>(
      'POST',
      `/suggestions/${suggestion.id}:accept`,
      overrides ? { overrides } : {},
      {
        idempotencyKey: crypto.randomUUID(),
        // The entry changed - its fields, or the children under it - so every read of it is stale,
        // and the engine matches by prefix: `/items/{id}`, its activity, its children. A jumble
        // acceptance is a conversion, so the inbox and the containers a board reads are stale too.
        // A container's suggestion is a template: accepting it is `CreateTemplate`, so the
        // templates that apply here are what changed. A draft for its asker is a rule: the rules
        // are what changed.
        invalidates:
          target === 'JUMBLE_ENTRY'
            ? [...touches(suggestion.target_id, target), '/jumble/entries', ENTRY_LISTS, '/containers']
            : target === 'CONTAINER'
              ? [...touches(suggestion.target_id, target), '/templates']
              : target === 'ACCOUNT'
                ? [...touches(suggestion.target_id, target), '/automation/rules']
                : [...touches(suggestion.target_id), ENTRY_LISTS, ANY_ENTRY_DOCUMENT],
      },
    );
  }

  /** Dismisses. A state, not a deletion; the listing narrows to what still stands. */
  async dismiss(suggestion: Suggestion): Promise<Suggestion> {
    const target = targetOf(suggestion);
    return engine.mutate<Suggestion>('POST', `/suggestions/${suggestion.id}:dismiss`, {}, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: touches(suggestion.target_id, target),
    });
  }

  /** Stops following and forgets what was asked. What the strip calls when it is destroyed. */
  forget(itemId: string): void {
    this.#follows.delete(itemId);
    const { [itemId]: _removed, ...rest } = this.#asking;
    this.#asking = rest;
  }

  /**
   * Follows one ask to its end, and records how it ended. A follow that was superseded - the strip
   * left, or asked again - records nothing.
   */
  async #follow(id: string, operation: Asking['operation'], askedAt: string, target: Target, jobId?: string): Promise<void> {
    const token = Symbol(operation);
    this.#follows.set(id, token);
    const isLive = () => this.#follows.get(id) === token;
    const outcome = await followArrival(engine, id, askedAt, wait, isLive, target, jobId, () => {
      if (isLive()) this.#put(id, { operation, askedAt, outcome: 'slow' });
    });
    if (outcome === 'left' || !isLive()) return;
    this.#follows.delete(id);
    this.#put(id, { operation, askedAt, ...endedAs(outcome) });
  }

  /**
   * Ends the pending state of an ask that was refused, where it is still this ask's: the caller says
   * the refusal, and a later ask about the same entry keeps its own state.
   */
  #withdraw(itemId: string, askedAt: string): void {
    if (this.#asking[itemId]?.askedAt !== askedAt) return;
    const { [itemId]: _withdrawn, ...rest } = this.#asking;
    this.#asking = rest;
  }

  #put(itemId: string, asking: Asking): void {
    this.#asking = { ...this.#asking, [itemId]: asking };
  }
}

export const suggestions = new Suggestions();
