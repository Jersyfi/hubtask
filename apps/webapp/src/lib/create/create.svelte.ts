// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether *Erstellen* is open, and the kinds this person created last on this device (§6.12).
 *
 * The last kinds are a convenience of the device, like the recently opened places: kept in
 * `localStorage` per account, read in a `try`, and nothing is lost when it is empty.
 *
 * **A small kind's dialog lives on its own screen** (goal, time): *Erstellen* goes there and
 * leaves the wish here; the screen takes it once and opens its dialog (C-3).
 */

import { noteLast, type CreateKind } from './kinds.ts';

const KEY = 'hubtask.create.last.';

class Create {
  #isOpen = $state(false);
  #isDirect = $state(false);
  #last = $state<readonly CreateKind[]>([]);
  #account: string | undefined;
  #wish = $state<CreateKind | undefined>(undefined);

  get isOpen(): boolean {
    return this.#isOpen;
  }

  get last(): readonly CreateKind[] {
    return this.#last;
  }

  /** Reads the last kinds of the account signed in. */
  adopt(accountId: string | undefined): void {
    this.#account = accountId;
    try {
      const written = accountId ? localStorage.getItem(KEY + accountId) : null;
      const parsed: unknown = written ? JSON.parse(written) : [];
      this.#last = Array.isArray(parsed) ? (parsed.filter((each) => typeof each === 'string') as CreateKind[]) : [];
    } catch {
      this.#last = [];
    }
  }

  /** C or Q, the bar's button, the phone's ＋: the key that opened it closes it. */
  summon(): void {
    this.#isOpen = !this.#isOpen;
  }

  /**
   * The ＋ held (C-5): the kind made last is made at once where it may be made here; *Erstellen*
   * opens on it and the menu takes it as soon as the place says it is allowed.
   */
  summonLast(): void {
    if (this.#last.length === 0) return this.summon();
    this.#isDirect = true;
    this.#isOpen = true;
  }

  /** The menu takes the held ＋ once. */
  takeDirect(): boolean {
    const was = this.#isDirect;
    this.#isDirect = false;
    return was;
  }

  get isDirect(): boolean {
    return this.#isDirect;
  }

  close(): void {
    this.#isDirect = false;
    this.#isOpen = false;
  }

  note(kind: CreateKind): void {
    this.#last = noteLast(this.#last, kind);
    try {
      if (this.#account) localStorage.setItem(KEY + this.#account, JSON.stringify(this.#last));
    } catch {
      // A browser that keeps nothing still creates; it only does not remember the kind.
    }
  }

  /** Leaves the wish for the screen whose dialog makes this kind. */
  wish(kind: CreateKind): void {
    this.#wish = kind;
  }

  /** The screen takes its wish once. */
  take(kind: CreateKind): boolean {
    if (this.#wish !== kind) return false;
    this.#wish = undefined;
    return true;
  }

  get wished(): CreateKind | undefined {
    return this.#wish;
  }
}

export const create = new Create();
