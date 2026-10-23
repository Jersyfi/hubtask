// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a capture line becomes: one entry per line, and a leading amount read off each line
 * (UC-WRK-25 checks 1 and 2, UC-WRK-26 checks 2 and 3). Pure, so that the list's capture line and
 * the global capture field read a collection and a line the same way.
 *
 * **What counts as an amount** is deliberately narrow, because a wrong split is worse than none: a
 * number at the start - any script's decimal digits, one decimal comma or point with at most two
 * places, a vulgar fraction, a digit with a fraction, or `1/2` - optionally followed, directly or
 * after a space, by `x`, `×` or one unit of the locale's list, and then a space and the rest of the
 * title. A range (`2-3`), a thousands separator (`1.000`), a number glued to a word (`7up`) and a
 * number alone stay in the title. The person sees the split as a chip before submitting and can
 * take it back.
 */

import type { EffectiveStructure } from '@hubtask/sync-engine';

/** How many lines one submit creates at most; the rest stay in the field, with a note. */
export const MAX_CAPTURE_LINES = 100;

export interface CaptureRules {
  /** Several lines are several entries: a collection of one level, or one that recognises `LINES`. */
  readonly splitsLines: boolean;
  /** A leading amount is read: the collection recognises `AMOUNT` and the type carries it. */
  readonly readsAmount: boolean;
  /** The unit words of the reader's locale, from the client's catalogue (`app.entries.capture_units`). */
  readonly units: readonly string[];
}

export interface CapturedLine {
  readonly title: string;
  readonly amount?: string;
}

export interface Capture {
  /** The entries this submit creates, in order. */
  readonly lines: readonly CapturedLine[];
  /** What stays in the field: the lines past the limit, as they were typed. */
  readonly rest: string;
}

/**
 * The one rule every capture field reads a collection by (record D7): the list's capture line and
 * the capture field that `Q` opens. Several lines split where the collection has one level in force
 * - check 1 holds for every one-level collection, whichever field the lines are typed into - or
 * where its structure recognises `LINES`; a leading amount is read where it recognises `AMOUNT` and
 * the type carries `AMOUNT` there. No levels answered is Hubtask's default, every type the
 * installation has, which is not one level.
 */
export function captureRulesOf(
  structure: Pick<EffectiveStructure, 'settings'> | undefined,
  itemType: string,
  units: readonly string[],
): CaptureRules {
  const settings = structure?.settings;
  const recognises = (token: 'LINES' | 'AMOUNT') => (settings?.capture ?? []).includes(token);
  return {
    splitsLines: settings?.levels?.length === 1 || recognises('LINES'),
    readsAmount: recognises('AMOUNT') && (settings?.fields?.[itemType]?.capabilities ?? []).includes('AMOUNT'),
    units,
  };
}

/** The unit list as the catalogue writes it: words separated by spaces. */
export function unitsOf(written: string): readonly string[] {
  return written.split(/\s+/u).filter((word) => word !== '');
}

/** A list marker somebody pasted along with the line: `-`, `•`, `*`, `[ ]`, `[x]`. */
const BULLET = /^(?:[-•*]|\[[ xX]?\])\s+/u;

/** Any script's decimal digit, as the ASCII digit it stands for. */
function asciiDigits(text: string): string {
  let out = '';
  for (const character of text) {
    if (!/\p{Nd}/u.test(character)) {
      out += character;
      continue;
    }
    // Decimal digits are encoded as runs of ten from zero (Unicode's Nd property): the distance
    // from the start of the run, modulo ten, is the digit.
    const point = character.codePointAt(0) ?? 0;
    let start = point;
    while (start > 0 && /\p{Nd}/u.test(String.fromCodePoint(start - 1))) start -= 1;
    out += String((point - start) % 10);
  }
  return out;
}

const FRACTION = '½¼¾⅓⅔⅛';
/** The number at the start of a line, in ASCII digits. */
const NUMBER = new RegExp(`^(?:\\d+/\\d+|\\d+(?:[.,]\\d{1,2})?[${FRACTION}]?|[${FRACTION}])`, 'u');

/** One line, with the amount read off its start where it has one. */
export function parseLine(raw: string, rules: CaptureRules): CapturedLine {
  const line = raw.trim().replace(BULLET, '').trim();
  if (!rules.readsAmount) return { title: line };

  const normalised = asciiDigits(line);
  const number = NUMBER.exec(normalised)?.[0];
  if (!number) return { title: line };

  let at = number.length;
  let unit: string | undefined;
  const after = normalised.slice(at);
  const glued = /^([^\s\d]+)(?=\s|$)/u.exec(after)?.[1];
  const spaced = /^ ([^\s\d]+)(?=\s|$)/u.exec(after)?.[1];
  const known = (word: string | undefined) =>
    word !== undefined &&
    (word === 'x' || word === '×' || rules.units.some((each) => each.toLocaleLowerCase() === word.toLocaleLowerCase()));
  if (known(glued)) {
    unit = glued;
    at += glued?.length ?? 0;
  } else if (glued === undefined && known(spaced)) {
    unit = spaced;
    at += 1 + (spaced?.length ?? 0);
  }

  // A space, then the title: a number glued to a word ("7up") or standing alone is a title.
  const rest = normalised.slice(at);
  if (!/^\s+\S/u.test(rest)) return { title: line };
  // Counted in code points: a digit outside the basic plane is one character in either spelling.
  const written = [...line];
  const title = written.slice(written.length - [...rest].length).join('').trim();
  const amount = unit === undefined ? number : unit === 'x' || unit === '×' ? `${number}×` : `${number} ${unit}`;
  return { title, amount };
}

/** What one submit of the capture field creates, and what stays in it. */
export function capture(text: string, rules: CaptureRules): Capture {
  const typed = text.split(/\r?\n/u);
  if (!rules.splitsLines) {
    const line = parseLine(typed.join(' ').replace(/\s+/gu, ' '), rules);
    return { lines: line.title === '' ? [] : [line], rest: '' };
  }
  const lines: CapturedLine[] = [];
  let index = 0;
  for (; index < typed.length && lines.length < MAX_CAPTURE_LINES; index += 1) {
    const line = parseLine(typed[index] ?? '', rules);
    if (line.title !== '') lines.push(line);
  }
  const rest = typed.slice(index).filter((line) => line.trim() !== '').join('\n');
  return { lines, rest };
}
