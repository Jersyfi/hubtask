// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which language the client speaks, and which way the document runs.
 *
 * `i18n-l10n.md` §2 gives the order — request, account, tenant, installation — with one
 * parenthesis that inverts the top of it: `Accept-Language` counts "only for anonymous/client
 * responses". So for somebody signed in, the account preference wins over the browser's own
 * setting, and the browser is what answers before there is an account to ask (F1-08 makes that
 * readable, F1-10 reads it).
 *
 * Both functions here are pure. The document is touched in one place, `applyDocumentLocale`, the
 * way `theme.ts` touches it in one place — for the same reason: an attribute set from two modules
 * is an attribute nobody owns.
 */

import { SOURCE_LOCALE } from './catalogue.ts';

/** One entry of `/meta/capabilities`' `supported_locales`. Direction is the manifest's answer. */
export interface SupportedLocale {
  readonly locale: string;
  readonly direction: 'ltr' | 'rtl';
  readonly week_start?: string;
}

/** Where a locale can come from, most specific first in the sense §2 means. */
export interface LocalePreferences {
  /** `account.locale`, once the client knows who it is. */
  readonly account?: string;
  /** What the browser asks for. `navigator.languages`, in its own order of preference. */
  readonly requested?: readonly string[];
  /** `tenant.default_locale`. */
  readonly tenant?: string;
  /** `HUBTASK_DEFAULT_LOCALE`, as the manifest reports it. */
  readonly installation?: string;
}

const canonical = (tag: string) => tag.trim().toLowerCase();

/**
 * A BCP 47 tag and everything it may fall back to: `de-AT` → `de-at`, `de`. The source language is
 * not in the chain — it is the last candidate rather than a suffix of every one of them, because
 * `de-AT` falling straight to `en` while `de` exists would be the wrong answer.
 */
export function fallbackChain(tag: string): string[] {
  const chain: string[] = [];
  let current = canonical(tag);
  while (current !== '') {
    chain.push(current);
    const cut = current.lastIndexOf('-');
    if (cut < 0) break;
    current = current.slice(0, cut);
  }
  return chain;
}

/**
 * The locale to speak, out of the ones this installation has.
 *
 * Answers rather than failing: an installation that supports nothing the reader asked for still
 * has to render something, and §3 says that something is the source language.
 */
export function resolveLocale(
  preferences: LocalePreferences,
  supported: readonly SupportedLocale[],
): string {
  const available = new Map(supported.map((entry) => [canonical(entry.locale), entry.locale]));

  const candidates = [
    preferences.account,
    ...(preferences.requested ?? []),
    preferences.tenant,
    preferences.installation,
  ].filter((tag): tag is string => typeof tag === 'string' && tag.trim() !== '');

  for (const candidate of candidates) {
    for (const step of fallbackChain(candidate)) {
      const match = available.get(step);
      if (match !== undefined) return match;
    }
  }
  return available.get(SOURCE_LOCALE) ?? SOURCE_LOCALE;
}

/**
 * Which way the document runs, from the manifest rather than from a list written here. A client
 * that carried its own table of right-to-left languages would be wrong about the installation that
 * added one — `/meta/capabilities` reports the direction per supported locale for that reason.
 */
export function directionOf(locale: string, supported: readonly SupportedLocale[]): 'ltr' | 'rtl' {
  const wanted = canonical(locale);
  for (const step of fallbackChain(wanted)) {
    const entry = supported.find((candidate) => canonical(candidate.locale) === step);
    if (entry) return entry.direction;
  }
  return 'ltr';
}

/** Everything of the document this module touches, so that a test needs no DOM (as in `theme.ts`). */
export interface LocaleTarget {
  setAttribute(name: string, value: string): void;
}

/**
 * `lang` and `dir` on the root element, together, because they answer to the same fact. `lang` is
 * not decoration: it is what a screen reader picks a voice from and what hyphenation and quotation
 * marks follow, and a document that says `en` while showing Arabic is read aloud as gibberish.
 */
export function applyDocumentLocale(root: LocaleTarget, locale: string, direction: 'ltr' | 'rtl'): void {
  root.setAttribute('lang', locale);
  root.setAttribute('dir', direction);
}
