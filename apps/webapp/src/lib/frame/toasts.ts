// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The frame's stack of passing messages, as plain logic: what is shown, what leaves when, and what
 * a newer message may and may not push out. The reactive half and the drawing are
 * `toasts.svelte.ts` and `ToastStack.svelte`; this half has no runes, so `node --test` reads it.
 *
 * **A message that still offers something is never pushed out by a newer one.** A confirmation
 * ("Saved") is replaced by the next confirmation - two of them saying the same kind of thing would
 * be noise. One that carries *Undo* or an action ("Label deleted · Undo") stays until it runs out,
 * is dismissed or is used: a second deletion must not take the first one's way back with it.
 *
 * **It stays long enough for what it asks.** A plain confirmation stays `--dur-dwell`; one that
 * offers something stays `--dur-dwell-offering`, which is at least eight seconds - long enough to
 * read, decide and reach *Undo* by keyboard. Both are tokens, handed in by the caller. And it does
 * not run out while somebody is with it (WCAG 2.2.1): while the pointer is over a message or focus
 * is in it, it is held, and its time starts again when they leave.
 *
 * **What it says is said once, by the application's announcer** (`announce.svelte.ts`); the
 * drawing has no live region of its own. `Escape` does not reach it: a message is not a layer that
 * owns the keyboard, and each one has its own *Dismiss*.
 */

/** The tones a message takes, as `Toast` draws them. */
export type ToastTone = 'info' | 'success' | 'warning' | 'danger';

/** What a caller asks to be shown. Every string is resolved text (ADR-0011). */
export interface ToastSpec {
  readonly text: string;
  readonly tone?: ToastTone;
  /** The way back from what just happened. Present means the message offers *Undo*. */
  readonly undo?: () => void | Promise<void>;
  /** One other thing to do about it. */
  readonly action?: { readonly label: string; readonly run: () => void | Promise<void> };
  /** Where to look at what happened. */
  readonly link?: { readonly label: string; readonly href: string };
  /**
   * Where focus goes if it is in the message when it leaves and the element it came from is gone:
   * the control that caused it, as a selector. Without one, the page's main landmark.
   */
  readonly returnTo?: string;
}

export interface ToastEntry extends ToastSpec {
  readonly id: number;
  /** When it runs out on its own, in the caller's clock; `undefined` while it is held. */
  readonly expiresAt: number | undefined;
}

/** Whether a message still offers something a newer one must not take away. */
export const isOffering = (entry: ToastSpec): boolean => entry.undo !== undefined || entry.action !== undefined;

/**
 * Why *Undo* is shown but cannot be used now, as a message code, or nothing where it can: it is a
 * write, and a device without a network cannot send one. Shown with its reason rather than hidden
 * (design-system.md §4, case 2), so the way back is still seen to exist.
 */
export function undoRefusal(isDeviceOffline: boolean): string | undefined {
  return isDeviceOffline ? 'app.offline.undo_waits' : undefined;
}

/**
 * How long a message stays, in milliseconds, read when it is needed: `isOffering` says which of
 * the two dwells. Nothing means no time - a message then stays until it is dismissed rather than
 * leaving at a moment made up here (rule 15).
 */
export type Dwell = (isOffering: boolean) => number | undefined;

export class ToastModel {
  #entries: ToastEntry[] = [];
  #next = 1;
  readonly #dwell: Dwell;

  constructor(dwell: Dwell) {
    this.#dwell = dwell;
  }

  get entries(): readonly ToastEntry[] {
    return this.#entries;
  }

  /** Shows a message and answers its id. A plain one replaces the plain ones before it. */
  show(spec: ToastSpec, now: number): number {
    const id = this.#next++;
    const kept = isOffering(spec) ? this.#entries : this.#entries.filter(isOffering);
    this.#entries = [...kept, { ...spec, id, expiresAt: this.#until(spec, now) }];
    return id;
  }

  /** The pointer or focus arrived: it does not run out while they stay. */
  hold(id: number): void {
    this.#entries = this.#entries.map((entry) => (entry.id === id ? { ...entry, expiresAt: undefined } : entry));
  }

  /** They left: its time starts again from now. */
  release(id: number, now: number): void {
    this.#entries = this.#entries.map((entry) =>
      entry.id === id && entry.expiresAt === undefined ? { ...entry, expiresAt: this.#until(entry, now) } : entry,
    );
  }

  /** Takes away what has run out; answers whether anything left. */
  expire(now: number): boolean {
    const before = this.#entries.length;
    this.#entries = this.#entries.filter((entry) => entry.expiresAt === undefined || entry.expiresAt > now);
    return this.#entries.length !== before;
  }

  dismiss(id: number): void {
    this.#entries = this.#entries.filter((entry) => entry.id !== id);
  }

  /** At sign-out: nothing of the previous account's - an *Undo* least of all - outlives it. */
  clear(): void {
    this.#entries = [];
  }

  /** The next moment something runs out, for the one timer the caller keeps; `undefined` if none. */
  nextExpiry(): number | undefined {
    const times = this.#entries.map((entry) => entry.expiresAt).filter((at): at is number => at !== undefined);
    return times.length === 0 ? undefined : Math.min(...times);
  }

  #until(spec: ToastSpec, now: number): number | undefined {
    const dwell = this.#dwell(isOffering(spec));
    return dwell === undefined ? undefined : now + dwell;
  }
}

/** A duration custom property as milliseconds - in either unit CSS writes - or nothing when unreadable. */
export function millisecondsOf(value: string): number | undefined {
  const trimmed = value.trim();
  const amount = Number.parseFloat(trimmed);
  if (Number.isNaN(amount)) return undefined;
  return trimmed.endsWith('ms') ? amount : amount * 1000;
}
