// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the installation says about itself, read once and held.
 *
 * `/meta/capabilities` is the one thing the client configures itself from: item types and their
 * capability profiles, view layouts, the query fields F2's filter editor is built from, the
 * supported locales and their direction, the role matrix, the limits. Nothing here may be
 * hard-coded against it — a list somebody typed is a list that is wrong on the installation that
 * has one more (`apps/webapp/CLAUDE.md`).
 *
 * It is a module rather than a `resource()` in a component for one reason: *once*. The engine
 * already keeps one state per path and loads it only when idle, so a second component asking
 * would not fetch twice — but the manifest is also read at boot, before anything is mounted, and
 * what it answers changes the language of the first paint.
 *
 * It is unauthenticated (`security: []` in the contract), so it works before anybody signs in —
 * which is what makes it the right thing to read first. **Once is not the same as once per page**,
 * though, and issue 1020 is the difference: the *route* takes no credential, the *request* carries
 * whatever bearer this tab still holds, and a stale one is answered `401` rather than anonymously
 * (`presentation/rest/Auth.go`: a credential that was presented is always verified). That `401`
 * ends the session, `engine.reset()` clears this subscription with every other, and a `failed`
 * entry is never loaded again by `subscribe` — so the whole application went on running against a
 * manifest it does not have until somebody reloaded the page. And the read is scoped by the
 * caller (`GetCapabilities`: the installation's scope when anonymous, the actor's when not), so
 * even an anonymous read that *succeeds* is not the manifest that applies after a sign-in.
 *
 * Hence one `refresh`, which re-listens before it reads: every change of actor asks again, as that
 * actor, and there is no second method that reads into an entry nobody hears.
 */

import type { Capabilities, ResourceState } from '@hubtask/sync-engine';

import type { SupportedLocale } from '../i18n/locale.ts';
import { engine } from './engine.ts';

const PATH = '/meta/capabilities';

class Manifest {
  #state = $state<ResourceState<Capabilities>>({ status: 'idle' });
  /** The listener registered with the engine, so that a second `start` replaces the first. */
  #stop: (() => void) | undefined;

  get state(): ResourceState<Capabilities> {
    return this.#state;
  }

  /** Whether anything it says may be relied on. Everything else here answers from a guess. */
  get isRead(): boolean {
    return this.#state.status === 'ready';
  }

  /** The manifest itself, or `undefined` while it is being read or if it could not be. */
  get value(): Capabilities | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /**
   * The locales this installation has, in the shape the renderer resolves against.
   *
   * Empty until the manifest arrives, and empty is the honest answer rather than a default: a
   * client that assumed English until told otherwise would be asserting something about the
   * installation it has not read yet. `resolveLocale` falls back to the source language on its
   * own, which is the same outcome without the assertion.
   */
  get supportedLocales(): SupportedLocale[] {
    const declared = this.value?.supported_locales ?? [];
    // Tolerant of an older server that answered the entry without its metadata (the contract
    // typed `week_start` and added `decimal_separator` in 0.8.0): a locale that states no
    // direction is left to right, and the two other facts are simply absent.
    return declared
      .filter((entry): entry is (typeof declared)[number] => typeof entry?.locale === 'string')
      .map((entry) => ({
        locale: entry.locale,
        direction: entry.direction === 'rtl' ? 'rtl' : 'ltr',
        week_start: entry.week_start,
        decimal_separator: entry.decimal_separator,
      }));
  }

  /**
   * Starts the one read, and returns the function that stops listening.
   *
   * It holds the manifest and applies nothing. What the supported locales *do* - decide which
   * language the document speaks and which way it runs - happens in one place in the frame, which
   * is the only place that also knows the account's preference; a data module that set an
   * attribute of its own would be the second answer to "which locale".
   */
  start(): () => void {
    this.#stop?.();
    const stop = engine.subscribe<Capabilities>({ path: PATH }, (next) => {
      this.#state = next;
    });
    this.#stop = stop;
    return () => {
      stop();
      if (this.#stop === stop) this.#stop = undefined;
    };
  }

  /**
   * Reads it again, as whoever is signed in now. A retry after a failure, and every change of
   * actor.
   *
   * **The listener first**, and that is the whole of issue 1020's third defect: `engine.reset()`
   * clears every listener there is, so a refresh on its own would read the manifest into an entry
   * nobody hears and leave this module publishing the state it failed in — for the life of the
   * page. Subscribing to an entry the reset removed starts the read by itself, which is why the
   * read below is only for the entry that survived: the one a retry is about.
   */
  async refresh(): Promise<void> {
    this.start();
    if (this.#state.status !== 'loading') await engine.refresh<Capabilities>({ path: PATH });
  }
}

export const manifest = new Manifest();
