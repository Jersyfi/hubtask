// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a capture line says besides its title (UC-WRK-26 checks 2, 3 and 5): a hub (`#`), a
 * collection (`@`), a person (`+`), a label (`!`), a day and a time. Pure: the clock, the zone,
 * the words and the names are inputs, so a midnight in any zone is a table row.
 *
 * **Two passes.** `readTargets` finds `#` and `@` first, because where the entry goes decides what
 * else is read: a collection without due dates takes no date, the jumble takes nothing
 * (`readLine`'s `recognition`, which the caller derives from the target). Reading a date before the
 * target is known would show a chip the write then refuses.
 *
 * **Names are matched, not delimited.** `#Client A` and `@Kitchen garden` need no quotes: the
 * longest known name that the text continues with, ending where a word ends, wins.
 *
 * **Narrow on purpose.** A bare number is never a time ("Buy 11 eggs"), a month name needs a day
 * beside it ("may"), and an abbreviated weekday needs its full stop or a time after it ("sat").
 * A wrong chip is worse than none: the person sees every chip before Enter and can take it back,
 * and a removed chip's text is held literal for the rest of the line (`literal`).
 */

import { addDays } from '../data/due.ts';
import { timeIn, todayIn } from '../i18n/zone.ts';
import { fold, type Vocabulary } from './words.ts';

export type ChipKind = 'hub' | 'collection' | 'person' | 'label' | 'date' | 'time';

export interface Chip {
  readonly kind: ChipKind;
  /** The text the chip stands for, as typed (composed). */
  readonly text: string;
  readonly start: number;
  readonly end: number;
  /** The hub, collection, account or label it names. */
  readonly id?: string;
  /** A day, `YYYY-MM-DD`, for a `date` chip. */
  readonly date?: string;
  /** A time, `HH:mm`, for a `time` chip. */
  readonly time?: string;
}

export interface Named {
  readonly id: string;
  readonly name: string;
}

export interface NamedCollection extends Named {
  readonly hubId: string;
}

/** What the target recognises besides itself (D4); the caller derives it from the target. */
export interface Recognition {
  readonly date: boolean;
  readonly person: boolean;
  readonly label: boolean;
  /** Who `+` may name. */
  readonly people: readonly Named[];
  /** What `!` may name. */
  readonly labels: readonly Named[];
}

/** Nothing recognised: the jumble, a target still being worked out, nowhere. */
export const NOTHING: Recognition = { date: false, person: false, label: false, people: [], labels: [] };

export interface Context {
  readonly now: number;
  readonly zone: string;
  readonly words: Vocabulary;
  /** The hubs a `#` may name: those the reader sees, not archived. */
  readonly hubs: readonly Named[];
  /** The collections an `@` may name, in tree order. */
  readonly collections: readonly NamedCollection[];
  /** The hub the reader stands in, which settles an `@` name two hubs share. */
  readonly currentHubId?: string;
  /** The chips removed in this text, as `literalKey` writes them. */
  readonly literal: ReadonlySet<string>;
}

/** How a removed chip is remembered, so the next reading leaves its text in the title. */
export const literalKey = (kind: ChipKind, text: string): string => `${kind}:${fold(text)}`;

/** Where the entry is meant to go, as the line names it. */
export interface Targets {
  readonly hub?: Chip;
  readonly collection?: Chip;
}

export interface Reading {
  readonly title: string;
  readonly chips: readonly Chip[];
}

// ------------------------------------------------------------------------------------------------
// Names
// ------------------------------------------------------------------------------------------------

const ENDS_WORD = /^(?:$|[\s,;:!?.)\]])/u;

/** Whether a sigil stands at the start of a word. */
const startsWord = (text: string, at: number) => at === 0 || /\s/u.test(text[at - 1] ?? '');

