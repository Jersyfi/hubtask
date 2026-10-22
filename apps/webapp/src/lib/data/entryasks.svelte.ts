// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The questions one entry's ✦ menus ask, each at its own place (UC-AI-04 checks 6 and 9).
 *
 * One question per kind at a time, followed the way every ask is (`followArrival`): the pending
 * card stands at the place it was asked from, the job's id is kept so that *Stop* can stop it, and
 * a stopped question ends quietly - the listing is read once afterwards, so whatever had already
 * arrived is shown. A translation is a request a person waits on rather than a job: its *Stop*
 * aborts the request, and the provider call ends with it.
 *
 * **Who may ask what is not decided here.** The menus offer only what the server answers
 * (`offersAt`); the server still decides, and a refusal is said where it was asked.
 */

import type { AiTranslation, Suggestion } from '@hubtask/sync-engine';

import type { Ask } from './aiplaces.ts';
import { ASKED_FIELD } from './aiplaces.ts';
import { engine } from './engine.ts';
import { endedAs, followArrival, suggestionsPath, waitWhileVisible, whilePending, type AskingState } from './suggestions.ts';

/** How one question stands: following (or slow), or how it ended - with the job that carries it. */
export interface Question extends AskingState {
  readonly ask: Ask;
  readonly askedAt: string;
  /** The job a `202` named, while one is known. Absent for a question answered at once. */
  readonly jobId?: string;
}

/** The pointer a `202` hands back. */
interface Accepted {
  readonly job_id?: string;
}

const wait = waitWhileVisible(typeof document === 'undefined' ? undefined : document);

/** How long an ask may take to be accepted; it queues and returns. */
const ASK_TIMEOUT_MS = 15_000;
/** How long a translation may take: a person is waiting, and the server bounds it the same way. */
const TRANSLATE_TIMEOUT_MS = 30_000;
/** How long stopping may take. */
const STOP_TIMEOUT_MS = 10_000;

/** The path of each question that queues a job, by the operation the contract names. */
const OPERATION: Readonly<Partial<Record<Ask, string>>> = {
  'suggest-title': 'suggest-fields',
  'suggest-notes': 'suggest-fields',
  'suggest-due': 'suggest-fields',
  summarize: 'summarize',
  classify: 'classify',
  decompose: 'decompose',
  'summarize-thread': 'summarize-thread',
};

const keyOf = (itemId: string, ask: Ask) => `${itemId}#${ask}`;

class EntryAsks {
  #questions = $state<Record<string, Question>>({});
  /** Which follow is current per question, outside `$state`: a token, not something a template reads. */
  readonly #follows = new Map<string, symbol>();
  /** The translation on its way per entry, so that *Stop* can abort it. */
  readonly #translating = new Map<string, AbortController>();

  /** How one question about one entry stands, or nothing. */
  of(itemId: string, ask: Ask): Question | undefined {
    return this.#questions[keyOf(itemId, ask)];
  }

