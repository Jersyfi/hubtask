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
 * **Years and months are refused in a duration** — the contract's own rule, and `duration.ts`'s,
 * because a template's `due_offset` refuses them for the same reason and one rule written twice is
 * one rule that will drift.
 *
 * **`LAPSED` is a state the schema does not declare.** It is real — `offline-sync.md` §8, and a
 * restore produces it — so the state is read as a string, and one this client has never met still
 * renders as words.
 */

import type { Reminder, WorkItem } from '@hubtask/sync-engine';

import { isPlainDuration } from './duration.ts';

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

  // The duration rule is `duration.ts`, because a template's `due_offset` refuses years and months
  // for the same reason and one rule written twice is one rule that will drift.
  return isPlainDuration(spec.slice(RELATIVE.length));
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
 * Whether the entry belongs to a series at all — as its template, or as one of its occurrences.
 *
 * `recurrence_rule_id` is set on the template *and* on every occurrence, so it says "this belongs
 * to a series" and nothing about which end. `occurrenceSourceOf` answers that, without a request.
 */
export function belongsToSeries(item: WorkItem): boolean {
  return Boolean((item as { recurrence_rule_id?: string | null }).recurrence_rule_id);
}

/**
 * The entry an occurrence repeats from, or undefined for anything that is not an occurrence.
 *
 * `recurrence_source_id` is written only on a copy the materialisation made, so it tells the two
 * ends of a series apart from the row itself — no request, which is what lets a list mark a
 * template and an occurrence differently. It is also the only thing an occurrence can link *to*:
 * `/items/{id}/recurrence` finds the rule by the entry it belongs to and answers `404` for an
 * occurrence, and no route resolves a rule by its own identifier.
 */
export function occurrenceSourceOf(item: WorkItem): string | undefined {
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

/**
 * The rule and its end, told apart — because the editor writes them together and the API takes
 * them apart.
 *
 * `RecurrenceEditor` composes an RFC 5545 rule, and RFC 5545 puts the end *inside* it as `UNTIL=`
 * or `COUNT=`. This API does not: it refuses a rule that carries either —
 * `recurrence.rrule_carries_end`, "A rule carries no UNTIL or COUNT: send ends_at or max_count
 * instead" — and takes the end as two fields of the request document beside the rule.
 *
 * Both are right. A rule that carried its own end could disagree with the fields beside it, and an
 * editor that could not express an end would be an editor missing a third of RFC 5545. So the
 * translation lives here, at the seam between the two, and is tested rather than trusted.
 */
export function splitEnd(rrule: string): {
  rrule: string;
  ends_at?: string | null;
  max_count?: number | null;
} {
  const kept: string[] = [];
  let endsAt: string | undefined;
  let maxCount: number | undefined;

  for (const part of rrule.split(';')) {
    const [name, value] = part.split('=');
    if (name === 'UNTIL' && value) endsAt = instantFromUntil(value);
    else if (name === 'COUNT' && value) maxCount = Number(value);
    else if (part !== '') kept.push(part);
  }

  return {
    rrule: kept.join(';'),
    // Null rather than absent, so that changing a series *from* having an end back to having none
    // clears the stored one. An omitted field on a `PUT` of the whole document would be the same
    // as a null here, and stating it is what makes that not need checking.
    ends_at: endsAt ?? null,
    max_count: Number.isFinite(maxCount) ? (maxCount as number) : null,
  };
}

/** `20261231T000000Z` — RFC 5545's own form — as the instant the contract takes. */
function instantFromUntil(value: string): string | undefined {
  const match = /^(\d{4})(\d{2})(\d{2})(?:T(\d{2})(\d{2})(\d{2})Z?)?$/.exec(value);
  if (!match) return undefined;
  const [, year, month, day, hour = '00', minute = '00', second = '00'] = match;
  return `${year}-${month}-${day}T${hour}:${minute}:${second}.000Z`;
}

/** …and the other way, so a stored series opens in the editor with its end showing. */
export function withEnd(
  rrule: string,
  rule: { ends_at?: string | null; max_count?: number | null } | undefined,
): string {
  const end = endOf(rule);
  if (end.kind === 'on') {
    const at = new Date(end.date);
    if (Number.isNaN(at.getTime())) return rrule;
    const stamp = at.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '');
    return `${rrule};UNTIL=${stamp}`;
  }
  if (end.kind === 'after') return `${rrule};COUNT=${end.count}`;
  return rrule;
}