/** The longest name the text continues with at `from`, ending where a word ends. */
function nameAt<T extends Named>(text: string, from: number, candidates: readonly T[]): T | undefined {
  let best: T | undefined;
  for (const candidate of candidates) {
    const name = candidate.name.normalize('NFC').trim();
    if (name === '' || (best && name.length <= best.name.normalize('NFC').trim().length)) continue;
    const typed = text.slice(from, from + name.length);
    if (fold(typed) !== fold(name) || typed.length !== name.length) continue;
    if (!ENDS_WORD.test(text.slice(from + name.length))) continue;
    best = candidate;
  }
  return best;
}

/** Every `sigil` + known name in the text, first one per name kind unless `isMany`. */
function sigilled<T extends Named>(
  text: string,
  sigil: string,
  kind: ChipKind,
  candidates: readonly T[],
  literal: ReadonlySet<string>,
  taken: readonly Chip[],
): { chip: Chip; named: T }[] {
  const found: { chip: Chip; named: T }[] = [];
  for (let at = text.indexOf(sigil); at >= 0; at = text.indexOf(sigil, at + 1)) {
    if (!startsWord(text, at) || taken.some((chip) => at >= chip.start && at < chip.end)) continue;
    const named = nameAt(text, at + 1, candidates);
    if (!named) continue;
    const end = at + 1 + named.name.normalize('NFC').trim().length;
    const chipText = text.slice(at, end);
    if (literal.has(literalKey(kind, chipText))) continue;
    found.push({ chip: { kind, text: chipText, start: at, end, id: named.id }, named });
  }
  return found;
}

/** A person's first name answers too, where nobody else shares it: "+Mara" for Mara Lind. */
function withFirstNames(people: readonly Named[]): readonly Named[] {
  const firsts = new Map<string, Named[]>();
  for (const person of people) {
    const first = person.name.trim().split(/\s+/u)[0] ?? '';
    if (first === '' || first === person.name.trim()) continue;
    firsts.set(fold(first), [...(firsts.get(fold(first)) ?? []), { id: person.id, name: first }]);
  }
  const unique = [...firsts.values()].filter((each) => each.length === 1).map((each) => each[0]!);
  return [...people, ...unique];
}

/** `#` and `@`: where the line says the entry goes. */
export function readTargets(raw: string, context: Context): Targets {
  const text = raw.normalize('NFC');
  const hubChip = sigilled(text, '#', 'hub', context.hubs, context.literal, [])[0];
  const hubId = hubChip?.chip.id;

  const ats = sigilled(text, '@', 'collection', context.collections, context.literal, hubChip ? [hubChip.chip] : []);
  // One name, two hubs: the one in the named hub, else the reader's, else the first in the tree.
  const first = ats[0];
  if (!first) return hubChip ? { hub: hubChip.chip } : {};
  const sameName = context.collections.filter((each) => fold(each.name) === fold(first.named.name));
  const chosen =
    sameName.find((each) => each.hubId === hubId) ??
    (hubId ? undefined : sameName.find((each) => each.hubId === context.currentHubId)) ??
    (hubId ? undefined : sameName[0]);
  const collection = chosen ? { ...first.chip, id: chosen.id } : undefined;
  return { ...(hubChip ? { hub: hubChip.chip } : {}), ...(collection ? { collection } : {}) };
}

// ------------------------------------------------------------------------------------------------
// Days and times
// ------------------------------------------------------------------------------------------------

interface Word {
  /** Folded, without trailing punctuation or full stop. */
  readonly core: string;
  readonly start: number;
  readonly end: number;
  /** Whether it ended in a full stop: "Fri." */
  readonly isStopped: boolean;
}

function wordsOf(text: string): Word[] {
  const out: Word[] = [];
  for (const match of text.matchAll(/\S+/gu)) {
    const raw = match[0];
    const trimmed = raw.replace(/[,;:!?)\]]+$/u, '');
    const end = (match.index ?? 0) + trimmed.length;
    out.push({ core: fold(trimmed), start: match.index ?? 0, end, isStopped: trimmed.endsWith('.') });
  }
  return out;
}

