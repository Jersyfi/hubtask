// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which languages a search asks after the reader's own has found nothing.
 *
 * **ADR-0034 stands; this softens what it costs.** An entry is indexed under the language it was
 * written in and `POST /search` reads the *query* under the caller's language — so a workspace
 * written in English and read in a German browser answers "nothing matches" until somebody changes
 * a control they have no reason to look at. R-08 step 8 found that and named F3's search work as
 * where to decide. The decision is here: the client asks again rather than the server guessing, so
 * the index keeps meaning one thing.
 *
 * **Two stages, and the split is what a measurement forced.** `text_languages` is what this
 * installation's PostgreSQL was *built* with — thirty tags on an ordinary build — rather than what
 * the workspace is written in. Two designs were tried against a running server and both were
 * rejected:
 *
 * * asking all of them automatically is twenty-nine round trips every time somebody mistypes a
 *   word;
 * * asking the first few is a coin flip, because the list arrives alphabetically and means nothing:
 *   on the installation walked, English sat at position **5 of 29** — inside a cap of five by luck,
 *   and outside it the moment an installation indexes one more language beginning with a, b, c or d.
 *
 * So: **the languages the reader themselves reads are asked automatically**, because a hit in one
 * of those is a hit they can act on and there are rarely more than two; and **the rest are offered
 * rather than spent**, on one control that appears exactly at the silence and says what it will do.
 * That is not the picker R-08 complained about — it makes no choice and needs no knowledge; it is
 * there at the moment it is useful and gone otherwise.
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

/** The tags an installation indexes, by base tag, so a reader's `de-DE` finds its `de`. */
function indexedBy(textLanguages: readonly string[]): Map<string | undefined, string> {
  const found = new Map<string | undefined, string>();
  for (const tag of textLanguages) {
    const base = baseTagOf(tag);
    if (base && !found.has(base)) found.set(base, tag);
  }
  return found;
}

/**
 * The languages asked without being asked for: the ones the reader's own device says they read,
 * minus the one the first search already used, and only where this installation indexes them.
 *
 * A language somebody's browser lists and the installation cannot index is not in the list, because
 * it cannot be searched at all — offering to would be offering nothing.
 */
export function readerLanguages(
  readerLocale: string | undefined,
  textLanguages: readonly string[],
  preferred: readonly string[],
): readonly string[] {
  const own = baseTagOf(readerLocale);
  const indexed = indexedBy(textLanguages);
  const seen = new Set<string>();
  const wider: string[] = [];

  for (const tag of preferred) {
    const base = baseTagOf(tag);
    if (!base || base === own || seen.has(base)) continue;
    const matched = indexed.get(base);
    if (!matched) continue;
    seen.add(base);
    wider.push(matched);
  }
  return wider;
}

/**
 * Everything else this installation indexes, in the order it reports them.
 *
 * What the offered search asks. The order is meaningless — it is whatever the database was built
 * with — which is exactly why this set is offered rather than sampled: a subset of a meaningless
 * order is a guess, and the whole of it is an answer.
 */
export function remainingLanguages(
  readerLocale: string | undefined,
  textLanguages: readonly string[],
  already: readonly string[],
): readonly string[] {
  const own = baseTagOf(readerLocale);
  const asked = new Set(already.map((tag) => baseTagOf(tag)));
  const seen = new Set<string>();
  const rest: string[] = [];

  for (const tag of textLanguages) {
    const base = baseTagOf(tag);
    if (!base || base === own || asked.has(base) || seen.has(base)) continue;
    seen.add(base);
    rest.push(tag);
  }
  return rest;
}

/**
 * Whether to ask the reader's own other languages at all.
 *
 * Three conditions, and each is a decision: nothing was found, the reader did not choose a language
 * themselves, and there is at least one other language to ask.
 */
export function shouldWiden(options: {
  found: number;
  chosenLanguage: string | undefined;
  wider: readonly string[];
}): boolean {
  return options.found === 0 && !options.chosenLanguage && options.wider.length > 0;
}

/** Whether to offer the rest: still nothing, nobody chose, and there is a rest to offer. */
export function canOfferRest(options: {
  found: number;
  chosenLanguage: string | undefined;
  remaining: readonly string[];
}): boolean {
  return options.found === 0 && !options.chosenLanguage && options.remaining.length > 0;
}
