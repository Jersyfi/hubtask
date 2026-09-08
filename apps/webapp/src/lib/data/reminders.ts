// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What this client decides about reminders and about a series, pure so each decision is tested.
 *
 * **An offset has two forms and no third.** `REL:<ISO-8601 duration>` is counted from the entry's
 * due date, negative for before it; `ABS:<instant>` is a fixed moment. A preset is only this
 * client's vocabulary for a common offset — "an hour before" — and what travels is the offset.
 *
 * **A relative reminder needs a due date**, because `fire_at` would otherwise mean nothing. The
 * server says so, and this predicts it so the control is off with the reason rather than refused
 * after somebody fills it in.
 *
 * **Years and months are refused in a duration**, which is the contract's own rule and worth
 * mirroring: they are calendar arithmetic rather than a length of time, and a reminder that meant
 * two different things in two months would be a promise the offset cannot keep.
 *
 * **`LAPSED` is a state the schema does not declare.** It is real — `offline-sync.md` §8, and a
 * restore produces it — so the state is read as a string, and one this client has never met still
 * renders as words.
 */

import type { Reminder, WorkItem } from '@hubtask/sync-engine';

/** The two prefixes, written once. */
export const RELATIVE = 'REL:';
export const ABSOLUTE = 'ABS:';

export function isRelative(spec: string): boolean {
  return spec.startsWith(RELATIVE);
}

/**
 * Whether an offset is one the server will take.
 *
 * A duration of years or months is refused for the reason above; a duration with no unit at all is
 * not a duration. This is a courtesy check — the server validates properly — so it errs towards
 * letting things through: what it must never do is refuse an offset the server would accept.
 */
export function isWellFormed(spec: string): boolean {
  if (spec.startsWith(ABSOLUTE)) return !Number.isNaN(Date.parse(spec.slice(ABSOLUTE.length)));
  if (!spec.startsWith(RELATIVE)) return false;

  const duration = spec.slice(RELATIVE.length);
  // A leading minus is what "before the due date" is written as.
  const body = duration.startsWith('-') ? duration.slice(1) : duration;
  if (!body.startsWith('P') || body === 'P') return false;
  // Y and M before the T are years and months. The M *after* it is minutes, which is why this
  // looks at the date half alone rather than at the whole string.
  const [datePart] = body.slice(1).split('T');
  if (/[YM]/.test(datePart ?? '')) return false;
  return /\d/.test(body);
}

/** The states this client has phrases for. Anything else is a state from a newer server. */
export const REMINDER_STATES = ['PENDING', 'SENT', 'CANCELLED', 'LAPSED'] as const;

/**
 * Whether a reminder can still be edited.
 *
 * One that has fired, been cancelled or lapsed is history: changing its offset would be changing
 * when something already happened. The server owns the state and this reads it.
 */
export function isPending(reminder: Reminder): boolean {
  return (reminder.state as string) === 'PENDING';
}

/**
 * Why a relative reminder cannot be set on this entry, or nothing when it can.
 *
 * `reminders.due_date_required` is the **server's own code** — "A reminder counted from the due
 * date needs the entry to have one." So one fact has one sentence whether this client saw the
 * refusal coming or the server sent it, which is the rule every prediction in this application
 * keeps.
 */
export function relativeRefusal(item: WorkItem): string | undefined {
  return item.due_at ? undefined : 'reminders.due_date_required';
}

/** Whether one more reminder may be added, against the manifest's bound. */
export function mayAddAnother(count: number, limit: number | undefined): boolean {
  return limit === undefined || count < limit;
}

/** How many reminders one entry carries at most, as the installation reports it. */
export function reminderLimitOf(limits: Record<string, unknown> | undefined): number | undefined {
  const declared = limits?.['max_reminders_per_item'];
  if (typeof declared !== 'number' || !Number.isFinite(declared) || declared <= 0) return undefined;
  return declared;
}

// --- the series ---------------------------------------------------------------------------------

/**
 * Whether the entry is an occurrence of a series rather than the entry the series belongs to.
 *
 * The contract carries `recurrence_source_id` on an occurrence, naming the entry it was copied
 * from. A row that has one links up to it; a row that *is* one carries the series itself.
 */
export function seriesSourceOf(item: WorkItem): string | undefined {
  return (item as { recurrence_source_id?: string | null }).recurrence_source_id ?? undefined;
}

/**
 * Which of the two ends a rule states, if either.
 *
 * RFC 5545 forbids both at once and so does the contract — "at most one of ends_at and max_count" —
 * so this answers one thing rather than two booleans a caller would have to reconcile.
 */
export function endOf(
  rule: { ends_at?: string | null; max_count?: number | null } | undefined,
): { kind: 'never' } | { kind: 'on'; date: string } | { kind: 'after'; count: number } {
  if (rule?.ends_at) return { kind: 'on', date: rule.ends_at };
  if (rule?.max_count) return { kind: 'after', count: rule.max_count };
  return { kind: 'never' };
}
