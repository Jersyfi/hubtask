// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How well a row's label answers what was typed into the palette (UC-WRK-27 check 2).
 *
 * Three grades and no fourth, because each is one a reader can predict: the label starts with the
 * words, a word of the label starts with them, or the words are somewhere inside it. A subsequence
 * match ("kch" for "Kitchen") was the alternative and was left out on purpose: it ranks "kitch"
 * under "Knit chairs" as readily as under "Kitchen", and the weights that would separate them are a
 * tuning nobody can check against a use case.
 *
 * Case and accents are folded in the reader's language, so "buro" finds "Büro" and a Turkish
 * reader's "i" folds as Turkish does. Pure, and tested beside itself.
 */

/** What a row scores: nothing, inside, at a word's start, at the label's start. */
export type Score = 0 | 1 | 2 | 3;

/**
 * Lower case in the reader's language, then with combining marks taken off. In that order: the
 * Turkish capital dotted I lower-cases to a plain i only while its dot is still there.
 */
export function fold(text: string, locale?: string): string {
  return text
    .toLocaleLowerCase(locale)
    .normalize('NFD')
    .replace(/\p{M}+/gu, '')
    .trim();
}

/** Where a word may start: the label's start, or after anything that is not a letter or digit. */
function startsAWord(folded: string, at: number): boolean {
  if (at === 0) return true;
  return !/[\p{L}\p{N}]/u.test(folded[at - 1] ?? '');
}

/**
 * The grade of `label` for `words`. Empty words match nothing: an empty palette lists what is
 * offered here in its own order, it does not rank it.
 */
export function score(label: string, words: string, locale?: string): Score {
  const wanted = fold(words, locale).replace(/\s+/g, ' ');
  if (wanted === '') return 0;
  const folded = fold(label, locale).replace(/\s+/g, ' ');
  if (folded.startsWith(wanted)) return 3;
  let at = folded.indexOf(wanted);
  let found: Score = 0;
  while (at >= 0) {
    if (startsAWord(folded, at)) return 2;
    found = 1;
    at = folded.indexOf(wanted, at + 1);
  }
  return found;
}
