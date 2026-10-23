// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The one question a person has asked the workspace (UC-AI-08), followed to its answer.
 *
 * One at a time and nothing kept: asking again replaces the question and its state, and nothing of
 * an earlier question travels with a new one (check 5). The answer appears in the
 * asker's own listing as an `ANSWER` suggestion; the follow is the one every ask uses, through the
 * job while it runs, so *Stop* cancels the job as an entry's ask does.
 */

import { engine } from '../data/engine.ts';
import { endedAs, followArrival, suggestionsPath, waitWhileVisible, whilePending, type AskingState } from '../data/suggestions.ts';

/** The question being followed, or how it ended. */
export interface Question extends AskingState {
  readonly words: string;
  readonly askedAt: string;
  readonly jobId?: string;
}

interface Accepted {
  readonly job_id?: string;
}

const wait = waitWhileVisible(typeof document === 'undefined' ? undefined : document);

/** How long the asking may take to be accepted: it queues and returns. */
const ASK_TIMEOUT_MS = 15_000;
const STOP_TIMEOUT_MS = 10_000;

class WorkspaceAsks {
  #question = $state<Question | undefined>(undefined);
  #follow: symbol | undefined;

  /** The current question, or nothing. */
  get question(): Question | undefined {
    return this.#question;
  }

  /** Asks, and follows the asker's listing until the answer stands in it. A refusal is the state. */
  async ask(accountId: string, words: string): Promise<void> {
    const askedAt = new Date().toISOString();
    const token = Symbol('ask');
    this.#follow = token;
    const isLive = () => this.#follow === token;
    try {
      const accepted = await whilePending(
        () => (this.#question = { words, askedAt, outcome: 'following' }),
        () => {
          if (isLive()) this.#question = undefined;
        },
        () =>
          engine.mutate<Accepted | undefined>('POST', '/ask', { question: words }, {
            idempotencyKey: crypto.randomUUID(),
            timeoutMs: ASK_TIMEOUT_MS,
            invalidates: [],
          }),
      );
      const jobId = accepted?.job_id;
      if (!isLive()) return;
      this.#question = { words, askedAt, outcome: 'following', ...(jobId ? { jobId } : {}) };
      const outcome = await followArrival(engine, accountId, askedAt, wait, isLive, 'ACCOUNT', jobId, () => {
        if (isLive()) this.#question = { words, askedAt, outcome: 'slow', ...(jobId ? { jobId } : {}) };
      });
      if (outcome === 'left' || !isLive()) return;
      this.#follow = undefined;
      this.#question = { words, askedAt, ...endedAs(outcome) };
    } catch (error) {
      if (!isLive()) return;
      const refused = error as { detailCode?: string; params?: Readonly<Record<string, string>> };
      this.#question = {
        words, askedAt, outcome: 'refused',
        refusal: { code: refused.detailCode ?? 'app.something_went_wrong', params: refused.params },
      };
    }
  }

  /**
   * Stops the question on its way: the follow ends here, and the job is cancelled where it has
   * one. A job that already finished answers that it cannot be stopped, which changes nothing here.
   */
  async stop(accountId: string): Promise<void> {
    const question = this.#question;
    this.forget();
    if (question?.jobId) {
      try {
        await engine.mutate('POST', `/jobs/${question.jobId}:cancel`, undefined, {
          idempotencyKey: crypto.randomUUID(),
          timeoutMs: STOP_TIMEOUT_MS,
          invalidates: [],
        });
      } catch {
        // Already finished, or the network failed: the question is put away either way.
      }
    }
    try {
      await engine.refresh({ path: suggestionsPath(accountId, 'ACCOUNT') });
    } catch {
      // The listing is read again when the dialog next opens.
    }
  }

  /** Forgets the question: a new one starts from nothing. */
  forget(): void {
    this.#follow = undefined;
    this.#question = undefined;
  }
}

export const workspaceAsks = new WorkspaceAsks();
