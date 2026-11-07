// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The application's one set of messages, and the only reactive thing in this directory.
 *
 * Everything under it is pure and tested in plain Node (`format`, `catalogue`, `locale`,
 * `messages`); this holds the current answer and re-renders what reads it when that answer
 * changes. The shape is `theme.ts`'s: one module owns the document attribute, and a component
 * never sets it.
 *
 * It starts with what the browser asks for, because at boot that is genuinely all the client
 * knows, and `ready` is when that locale's catalogue has arrived. The account preference and the installation's supported
 * locales (`/meta/capabilities`) arrive later and are handed to `adopt`.
 */

import { SOURCE_LOCALE, bundledLocales, hasCatalogue, loadCatalogue, type Catalogue } from './catalogue.ts';
import type { MessageParams } from './format.ts';
import {
  applyDocumentLocale,
  directionOf,
  platformDirection,
  resolveLocale,
  type LocalePreferences,
  type LocaleTarget,
  type SupportedLocale,
} from './locale.ts';
import { createMessages } from './messages.ts';

/**
 * What this build carries, until `/meta/capabilities` says what the installation has: so that a
 * reader whose browser asks for German loads the German catalogue at boot rather than the English
 * one first. The direction is the platform's guess; the manifest's answer replaces it.
 */
const BUNDLED: readonly SupportedLocale[] = bundledLocales().map((locale) => ({ locale, direction: platformDirection(locale) }));

class ActiveMessages {
  #locale = $state(SOURCE_LOCALE);
  #direction = $state<'ltr' | 'rtl'>('ltr');
  // Raw: a catalogue is replaced whole, never changed in place, and a deep proxy of it would be a
  // second copy of a megabyte of strings.
  #catalogues = $state.raw<readonly Catalogue[]>([]);
  #messages = $derived(
    createMessages({ locale: this.#locale, catalogues: this.#catalogues, onMissing: () => this.#wantSource() }),
  );
  // The source, once loaded; and the load in flight, so that a thousand misses fetch it once.
  #source: Catalogue | undefined;
  #sourceLoad: Promise<Catalogue | undefined> | undefined;
  #ready: Promise<void> = Promise.resolve();

  /** Settles when the catalogue of the locale last adopted has arrived, or failed to. */
  get ready(): Promise<void> {
    return this.#ready;
  }

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
   * The catalogue follows: the resolved locale's chunk is loaded and replaces the chain when it
   * arrives, the previous chain rendering meanwhile. The source is laid under a translation only
   * once it is loaded - on the first code the translation lacks, not up front. A load that lands
   * after the locale moved on is dropped - the reader changed their mind faster than the network,
   * and the later choice wins.
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
    this.#ready = this.#follow(locale);
    return locale;
  }

  async #follow(locale: string): Promise<void> {
    if (locale === SOURCE_LOCALE || !hasCatalogue(locale)) {
      // The manifest lists a locale this build has no file for: the source renders, and in
      // development the fact is said once so that a missing translation is not mistaken for a
      // rendering bug.
      if (locale !== SOURCE_LOCALE && import.meta.env?.DEV && !this.#noticed.has(locale)) {
        this.#noticed.add(locale);
        console.info(`no catalogue for ${locale} in this build; rendering the source language`);
      }
      const source = await this.#loadSource();
      if (this.#locale === locale && source) this.#catalogues = [source];
      return;
    }
    let catalogue: Catalogue | undefined;
    try {
      catalogue = await loadCatalogue(locale);
    } catch {
      // A chunk that does not arrive leaves the chain as it was; the source is the next answer.
      catalogue = undefined;
    }
    if (this.#locale !== locale) return;
    if (!catalogue) {
      const source = await this.#loadSource();
      if (this.#locale === locale && source) this.#catalogues = [source];
      return;
    }
    this.#catalogues = this.#source ? [catalogue, this.#source] : [catalogue];
  }

  /** The source catalogue, loaded once; a failed load is tried again on the next call. */
  #loadSource(): Promise<Catalogue | undefined> {
    if (this.#source) return Promise.resolve(this.#source);
    this.#sourceLoad ??= loadCatalogue(SOURCE_LOCALE).then(
      (source) => {
        this.#source = source;
        return source;
      },
      () => undefined,
    ).finally(() => {
      this.#sourceLoad = undefined;
    });
    return this.#sourceLoad;
  }

  /**
   * A code the chain does not know: the source is fetched and laid under the chain. Called while
   * rendering, so the state changes only after the load, never synchronously.
   */
  #wantSource(): void {
    if (this.#source || this.#sourceLoad) return;
    void this.#loadSource().then((source) => {
      if (source && !this.#catalogues.includes(source)) this.#catalogues = [...this.#catalogues, source];
    });
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
