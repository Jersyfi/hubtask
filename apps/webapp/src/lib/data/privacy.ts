// SPDX-License-Identifier: Apache-2.0
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
 * **An `INSTALLATION` case is listed and never acted on here.** It crosses the tenant boundary,
 * needs the `admin:tenants` scope and is the operator's path (data-protection.md §4, UC-PRV-06):
 * the register shows it, because its deadline is this workspace's to see, and `canAct` keeps every
 * control that moves it off the row. The operator moves it through the API or `hubctl`.
 */

import { nextDay } from './due.ts';
import { instantOf } from '../i18n/zone.ts';

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

/** Why a deadline was extended: the two reasons Art. 12(3) names. */
export type ExtensionReason = 'COMPLEXITY' | 'NUMBER_OF_REQUESTS';

/** Both reasons, in the order the form offers them. */
export const EXTENSION_REASONS: readonly ExtensionReason[] = ['COMPLEXITY', 'NUMBER_OF_REQUESTS'];

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
  /** The deadline in force: thirty days from receipt unless named, and the extended one once extended. */
  readonly due_at: string;
  /** The deadline before the one extension, present once there was one. */
  readonly original_due_at?: string | null;
  readonly extension_reason?: ExtensionReason | null;
  /** The day the person was told of the extension, as YYYY-MM-DD. */
  readonly informed_on?: string | null;
  /**
   * The latest day the deadline can be extended to, as YYYY-MM-DD in the workspace's zone. The
   * server answers it exactly while an extension can succeed, and nothing here recomputes it.
   */
  readonly extendable_until?: string;
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

/**
 * How near counts as near: seven days, the moment the installation's deadline watch starts warning
 * (alert A-19, `WarningWindow` in the server's privacy service). The register and the alert say
 * *owed soon* at the same moment (UC-PRV-01 check 5); two windows would be two answers to one
 * question.
 */
export const SOON_MS = 7 * 24 * 60 * 60 * 1000;

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

/** A message code and its parameters: a sentence the view renders without composing it. */
export interface Phrase {
  readonly code: string;
  readonly params: Readonly<Record<string, string>>;
}

/**
 * Whether the row offers any control that moves the case: start, complete, refuse, the erasure
 * mode, the extension. Not on an installation-wide case, which is the operator's (UC-PRV-06/6):
 * a control the server refuses without `admin:tenants` is a control that cannot succeed (P-05).
 */
export function canAct(request: Request): boolean {
  return request.scope !== 'INSTALLATION';
}

/**
 * Whether the row offers *Extend the deadline* (P-05: only where it can succeed).
 *
 * The server says when: `extendable_until` is answered while the case is open, not yet extended
 * and before its deadline. And only where the row may act at all.
 */
export function canExtend(request: Request): boolean {
  return canAct(request) && typeof request.extendable_until === 'string' &&
    request.extendable_until !== '';
}

/**
 * The row's deadline line. An extended case names both dates, the one in force and the one it
 * replaced, so nobody reads the extension as the deadline the case always had (P-11).
 */
export function deadlinePhrase(request: Request, format: (iso: string) => string): Phrase {
  if (request.original_due_at) {
    return {
      code: 'app.privacy.due_extended',
      params: { at: format(request.due_at), original: format(request.original_due_at) },
    };
  }
  return { code: 'app.privacy.due', params: { at: format(request.due_at) } };
}

/** Why the deadline was extended, as a sentence, with the day the person was told (P-12). */
export function extensionPhrase(
  request: Request,
  formatDay: (day: string) => string,
): Phrase | undefined {
  if (!request.extension_reason || !request.informed_on) return undefined;
  return {
    code: `app.privacy.extended_${request.extension_reason.toLowerCase()}`,
    params: { informed: formatDay(request.informed_on) },
  };
}

/** What the form sends: three facts, each required, both days as YYYY-MM-DD. */
export interface Extension {
  readonly due_on: string;
  readonly reason: ExtensionReason;
  readonly informed_on: string;
}

const DAY = /^\d{4}-\d{2}-\d{2}$/;

/**
 * The form's payload, or nothing while one of the three facts is missing.
 *
 * Only the shape is checked here. Whether the day lies inside the bound, or the informed day
 * between receipt and today, is the server's to say in the workspace's own zone, and the screen
 * renders its refusal rather than guessing at it.
 */
export function extensionPayload(draft: {
  dueOn: string;
  reason: string;
  informedOn: string;
}): Extension | undefined {
  const dueOn = draft.dueOn.trim();
  const informedOn = draft.informedOn.trim();
  if (!DAY.test(dueOn) || !DAY.test(informedOn)) return undefined;
  if (!EXTENSION_REASONS.includes(draft.reason as ExtensionReason)) return undefined;
  return { due_on: dueOn, reason: draft.reason as ExtensionReason, informed_on: informedOn };
}

/**
 * A deadline named as a day, as the instant the contract takes: the last second of that day in the
 * workspace's zone (UC-PRV-01 checks 2 and 6). The workspace's, not the reader's: a case is owed
 * by the workspace, and an administrator travelling does not move it. Undefined for a day that is
 * not one.
 */
export function deadlineOfDay(day: string, zone: string): string | undefined {
  if (!DAY.test(day.trim())) return undefined;
  const midnight = instantOf(nextDay(day.trim()), '00:00', zone);
  if (midnight === undefined) return undefined;
  return new Date(Date.parse(midnight) - 1000).toISOString();
}
