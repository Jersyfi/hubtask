// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The job watcher, once and in the data layer.
 *
 * A backup, a verification, a restore, an audit export and a data subject request's archive all
 * answer `202 Accepted` with a `JobRef`, and the answer arrives at `GET /jobs/{jobId}`. Five
 * screens in this milestone need that loop; five copies of it is where a bug lives in four of
 * them, so it is written here and each screen watches rather than polls.
 *
 * **It is not a resource.** `engine.subscribe` holds a state per path and re-reads it when
 * something invalidates it — the right shape for a listing, and the wrong one for a row that
 * changes on its own while nobody writes anything. A job is polled on a schedule of its own and
 * stops the moment it is terminal, which is a lifetime `subscribe` has no way to express.
 *
 * **A terminal job is polled no further**, and that is the property worth being careful about: a
 * loop that kept asking after `SUCCEEDED` would be a tab making a request every fifteen seconds
 * for as long as it stays open, on every screen anybody left behind.
 *
 * **Nothing here renders a failure.** `error_code` is carried as the code it is; the screen puts
 * it through the catalogue like every other code (ADR-0011).
 */

import { engine } from './engine.ts';
import { isTerminal, pollDelay, worthRetrying, type Job, type JobRef } from './jobs.ts';

export type { Job, JobRef, JobStatus } from './jobs.ts';
export { isTerminal, mayCancel } from './jobs.ts';

const JOBS = '/jobs';

/** How long one poll may take. Longer than the shortest wait, shorter than the longest. */
const READ_TIMEOUT_MS = 10_000;

/**
 * What a screen sees of a job it is watching.
 *
 * `unreachable` is separate from the job's own `error_code` on purpose: a job that failed and a
 * job whose status could not be read are different facts, and a screen that showed the second as
 * the first would report a failure the server never reported.
 */
export interface Watch {
  readonly job: Job;
  /** True while a poll is scheduled — the job is not terminal and the watcher is still asking. */
  readonly watching: boolean;
  /** The code of a read that failed and will not be retried. Not the job's own failure. */
  readonly unreachable?: string;
}

class Jobs {
  #watches = $state<Record<string, Watch>>({});
  /** Timers by job id, outside `$state`: a pending timeout is not something a template reads. */
  readonly #timers = new Map<string, ReturnType<typeof setTimeout>>();

  /** What is known about one job, or nothing if this tab never watched it. */
  of(jobId: string): Watch | undefined {
    return this.#watches[jobId];
  }

  /**
   * Starts watching what a `202` handed back, and answers at once with what it already says.
   *
   * Idempotent: watching a job that is already watched changes nothing, so a screen may call this
   * from a click handler and from a re-render without arranging not to.
   */
  watch(accepted: JobRef): void {
    const existing = this.#watches[accepted.job_id];
    if (existing && this.#timers.has(accepted.job_id)) return;

    const job: Job = { ...existing?.job, ...accepted };
    this.#put(accepted.job_id, { job, watching: !isTerminal(job.status) });
    if (isTerminal(job.status)) return;
    this.#schedule(accepted.job_id, 0);
  }

  /**
   * Stops watching and forgets what was seen.
   *
   * What a screen calls when it is destroyed. A watcher that outlived its screen would keep a tab
   * polling for something nobody is looking at.
   */
  forget(jobId: string): void {
    this.#stop(jobId);
    const { [jobId]: _removed, ...rest } = this.#watches;
    this.#watches = rest;
  }

  /** Forgets every job. What a sign-out calls, beside `engine.reset()`. */
  forgetAll(): void {
    for (const jobId of [...this.#timers.keys()]) this.#stop(jobId);
    this.#watches = {};
  }

  /**
   * Asks the server to stop the work.
   *
   * Cooperative rather than a kill, and what a pass has already put outside the database — bytes
   * at a backup target — a cancellation cannot take back. The screen says that; this only asks.
   * A job that has already finished answers `409`, which the caller renders like any refusal.
   */
  async cancel(jobId: string): Promise<void> {
    const answer = await engine.mutate<Job>('POST', `${JOBS}/${jobId}:cancel`, {}, {
      idempotencyKey: crypto.randomUUID(),
      timeoutMs: READ_TIMEOUT_MS,
    });
    this.#stop(jobId);
    this.#put(jobId, { job: answer, watching: false });
  }

  #put(jobId: string, watch: Watch): void {
    this.#watches = { ...this.#watches, [jobId]: watch };
  }

  #stop(jobId: string): void {
    const timer = this.#timers.get(jobId);
    if (timer !== undefined) clearTimeout(timer);
    this.#timers.delete(jobId);
  }

  #schedule(jobId: string, attempt: number): void {
    this.#stop(jobId);
    const timer = setTimeout(() => {
      this.#timers.delete(jobId);
      void this.#poll(jobId, attempt);
    }, pollDelay(attempt));
    this.#timers.set(jobId, timer);
  }

  async #poll(jobId: string, attempt: number): Promise<void> {
    // Gone means forgotten: a screen that was destroyed between the timer firing and this line
    // has already said it is not interested.
    if (!this.#watches[jobId]) return;

    const read = await engine.refresh<Job>({ path: `${JOBS}/${jobId}`, timeoutMs: READ_TIMEOUT_MS });

    // Gone again: the screen may have been destroyed while the read was in flight.
    if (!this.#watches[jobId]) return;

    if (read.status !== 'ready') {
      const current = this.#watches[jobId];
      if (!current) return;
      if (read.status !== 'failed' || worthRetrying(read.error)) {
        this.#schedule(jobId, attempt + 1);
        return;
      }
      // Not worth asking again: the server has said this caller will not be told about this job,
      // and asking every second would be asking the same question of the same answer.
      this.#put(jobId, {
        job: current.job,
        watching: false,
        unreachable: read.error.detailCode ?? read.error.code ?? 'route.internal_error',
      });
      return;
    }

    const job = read.data;
    const done = isTerminal(job.status);
    this.#put(jobId, { job, watching: !done });
    if (!done) this.#schedule(jobId, attempt + 1);
  }
}

export const jobs = new Jobs();
