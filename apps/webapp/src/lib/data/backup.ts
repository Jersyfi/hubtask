// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a backup schedule's rule says, without working out when it will next fire.
 *
 * **Nothing here expands an RRULE.** ADR-0008 put recurrence behind one library on the server for
 * the reason `RecurrenceEditor` records: two implementations of a calendar disagree on the first
 * leap day, and a client that computed occurrences would be the second one. `next_run_at` is the
 * server's answer and the screen shows that. This reads the rule into its parts so the screen can
 * say what it *means* — "every day at 03:00" rather than `FREQ=DAILY;BYHOUR=3;BYMINUTE=0` — which
 * is a different question from when it fires, and one a parser can answer honestly.
 *
 * A part this reading does not know is reported rather than dropped: a rule that says more than
 * the sentence says is a rule whose sentence is a lie, and the screen falls back to the text.
 */

/** The four frequencies a sentence exists for. Anything else is read as unrecognised. */
export type Frequency = 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY';

const FREQUENCIES: readonly string[] = ['DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY'];

/** The parts of `BYDAY` this reading understands, in the order RFC 5545 writes them. */
const WEEKDAYS: readonly string[] = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'];

/** What a rule was read as. Every field is what the rule said, never a default nobody chose. */
export interface RuleReading {
  readonly frequency?: Frequency;
  /** `INTERVAL`, defaulting to 1 as RFC 5545 does. */
  readonly interval: number;
  /** `BYDAY`, filtered to the seven plain weekday names. */
  readonly weekdays: readonly string[];
  /** `BYMONTHDAY`. */
  readonly monthDays: readonly number[];
  /** `BYHOUR`/`BYMINUTE` as one time of day, where the rule pins both. */
  readonly at?: { readonly hour: number; readonly minute: number };
  /**
   * True when the rule carries something this reading does not cover.
   *
   * The screen shows the rule's own text then. A sentence that quietly omitted `BYSETPOS=-1`
   * would describe a schedule that does not exist.
   */
  readonly partial: boolean;
}

function toNumbers(value: string): number[] {
  return value
    .split(',')
    .map((part) => Number.parseInt(part, 10))
    .filter((number) => Number.isFinite(number));
}

/**
 * Reads an RFC 5545 rule into the parts a sentence is made of.
 *
 * The `RRULE:` prefix is accepted and dropped: the contract stores the rule without it, and a
 * value pasted out of a calendar file carries it.
 */
export function readRule(rrule: string): RuleReading {
  const text = rrule.trim().replace(/^RRULE:/i, '');
  let frequency: Frequency | undefined;
  let interval = 1;
  let weekdays: string[] = [];
  let monthDays: number[] = [];
  let hour: number | undefined;
  let minute: number | undefined;
  let partial = text === '';

  for (const part of text.split(';')) {
    if (part === '') continue;
    const separator = part.indexOf('=');
    const name = (separator < 0 ? part : part.slice(0, separator)).toUpperCase();
    const value = separator < 0 ? '' : part.slice(separator + 1);
    switch (name) {
      case 'FREQ':
        if (FREQUENCIES.includes(value.toUpperCase())) frequency = value.toUpperCase() as Frequency;
        else partial = true;
        break;
      case 'INTERVAL': {
        const parsed = Number.parseInt(value, 10);
        if (Number.isFinite(parsed) && parsed > 0) interval = parsed;
        else partial = true;
        break;
      }
      case 'BYDAY': {
        const named = value.toUpperCase().split(',');
        weekdays = named.filter((day) => WEEKDAYS.includes(day));
        // `2MO` is the second Monday and is not "Monday": a sentence that dropped the ordinal
        // would name the wrong day three weeks in four.
        if (weekdays.length !== named.length) partial = true;
        break;
      }
      case 'BYMONTHDAY':
        monthDays = toNumbers(value);
        break;
      case 'BYHOUR': {
        const hours = toNumbers(value);
        if (hours.length === 1) hour = hours[0];
        else partial = true;
        break;
      }
      case 'BYMINUTE': {
        const minutes = toNumbers(value);
        if (minutes.length === 1) minute = minutes[0];
        else partial = true;
        break;
      }
      case 'WKST':
        // Which day a week starts on changes nothing about the sentence for the rules above.
        break;
      default:
        partial = true;
    }
  }

  // A rule that pins the hour and not the minute fires on the hour, which is what RFC 5545 means
  // by omitting it here — the value comes from the anchor, and the anchor's minute is zero for
  // every schedule the product creates.
  const at = hour === undefined ? undefined : { hour, minute: minute ?? 0 };

  return { frequency, interval, weekdays, monthDays, at, partial };
}

/** The generation plan, as `BackupRetention` writes it. */
export interface Retention {
  readonly keep_last?: number;
  readonly keep_daily?: number;
  readonly keep_weekly?: number;
  readonly keep_monthly?: number;
  readonly keep_yearly?: number;
  readonly min_keep?: number;
}

/** One line of the plan: which generation, and how many of it are kept. */
export interface Generation {
  readonly kind: 'last' | 'daily' | 'weekly' | 'monthly' | 'yearly';
  readonly keep: number;
}

/**
 * The plan as lines, skipping the generations this schedule keeps none of.
 *
 * `min_keep` is deliberately not among them: it is not a generation but a floor under all of
 * them — "a retention rule may never result in no backup being left" (`backup-restore.md` §6) —
 * and listing it beside the others would read as a sixth kind of archive.
 */
export function generations(retention: Retention | undefined): readonly Generation[] {
  const plan = retention ?? {};
  const lines: Generation[] = [
    { kind: 'last', keep: plan.keep_last ?? 0 },
    { kind: 'daily', keep: plan.keep_daily ?? 0 },
    { kind: 'weekly', keep: plan.keep_weekly ?? 0 },
    { kind: 'monthly', keep: plan.keep_monthly ?? 0 },
    { kind: 'yearly', keep: plan.keep_yearly ?? 0 },
  ];
  return lines.filter((line) => line.keep > 0);
}
