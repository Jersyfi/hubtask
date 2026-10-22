// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The frame's passing messages, for any screen to raise: `toast.show({ text, tone, undo, action,
 * link })`. The rules are `toasts.ts`'s; this is the reactive half - one timer for the next expiry,
 * and the one announcement per message, through the application's announcer, so a reader hears it
 * once and the stack draws no live region of its own.
 *
 * The dwells are `--dur-dwell` and `--dur-dwell-offering` from tokens.json, read from the document
 * when they are needed, so the figures stay in one place (rule 15).
 */

import { announcer } from '../announce.svelte.ts';
import { ToastModel, millisecondsOf, type ToastEntry, type ToastSpec } from './toasts.ts';

/** The handle a caller keeps: to take its message away when what it reports is over. */
export interface ToastHandle {
  dismiss(): void;
}

class Toasts {
  #model = new ToastModel((isOffering) =>
    typeof document === 'undefined'
      ? undefined
      : millisecondsOf(getComputedStyle(document.documentElement).getPropertyValue(isOffering ? '--dur-dwell-offering' : '--dur-dwell')),
  );
  #entries = $state<readonly ToastEntry[]>([]);
  #timer: ReturnType<typeof setTimeout> | undefined;

  get entries(): readonly ToastEntry[] {
    return this.#entries;
  }

  show(spec: ToastSpec): ToastHandle {
    const id = this.#model.show(spec, Date.now());
    announcer.say(spec.text);
    this.#sync();
    return { dismiss: () => this.dismiss(id) };
  }

  hold(id: number): void {
    this.#model.hold(id);
    this.#sync();
  }

  release(id: number): void {
    this.#model.release(id, Date.now());
    this.#sync();
  }

  dismiss(id: number): void {
    this.#model.dismiss(id);
    this.#sync();
  }

  /** At sign-out, beside the replica's reset. */
  clear(): void {
    this.#model.clear();
    this.#sync();
  }

  /** Publishes the model and keeps exactly one timer, for whatever runs out next. */
  #sync(): void {
    this.#entries = this.#model.entries;
    if (this.#timer !== undefined) clearTimeout(this.#timer);
    this.#timer = undefined;
    const next = this.#model.nextExpiry();
    if (next === undefined) return;
    this.#timer = setTimeout(() => {
      this.#model.expire(Date.now());
      this.#sync();
    }, Math.max(0, next - Date.now()));
  }
}

export const toast = new Toasts();