/** How many words from `at` spell one of the phrases, or 0. */
function phraseAt(words: readonly Word[], at: number, phrases: readonly string[]): number {
  let best = 0;
  for (const phrase of phrases) {
    const parts = phrase.split(/\s+/u);
    if (parts.length <= best) continue;
    if (parts.every((part, index) => words[at + index]?.core === part)) best = parts.length;
  }
  return best;
}

const pad = (value: number) => String(value).padStart(2, '0');

function validDay(year: number, month: number, day: number): string | undefined {
  if (month < 1 || month > 12 || day < 1) return undefined;
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCMonth() !== month - 1) return undefined;
  return `${year}-${pad(month)}-${pad(day)}`;
}

/** A day and month without a year: this year's if it is still ahead or today, else next year's. */
function nextOccurrence(today: string, month: number, day: number): string | undefined {
  const year = Number(today.slice(0, 4));
  const thisYear = validDay(year, month, day);
  if (thisYear && thisYear >= today) return thisYear;
  return validDay(year + 1, month, day) ?? thisYear;
}

/** Monday 0 … Sunday 6. */
const isoWeekdayOf = (date: string) => (new Date(`${date}T00:00:00Z`).getUTCDay() + 6) % 7;

/** The next such weekday, never today: "Friday" said on a Friday is next week's. */
function nextWeekday(today: string, weekday: number): string {
  const ahead = (weekday - isoWeekdayOf(today) + 7) % 7 || 7;
  return addDays(today, ahead);
}

interface DayMatch {
  readonly length: number;
  readonly date: string;
}

interface TimeMatch {
  readonly length: number;
  readonly time: string;
}

const DAY_NUMBER = /^(\d{1,2})(?:\.|st|nd|rd|th)?$/u;

function dayAt(words: readonly Word[], at: number, vocabulary: Vocabulary, today: string): DayMatch | undefined {
  const word = words[at];
  if (!word) return undefined;

  let length = phraseAt(words, at, vocabulary.dayAfterTomorrow);
  if (length > 0) return { length, date: addDays(today, 2) };
  length = phraseAt(words, at, vocabulary.tomorrow);
  if (length > 0) return { length, date: addDays(today, 1) };
  length = phraseAt(words, at, vocabulary.today);
  if (length > 0) return { length, date: today };

  // "in 3 days"
  if (vocabulary.inWords.includes(word.core)) {
    const count = /^\d{1,3}$/u.test(words[at + 1]?.core ?? '') ? Number(words[at + 1]!.core) : undefined;
    if (count !== undefined && vocabulary.days.includes(words[at + 2]?.core ?? '')) return { length: 3, date: addDays(today, count) };
  }

  // An ISO day.
  const iso = /^(\d{4})-(\d{2})-(\d{2})$/u.exec(word.core);
  if (iso) {
    const date = validDay(Number(iso[1]), Number(iso[2]), Number(iso[3]));
    return date ? { length: 1, date } : undefined;
  }

  // A numeric day and month in the language's order: "3.11." or "3.11.2026", "11/3". A dot
  // between two numbers alone is a decimal ("1.5"), so the dotted form needs its last dot or a year.
  const raw = words[at]!;
  const numeric = /^(\d{1,2})([./])(\d{1,2})(?:\2(\d{4}|\d{2})?)?$/u.exec(raw.core + (raw.isStopped ? '.' : ''));
  if (numeric) {
    const isDotted = numeric[2] === '.';
    const hasTail = numeric[4] !== undefined || (raw.core + (raw.isStopped ? '.' : '')).endsWith('.');
    if (!isDotted || hasTail) {
      const first = Number(numeric[1]);
      const second = Number(numeric[3]);
      const [day, month] = vocabulary.dayFirst ? [first, second] : [second, first];
      const year = numeric[4] ? Number(numeric[4].length === 2 ? `20${numeric[4]}` : numeric[4]) : undefined;
      const date = year !== undefined ? validDay(year, month, day) : nextOccurrence(today, month, day);
      if (date) return { length: 1, date };
    }
  }

  // A month name needs a day number beside it: "3 May", "May 3", "3. Mai".
  const monthOf = (core: string) => {
    const full = vocabulary.months.indexOf(core);
    return full >= 0 ? full + 1 : vocabulary.monthsShort.indexOf(core) + 1 || undefined;
  };
  const dayNumber = (word: Word | undefined) => {
    const match = DAY_NUMBER.exec((word?.core ?? '') + (word?.isStopped ? '.' : ''));
    return match ? Number(match[1]) : undefined;
  };
  const monthFirst = monthOf(word.core);
  const dayAfter = monthFirst ? dayNumber(words[at + 1]) : undefined;
  if (monthFirst && dayAfter) {
    const date = nextOccurrence(today, monthFirst, dayAfter);
    if (date) return { length: 2, date };
  }
  const dayFirst = dayNumber(word);
  const monthAfter = dayFirst !== undefined ? monthOf(words[at + 1]?.core ?? '') : undefined;
  if (dayFirst !== undefined && monthAfter) {
    const date = nextOccurrence(today, monthAfter, dayFirst);
    if (date) return { length: 2, date };
  }

  // A weekday: written out, or abbreviated with its full stop. One followed by a time is read in
  // `readDates`, where the time is known.
  const full = vocabulary.weekdays.indexOf(word.core);
  if (full >= 0) return { length: 1, date: nextWeekday(today, full) };
  const short = vocabulary.weekdaysShort.indexOf(word.core);
  if (short >= 0 && word.isStopped) return { length: 1, date: nextWeekday(today, short) };
  return undefined;
}

