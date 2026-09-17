// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The application's one set of messages, and the only reactive thing in this directory.
 *
 * Everything under it is pure and tested in plain Node (`format`, `catalogue`, `locale`,
 * `messages`); this holds the current answer and re-renders what reads it when that answer
 * changes. The shape is `theme.ts`'s: one module owns the document attribute, and a component
 * never sets it.
 *
 * It starts in the source language with what the browser asks for, because at boot that is
 * genuinely all the client knows. The account preference (F1-08) and the installation's supported
 * locales (`/meta/capabilities`, F1-10) arrive later and are handed to `adopt`.
 */

import { SOURCE, SOURCE_LOCALE, hasCatalogue, loadCatalogue, type Catalogue } from './catalogue.ts';
import type { MessageParams } from './format.ts';
import {
  applyDocumentLocale,
  directionOf,
  resolveLocale,
  type LocalePreferences,
  type LocaleTarget,
  type SupportedLocale,
} from './locale.ts';
import { createMessages } from './messages.ts';

/**
 * What this build carries. One locale, which is the honest state: `locales/` holds `en.json` and
 * nothing else, so a client that offered a list would be offering languages it cannot render.
 * `/meta/capabilities` replaces it with what the installation actually has.
 */
const BUNDLED: readonly SupportedLocale[] = [{ locale: SOURCE_LOCALE, direction: 'ltr' }];

class ActiveMessages {
  #locale = $state(SOURCE_LOCALE);
  #direction = $state<'ltr' | 'rtl'>('ltr');
  #catalogues = $state<readonly Catalogue[]>([SOURCE]);
  #messages = $derived(createMessages({ locale: this.#locale, catalogues: this.#catalogues }));

  get locale(): string {
    return this.#locale;
  }

  get direction(): 'ltr' | 'rtl' {
    return this.#direction;
  }

  /** The sentence for a code. This is what a component calls, through the `t` below. */
  t(code: string, params?: MessageParams): string {
    return this.#messages.t(code, params);
  }

  /** Whether a code has a message — for a caller choosing between `code` and `detail_code`. */
  has(code: string): boolean {
    return this.#messages.has(code);
  }

  /** The locales this build has said nothing about yet, so that a missing file is said once. */
  readonly #noticed = new Set<string>();

  /**
   * Take the locale the preferences and the manifest resolve to, and say so on the document.
   *
   * The catalogue follows (F5-07): the resolved locale's chunk is loaded when it is not the
   * source and laid over the source in §3's chain when it arrives, the source rendering meanwhile
   * rather than nothing. A load that lands after the locale moved on is dropped - the reader
   * changed their mind faster than the network, and the later choice wins.
   */
  adopt(
    preferences: LocalePreferences,
    supported: readonly SupportedLocale[] = BUNDLED,
    root: LocaleTarget = document.documentElement,
  ): string {
    const locale = resolveLocale(preferences, supported);
    this.#locale = locale;
    this.#direction = directionOf(locale, supported);
    applyDocumentLocale(root, locale, this.#direction);
    void this.#follow(locale);
    return locale;
  }

  async #follow(locale: string): Promise<void> {
    if (locale === SOURCE_LOCALE) {
      this.#catalogues = [SOURCE];
      return;
    }
    if (!hasCatalogue(locale)) {
      // The manifest lists a locale this build has no file for: the source renders, and in
      // development the fact is said once so that a missing translation is not mistaken for a
      // rendering bug.
      if (import.meta.env?.DEV && !this.#noticed.has(locale)) {
        this.#noticed.add(locale);
        console.info(`no catalogue for ${locale} in this build; rendering the source language`);
      }
      this.#catalogues = [SOURCE];
      return;
    }
    const catalogue = await loadCatalogue(locale);
    if (this.#locale !== locale) return;
    this.#catalogues = catalogue ? [catalogue, SOURCE] : [SOURCE];
  }
}

export const messages = new ActiveMessages();

/** What a component imports. Short because it is written more often than anything else here. */
export function t(code: string, params?: MessageParams): string {
  return messages.t(code, params);
}

/**
 * The boot call, beside `followSystemTheme()`: before the first paint the document has to say what
 * language it is in and which way it runs, and the browser's own preference is the only thing the
 * client knows at that point.
 */
export function startLocale(): string {
  return messages.adopt({ requested: navigator.languages });
}
