// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a caller can work out about a job without asking the server again.
 *
 * The pure half of the job watcher: which statuses end a job, how long to wait before the next
 * poll, and whether a failed read is worth another attempt. It is separate from the store for the
 * reason `reminders.ts` is separate from its store — a schedule is arithmetic, and arithmetic is
 * worth a test that needs neither a browser nor a fake clock inside a rune.
 */

import { TransportError } from '@hubtask/sync-engine';

/** The five states `JobStatus` declares. A caller sees no more of the queue than this. */
export type JobStatus = 'QUEUED' | 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'CANCELLED';

/** A job as `GET /jobs/{jobId}` answers it. */
export interface Job {
  readonly job_id: string;
  readonly status: JobStatus;
  /**
   * How far along, between 0 and 1, or `null` from a job that cannot say. `null` is the
   * indeterminate case rather than zero — a bar drawn at zero for it would report no progress on
   * a job that is making some.
   */
  readonly progress?: number | null;
  readonly result_url?: string | null;
  /** The message code of the last failure. Never a sentence (ADR-0011). */
  readonly error_code?: string | null;
  readonly created_at?: string;
  readonly finished_at?: string | null;
}

/** What a `202 Accepted` hands back: the same shape, narrowed to what is knowable then. */
export interface JobRef {
  readonly job_id: string;
  readonly status: JobStatus;
  readonly progress?: number | null;
  readonly result_url?: string | null;
}

/** The three that end it. Polling one of these again would learn nothing. */
const TERMINAL: readonly JobStatus[] = ['SUCCEEDED', 'FAILED', 'CANCELLED'];

export function isTerminal(status: JobStatus): boolean {
  return TERMINAL.includes(status);
}

/** Only a job that is still going can be stopped; the server answers `409` for the rest. */
export function mayCancel(status: JobStatus): boolean {
  return status === 'QUEUED' || status === 'RUNNING';
}

/** The first wait, once the job has been accepted. Short: most small jobs are done by then. */
export const POLL_FIRST_MS = 500;
/** The ceiling. A backup of a large workspace runs for minutes and nobody watches every second. */
export const POLL_MAX_MS = 15_000;
/** How fast the wait grows. 1.6 reaches the ceiling in about eight polls. */
const POLL_GROWTH = 1.6;

/**
 * How long to wait before poll number `attempt` (counting the first as 0).
 *
 * Geometric with a ceiling, and deliberately without jitter: this is one tab watching one job it
 * started itself, not a fleet reconnecting to a server that just came back — the thundering herd
 * jitter exists for is not a shape a single watcher can make.
 */
export function pollDelay(attempt: number): number {
  const grown = POLL_FIRST_MS * POLL_GROWTH ** Math.max(0, attempt);
  return Math.min(POLL_MAX_MS, Math.round(grown));
}

/**
 * Whether a failed read is worth repeating.
 *
 * A timeout or a dead network is: the job is still running and the answer will come. A problem
 * document with a client status is not — `403` and `404` are the server saying this caller will
 * not be told about this job, and asking again every second would be asking the same question of
 * the same answer. `408` and `429` are the two client statuses that mean "later" rather than "no".
 */
export function worthRetrying(cause: unknown): boolean {
  if (!(cause instanceof TransportError)) return false;
  if (cause.kind !== 'problem') return true;
  const status = cause.status ?? 0;
  if (status === 408 || status === 429) return true;
  return status >= 500;
}
