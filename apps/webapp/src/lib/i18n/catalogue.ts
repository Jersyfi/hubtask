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