function timeAt(words: readonly Word[], at: number, vocabulary: Vocabulary): TimeMatch | undefined {
  const word = words[at];
  if (!word) return undefined;
  const core = word.core + (word.isStopped ? '.' : '');

  const meridiem = (suffix: string): 'am' | 'pm' | undefined => {
    const folded = fold(suffix);
    if (vocabulary.am.some((form) => fold(form) === folded)) return 'am';
    if (vocabulary.pm.some((form) => fold(form) === folded)) return 'pm';
    return undefined;
  };
  const clock = (hour: number, minute: number, half?: 'am' | 'pm'): string | undefined => {
    if (minute > 59) return undefined;
    if (half) {
      if (hour < 1 || hour > 12) return undefined;
      return `${pad((hour % 12) + (half === 'pm' ? 12 : 0))}:${pad(minute)}`;
    }
    return hour <= 23 ? `${pad(hour)}:${pad(minute)}` : undefined;
  };

  // "11am", "11:30pm", "11 a.m."
  const glued = /^(\d{1,2})(?::(\d{2}))?([^\d:].*)$/u.exec(core);
  const gluedHalf = glued ? meridiem(glued[3] ?? '') : undefined;
  if (glued && gluedHalf) {
    const time = clock(Number(glued[1]), Number(glued[2] ?? 0), gluedHalf);
    return time ? { length: 1, time } : undefined;
  }

  const number = /^(\d{1,2})(?::(\d{2}))?$/u.exec(word.core);
  if (!number) return undefined;
  const hasColon = number[2] !== undefined;
  const next = words[at + 1];
  const nextCore = next ? next.core + (next.isStopped ? '.' : '') : '';
  const half = next ? meridiem(nextCore) : undefined;
  if (half) {
    const time = clock(Number(number[1]), Number(number[2] ?? 0), half);
    return time ? { length: 2, time } : undefined;
  }
  if (next && vocabulary.hour.includes(next.core)) {
    const time = clock(Number(number[1]), Number(number[2] ?? 0));
    return time ? { length: 2, time } : undefined;
  }
  // A bare number is a count, not a time; a colon makes it one.
  if (!hasColon) return undefined;
  const time = clock(Number(number[1]), Number(number[2]));
  return time ? { length: 1, time } : undefined;
}

