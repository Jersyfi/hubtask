// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The catalogue, read where it lives.
 *
 * `locales/en.json` is the one source (`i18n-l10n.md` §3) and it sits at the repository root
 * because the Go binary embeds it — `locales/Embed.go` exists for exactly the same reason this
 * import climbs five directories: a catalogue that is copied is a catalogue that is forgotten, and
 * the two halves of the product would then disagree about what a code says.
 *
 * That is this task's decision, out of the three it was offered. A **build-time import** rather
 * than a package that owns the read, because there is one consumer today and a package is a
 * manifest, a CI matrix entry, a filter and a dependency edge for a file with no logic in it;
 * rather than a Vite alias, because an alias is the same path written in two configuration files
 * that can drift. The path is ugly on purpose, and it appears exactly once: everything else in the
 * client imports the catalogue from here.
 *
 * When a second client renders codes — the website does not — this module is what becomes
 * `packages/i18n`, and nothing that imports it has to change.
 */

import english from '../../../../../locales/en.json' with { type: 'json' };

/** One locale's messages, by code. */
export type Catalogue = Readonly<Record<string, string>>;

/** The language the catalogue is written in, and the end of every fallback chain (§2, §3). */
export const SOURCE_LOCALE = 'en';

/**
 * A key that is not a message. The catalogue documents itself in `_comment`, and a renderer that
 * offered that as a message would be offering the reader a note to the translators. The Go side
 * skips the same prefix, in `infrastructure/i18n`.
 */
const METADATA_PREFIX = '_';

function withoutMetadata(entries: Readonly<Record<string, string>>): Catalogue {
  const messages: Record<string, string> = {};
  for (const [code, message] of Object.entries(entries)) {
    if (!code.startsWith(METADATA_PREFIX)) messages[code] = message;
  }
  return messages;
}

/** The source catalogue: `locales/en.json`, minus its notes to translators. */
export const SOURCE: Catalogue = withoutMetadata(english);

/**
 * The other catalogues, lazily (F5-07, `milestone-F5.md` decision 5).
 *
 * `import.meta.glob` over the same directory: one chunk per file, none of them in the initial
 * bundle, loaded when the resolved locale is not the source. Not a `fetch` of `/locales/<tag>.json`
 * - the server serves no such route and `connect-src` names none - and not a second copy under
 * `apps/`, which `i18n-l10n.md` §6 line 1 forbids by name. The path is the same ugly climb as
 * the source's, and it appears exactly twice in this file.
 *
 * Outside Vite - the tests, the residue render - `import.meta.glob` does not exist and the call
 * throws, and then there are no chunks: the source renders, which is what happens for a missing
 * file too. The call is made rather than tested for, because Vite rewrites the *call* at build
 * time and knows nothing of a `typeof` beside it.
 */
function chunksOf(): Record<string, () => Promise<unknown>> {
  try {
    return import.meta.glob('../../../../../locales/*.json', { import: 'default' });
  } catch {
    return {};
  }
}
const chunks = chunksOf();

/**
 * The chunk for a locale. The glob's keys are the paths as Vite spells them - relative in a
 * build, absolute under the dev server for a file outside the project root - so the match is on
 * the tail every spelling shares, `/locales/<tag>.json`, rather than on one spelling.
 */
function chunkFor(locale: string): (() => Promise<unknown>) | undefined {
  const tail = `/locales/${locale}.json`;
  for (const [key, load] of Object.entries(chunks)) {
    if (key.endsWith(tail)) return load;
  }
  return undefined;
}

/** Whether this build carries a catalogue for the locale, without loading it. */
export function hasCatalogue(locale: string): boolean {
  return locale === SOURCE_LOCALE || chunkFor(locale) !== undefined;
}

/**
 * The catalogue for a locale, loaded on first use, or nothing where this build carries none.
 *
 * Nothing rather than a refusal: a locale the manifest lists and no file exists for is the
 * source rendering, exactly as an untranslated code does (§3). The caller says so once, in
 * development, and nowhere else.
 */
export async function loadCatalogue(locale: string): Promise<Catalogue | undefined> {
  if (locale === SOURCE_LOCALE) return SOURCE;
  const load = chunkFor(locale);
  if (!load) return undefined;
  const entries = (await load()) as Readonly<Record<string, string>>;
  return withoutMetadata(entries);
}

/**
 * The pattern for a code, along the chain: the reader's locale first, then the source language.
 *
 * §3's fallback in one line, and the part that matters is what it does *not* do. A half-translated
 * file is the normal state of a translation rather than an error, so a code the German catalogue
 * has not reached yet renders the English sentence — never the key, and never nothing.
 */
export function patternFor(code: string, catalogues: readonly Catalogue[]): string | undefined {
  for (const catalogue of catalogues) {
    const pattern = catalogue[code];
    if (pattern !== undefined) return pattern;
  }
  return undefined;
}