  /**
   * Asks one question, and follows it until it ends. `duplicates` answers at once - a suggestion,
   * or nothing near, which is said - and a field question names its one field (`AiFieldsAsk`).
   */
  async ask(itemId: string, ask: Ask): Promise<void> {
    const askedAt = new Date().toISOString();
    try {
      await this.#ask(itemId, ask, askedAt);
    } catch (error) {
      // A refusal of the ask itself - AI out of reach, nothing to work with, the budget - is said
      // at the place it was asked from, as the refusal a job ends with is.
      const refused = error as { detailCode?: string; params?: Readonly<Record<string, string>> };
      this.#put(keyOf(itemId, ask), {
        ask, askedAt, outcome: 'refused',
        refusal: { code: refused.detailCode ?? 'app.something_went_wrong', params: refused.params },
      });
    }
  }

  async #ask(itemId: string, ask: Ask, askedAt: string): Promise<void> {
    const key = keyOf(itemId, ask);
    const begin = () => this.#put(key, { ask, askedAt, outcome: 'following' });
    const withdraw = () => this.#withdraw(key, askedAt);

    if (ask === 'duplicates') {
      const answer = await whilePending(begin, withdraw, () =>
        engine.mutate<Suggestion | undefined>('POST', `/items/${itemId}:duplicates`, undefined, {
          idempotencyKey: crypto.randomUUID(),
          timeoutMs: ASK_TIMEOUT_MS,
          invalidates: [suggestionsPath(itemId)],
        }),
      );
      this.#put(key, { ask, askedAt, outcome: answer ? 'arrived' : 'nothing_near' });
      return;
    }

    const operation = OPERATION[ask];
    if (!operation) return;
    const field = ASKED_FIELD[ask];
    const accepted = await whilePending(begin, withdraw, () =>
      engine.mutate<Accepted | undefined>('POST', `/items/${itemId}:${operation}`, field ? { fields: [field] } : {}, {
        idempotencyKey: crypto.randomUUID(),
        timeoutMs: ASK_TIMEOUT_MS,
        // Nothing to invalidate: the answer is not there yet, and the follow reads the listing.
        invalidates: [],
      }),
    );
    const jobId = accepted?.job_id;
    if (jobId) this.#put(key, { ask, askedAt, outcome: 'following', jobId });

    const token = Symbol(ask);
    this.#follows.set(key, token);
    const isLive = () => this.#follows.get(key) === token;
    const outcome = await followArrival(engine, itemId, askedAt, wait, isLive, 'WORK_ITEM', jobId, () => {
      if (isLive()) this.#put(key, { ask, askedAt, outcome: 'slow', jobId });
    });
    if (outcome === 'left' || !isLive()) return;
    this.#follows.delete(key);
    this.#put(key, { ask, askedAt, ...endedAs(outcome) });
  }

  /**
   * Stops a question that is still on its way: the job is cancelled, the question ends without a
   * note, and the listing is read once - a proposal recorded before the stop stands and is shown.
   * A job that had already finished answers `409`, which is the same reading.
   */
  async stop(itemId: string, ask: Ask): Promise<void> {
    const key = keyOf(itemId, ask);
    const question = this.#questions[key];
    this.#follows.delete(key);
    this.#withdraw(key, question?.askedAt ?? '');
    if (question?.jobId) {
      try {
        await engine.mutate('POST', `/jobs/${question.jobId}:cancel`, undefined, {
          idempotencyKey: crypto.randomUUID(),
          timeoutMs: STOP_TIMEOUT_MS,
          invalidates: [],
        });
      } catch {
        // Finished already, or out of reach: either way what stands is read next, and that is
        // the answer the person sees.
      }
    }
    try {
      await engine.refresh({ path: suggestionsPath(itemId) });
    } catch {
      // The listing shows what it last held.
    }
  }

  /** Reads the entry in another language; `stopTranslation` aborts it. */
  async translate(itemId: string, targetLocale: string): Promise<AiTranslation> {
    this.#translating.get(itemId)?.abort();
    const controller = new AbortController();
    this.#translating.set(itemId, controller);
    try {
      return await engine.mutate<AiTranslation>('POST', `/items/${itemId}:translate`, { target_locale: targetLocale }, {
        idempotencyKey: crypto.randomUUID(),
        timeoutMs: TRANSLATE_TIMEOUT_MS,
        signal: controller.signal,
        // A read: nothing it answers makes anything the client holds stale.
        invalidates: [],
      });
    } finally {
      if (this.#translating.get(itemId) === controller) this.#translating.delete(itemId);
    }
  }

  /** Stops the translation on its way, if one is. */
  stopTranslation(itemId: string): void {
    this.#translating.get(itemId)?.abort();
    this.#translating.delete(itemId);
  }

  /** Forgets every question about one entry: what the entry's page calls when it is left. */
  forget(itemId: string): void {
    const prefix = `${itemId}#`;
    for (const key of [...this.#follows.keys()]) if (key.startsWith(prefix)) this.#follows.delete(key);
    this.stopTranslation(itemId);
    this.#questions = Object.fromEntries(Object.entries(this.#questions).filter(([key]) => !key.startsWith(prefix)));
  }

  #withdraw(key: string, askedAt: string): void {
    if (this.#questions[key]?.askedAt !== askedAt) return;
    const { [key]: _withdrawn, ...rest } = this.#questions;
    this.#questions = rest;
  }

  #put(key: string, question: Question): void {
    this.#questions = { ...this.#questions, [key]: question };
  }
}

export const entryAsks = new EntryAsks();
