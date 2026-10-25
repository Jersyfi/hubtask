// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The words a capture line's dates are read in (UC-WRK-26 check 5): the relative days and the
 * hour words from the catalogue, the weekday and month names from `Intl`.
 *
 * **One language for both halves.** The catalogue falls back to the source language code by code
 * (`catalogue.ts`, `patternFor`), so a reader whose catalogue lacks these words gets the English
 * ones. `app.capture.words.language` says which catalogue answered, and the names are taken from
 * `Intl` in **that** language - an English "tomorrow" beside German weekday names would be a line
 * nobody types.
 *
 * Pure: the caller hands in its `t`.
 */

export interface Vocabulary {
  /** The language the words came from, as `app.capture.words.language` says it. */
  readonly language: string;
  readonly today: readonly string[];
  readonly tomorrow: readonly string[];
  /** Phrases of one or more words: "day after tomorrow", "übermorgen". */
  readonly dayAfterTomorrow: readonly string[];
  /** "in" of "in 3 days". */
  readonly inWords: readonly string[];
  /** "days" of "in 3 days", every form the language writes after a number. */
  readonly days: readonly string[];
  /** "o'clock", "Uhr": what makes a bare number a time. */
  readonly hour: readonly string[];
  /** "on", "at", "am", "um": a word before a day or a time that belongs to it, not to the title. */
  readonly on: readonly string[];
  readonly am: readonly string[];
  readonly pm: readonly string[];
  /** Monday first: index 0 is Monday, 6 is Sunday. Folded. */
  readonly weekdays: readonly string[];
  readonly weekdaysShort: readonly string[];
  /** January first. Folded. */
  readonly months: readonly string[];
  readonly monthsShort: readonly string[];
  /** Whether a numeric date is written day first (`3.11.`) or month first (`11/3`). */
  readonly dayFirst: boolean;
}

/** What `t` is to this module: a code in, its text out. */
export type Translate = (code: string) => string;

/** Folded for comparing what was typed: case, composition and a trailing full stop do not count. */
export function fold(word: string): string {
  return word.normalize('NFC').toLocaleLowerCase().replace(/\.$/u, '');
}

/** The forms a catalogue entry lists, `|`-separated; an empty entry is none. */
function formsOf(written: string): readonly string[] {
  return written
    .split('|')
    .map((form) => fold(form.trim()))
    .filter((form) => form !== '');
}

// A Monday and the first of January, read in UTC so the machine's zone cannot move them.
const A_MONDAY = Date.UTC(2024, 0, 1);
const DAY = 86_400_000;

function names(language: string, options: Intl.DateTimeFormatOptions, count: number, at: (index: number) => number) {
  const format = new Intl.DateTimeFormat(language, { ...options, timeZone: 'UTC' });
  return Array.from({ length: count }, (_, index) => fold(format.format(new Date(at(index)))));
}

/** The vocabulary of the catalogue that is rendering. */
export function vocabularyOf(t: Translate): Vocabulary {
  const language = t('app.capture.words.language').trim() || 'en';
  const parts = new Intl.DateTimeFormat(language, { day: 'numeric', month: 'numeric', timeZone: 'UTC' }).formatToParts(new Date(A_MONDAY));
  const dayAt = parts.findIndex((part) => part.type === 'day');
  const monthAt = parts.findIndex((part) => part.type === 'month');
  return {
    language,
    today: formsOf(t('app.capture.words.today')),
    tomorrow: formsOf(t('app.capture.words.tomorrow')),
    dayAfterTomorrow: formsOf(t('app.capture.words.day_after_tomorrow')),
    inWords: formsOf(t('app.capture.words.in')),
    days: formsOf(t('app.capture.words.days')),
    hour: formsOf(t('app.capture.words.hour')),
    on: formsOf(t('app.capture.words.on')),
    am: formsOf(t('app.capture.words.am')),
    pm: formsOf(t('app.capture.words.pm')),
    weekdays: names(language, { weekday: 'long' }, 7, (index) => A_MONDAY + index * DAY),
    weekdaysShort: names(language, { weekday: 'short' }, 7, (index) => A_MONDAY + index * DAY),
    months: names(language, { month: 'long' }, 12, (index) => Date.UTC(2024, index, 15)),
    monthsShort: names(language, { month: 'short' }, 12, (index) => Date.UTC(2024, index, 15)),
    dayFirst: dayAt < monthAt,
  };
}
