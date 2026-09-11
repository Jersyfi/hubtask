// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A right somebody exercised, as a case with a deadline.
 *
 * The pure half, for the reason `jobs.ts` is separate from its store: what a deadline *is* — past,
 * near, or comfortably away — is arithmetic, and it decides how a row is drawn.
 *
 * **A statutory deadline is a due date.** `data-protection.md` §4 gives thirty days from receipt
 * unless the caller names another, and this reads it exactly the way `lib/data/due.ts` reads an
 * entry's: by comparing instants. Inventing a second reading for it would be inventing a second
 * visual language, which is a design decision nobody took.
 *
 * **`INSTALLATION` is not among the scopes here.** It crosses the tenant boundary, needs the
 * `admin:tenants` scope and is the provider's path rather than a tenant's (F4's decision 6). It is
 * absent from the type rather than filtered at the end.
 */

/** The six rights the contract names. */
export type Kind = 'ACCESS' | 'ERASURE' | 'PORTABILITY' | 'RESTRICTION' | 'OBJECTION' | 'RECTIFICATION';

/** Every kind, in the order a picker should offer them. */
export const KINDS: readonly Kind[] = [
  'ACCESS',
  'PORTABILITY',
  'RECTIFICATION',
  'RESTRICTION',
  'OBJECTION',
  'ERASURE',
];

/** The state machine: `RECEIVED → IN_PROGRESS → COMPLETED | REJECTED`. */
export type Status = 'RECEIVED' | 'IN_PROGRESS' | 'COMPLETED' | 'REJECTED';

/** What an erasure does to other people's content. The default is the one that preserves it. */
export type ErasureMode = 'ANONYMIZE' | 'FULL_DELETE';

/** The kinds whose work produces an archive, and which therefore need a target before they start. */
export function producesArchive(kind: Kind): boolean {
  return kind === 'ACCESS' || kind === 'PORTABILITY';
}

/** One case. */
export interface Request {
  readonly id: string;
  readonly kind: Kind;
  readonly status: Status;
  readonly scope?: string;
  readonly subject_account_id?: string | null;
  /** Who asked, for a request that has no account behind it. */
  readonly subject_email?: string | null;
  readonly erasure_mode?: ErasureMode;
  readonly received_at: string;
  /** The statutory deadline. Thirty days from receipt unless the caller named another. */
  readonly due_at: string;
  readonly completed_at?: string | null;
  readonly handled_by?: string | null;
  readonly rejection_reason?: string | null;
  /** Where the export was written at the backup target, once a case has produced one. */
  readonly result_archive?: string | null;
  readonly result_target_id?: string | null;
  readonly notes?: string | null;
}

/**
 * How a deadline stands, in the product's own three states.
 *
 * The same three `DueMark` draws, and for the same reason: a reader who has learned what an overdue
 * task looks like has already learned what an overdue case looks like.
 */
export type Standing = 'overdue' | 'soon' | 'ahead';

/** How near counts as near. Two days, because a statutory answer is not a same-day task. */
export const SOON_MS = 2 * 24 * 60 * 60 * 1000;

/** Where a case's deadline stands at a given moment. A closed case has no standing to report. */
export function standingOf(request: Request, now: number): Standing | undefined {
  if (request.status === 'COMPLETED' || request.status === 'REJECTED') return undefined;
  const due = new Date(request.due_at).getTime();
  if (!Number.isFinite(due)) return undefined;
  if (due <= now) return 'overdue';
  return due - now <= SOON_MS ? 'soon' : 'ahead';
}

/**
 * The cases in the order the list has to be in: closest to its deadline first.
 *
 * The deadline is the column that matters, and a list ordered by anything else would put the case
 * somebody has two days for underneath the one they have three weeks for. Closed cases go last,
 * whatever their deadline was: a deadline that has been answered is not a deadline any more.
 */
export function byDeadline(requests: readonly Request[]): readonly Request[] {
  const closed = (request: Request) =>
    request.status === 'COMPLETED' || request.status === 'REJECTED';
  return [...requests].sort((left, right) => {
    if (closed(left) !== closed(right)) return closed(left) ? 1 : -1;
    return new Date(left.due_at).getTime() - new Date(right.due_at).getTime();
  });
}