/** The first day and the first time in the text, each as a chip. */
function readDates(text: string, context: Context): Chip[] {
  const words = wordsOf(text);
  const today = todayIn(context.zone, context.now);
  let day: Chip | undefined;
  let time: Chip | undefined;

  const chipOf = (kind: 'date' | 'time', from: number, length: number, value: string): Chip | undefined => {
    const start = words[from]!.start;
    const end = words[from + length - 1]!.end;
    const chipText = text.slice(start, end);
    if (context.literal.has(literalKey(kind, chipText))) return undefined;
    return kind === 'date' ? { kind, text: chipText, start, end, date: value } : { kind, text: chipText, start, end, time: value };
  };

  for (let at = 0; at < words.length; at += 1) {
    if (!time) {
      const found = timeAt(words, at, context.words);
      if (found) {
        const chip = chipOf('time', at, found.length, found.time);
        if (chip) {
          time = chip;
          at += found.length - 1;
          continue;
        }
      }
    }
    if (!day) {
      const found = dayAt(words, at, context.words, today);
      if (found) {
        const chip = chipOf('date', at, found.length, found.date);
        if (chip) {
          day = chip;
          at += found.length - 1;
          continue;
        }
      }
      // An abbreviated weekday without its full stop counts when a time follows it: "Fri 10:00".
      const short = context.words.weekdaysShort.indexOf(words[at]!.core);
      if (short >= 0 && !time && timeAt(words, at + 1, context.words)) {
        const chip = chipOf('date', at, 1, nextWeekday(today, short));
        if (chip) {
          day = chip;
          continue;
        }
      }
    }
  }
  return [day, time].filter((chip): chip is Chip => chip !== undefined);
}

/**
 * The day and time the chips say, ready for `dueInputFor`. A time without a day is today's while it
 * is still ahead where the person is, else tomorrow's.
 */
export function dueOf(chips: readonly Chip[], context: Pick<Context, 'now' | 'zone'>): { date: string; time?: string } | undefined {
  const day = chips.find((chip) => chip.kind === 'date')?.date;
  const time = chips.find((chip) => chip.kind === 'time')?.time;
  if (day) return time ? { date: day, time } : { date: day };
  if (!time) return undefined;
  const today = todayIn(context.zone, context.now);
  const nowTime = timeIn(new Date(context.now).toISOString(), context.zone) ?? '00:00';
  return { date: time > nowTime ? today : addDays(today, 1), time };
}

// ------------------------------------------------------------------------------------------------
// The line
// ------------------------------------------------------------------------------------------------

/** Spaces where the chips are, so later passes neither see them nor shift a position. */
const blank = (text: string, chips: readonly Chip[]) =>
  chips.reduce((out, chip) => out.slice(0, chip.start) + ' '.repeat(chip.end - chip.start) + out.slice(chip.end), text);

/**
 * Everything the line says under the target's recognition, and what is left as the title.
 *
 * `targets` are `readTargets`' chips, kept as they are. One person (an entry has one responsible),
 * every label, the first day and the first time.
 */
export function readLine(raw: string, context: Context, targets: Targets, recognition: Recognition): Reading {
  const text = raw.normalize('NFC');
  const chips: Chip[] = [targets.hub, targets.collection].filter((chip): chip is Chip => chip !== undefined);

  if (recognition.person) {
    const person = sigilled(text, '+', 'person', withFirstNames(recognition.people), context.literal, chips)[0];
    if (person) chips.push(person.chip);
  }
  if (recognition.label) {
    const seen = new Set<string>();
    for (const label of sigilled(text, '!', 'label', recognition.labels, context.literal, chips)) {
      if (seen.has(label.chip.id ?? '')) continue;
      seen.add(label.chip.id ?? '');
      chips.push(label.chip);
    }
  }
  if (recognition.date) chips.push(...readDates(blank(text, chips), context));

  chips.sort((a, b) => a.start - b.start);
  const title = blank(text, chips).replace(/\s+/gu, ' ').trim();
  return { title, chips };
}
