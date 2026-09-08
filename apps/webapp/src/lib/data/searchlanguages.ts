// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which languages a search asks after the reader's own has found nothing.
 *
 * **ADR-0034 stands; this softens what it costs.** An entry is indexed under the language it was
 * written in and `POST /search` reads the *query* under the caller's language — so a workspace
 * written in English and read in a German browser answers "nothing matches" until somebody changes
 * a control they have no reason to look at. R-08 step 8 found that and named F3's search work as
 * where to decide. The decision is here: the client asks again rather than the server guessing, and
 * the index keeps meaning one thing.
 *
 * **Only after a silence, and only when there is something else to ask.** A search that found
 * something under the reader's own language asks no second question — the first answer is the one
 * they wanted, and widening it would put worse matches under better ones.
 *
 * **The picker still wins.** Somebody who chose a language asked a precise question, and answering
 * a wider one would be ignoring it.
 */

/** The reader's language as a search would read it: the base tag, so `de-DE` matches `de`. */
export function baseTagOf(tag: string | undefined): string | undefined {
  const trimmed = tag?.trim();
  if (!trimmed) return undefined;
  return trimmed.split('-')[0]?.toLowerCase();
}

/**
 * The languages to try, in the order the installation reports them.
 *
 * The reader's own is left out — it is what the first search already asked — and a tag that only
 * differs by region is the same language to an index, so `de-DE` in the reader's account excludes
 * `de` from the list.
 */
export function widenTo(
  readerLocale: string | undefined,
  textLanguages: readonly string[],
): readonly string[] {
  const own = baseTagOf(readerLocale);
  const seen = new Set<string>();
  const wider: string[] = [];

  for (const tag of textLanguages) {
    const base = baseTagOf(tag);
    if (!base || base === own || seen.has(base)) continue;
    seen.add(base);
    wider.push(tag);
  }
  return wider;
}

/**
 * Whether to ask the other languages at all.
 *
 * Three conditions, and each is a decision: nothing was found, the reader did not choose a language
 * themselves, and there is at least one other language to ask. A widening that ran when any of the
 * three were false would be a client asking questions nobody wanted answered.
 */
export function shouldWiden(options: {
  found: number;
  chosenLanguage: string | undefined;
  wider: readonly string[];
}): boolean {
  return options.found === 0 && !options.chosenLanguage && options.wider.length > 0;
}
