// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The handle a search is known by in the address, and where its words are kept (issue 997).
 *
 * **The address carries the narrowing, never the words** (ADR-0063 decision 4, as corrected).
 * `POST /search` has no `GET` so that a term never becomes a query string, and a client that
 * reflected it would undo the reason. But a reload must not lose what somebody typed, and that is
 * what this pays for without the address: the address carries a short, meaningless handle, and the
 * words live under it in `sessionStorage`, where the bearer already lives.
 *
 * **Minted, never derived.** A handle computed from the term - a hash, an encoding - would be an
 * oracle for it: anyone holding the link could confirm a guess by computing the same handle. So it
 * comes from the random source and says nothing about what it names.
 *
 * `sessionStorage` rather than `localStorage`, for what a search is: the thing somebody is doing
 * now. It survives a reload and the back button and dies with the tab, exactly as the credential
 * does. Sign-out clears it anyway, because a shared browser is a real thing.
 *
 * Pure, and tested beside itself: the browser-shaped half is three lines in `search.svelte.ts`.
 */

/** Where one handle's words are kept. Prefixed, so sign-out can find every one of them. */
const PREFIX = 'hubtask.search.';

/** What the address calls it. Short: it is in a URL a person may look at. */
export const HANDLE = 's';

/** How long a handle is, in hex characters. 8 is 32 bits — enough that two tabs do not collide. */
const LENGTH = 8;

export function keyFor(handle: string): string {
  return PREFIX + handle;
}

/**
 * A fresh handle.
 *
 * `crypto.getRandomValues` rather than `Math.random`, not because this is a secret — it names
 * nothing on its own — but because the one source of randomness in a client that has one is the
 * one that cannot surprise anybody later. It takes the *filling* rather than the source, because
 * `getRandomValues` is generic in the browser and in node in two ways that do not line up — and a
 * cast to reconcile them would be a cast in the one place that has to be beyond doubt.
 */
export function mint(fill: (bytes: Uint8Array<ArrayBuffer>) => void): string {
  const bytes = new Uint8Array(LENGTH / 2);
  fill(bytes);
  return [...bytes].map((byte) => byte.toString(16).padStart(2, '0')).join('');
}

/** Whether the address is naming a handle this client would have written. */
export function isHandle(value: string | undefined): value is string {
  return typeof value === 'string' && new RegExp(`^[0-9a-f]{${LENGTH}}$`).test(value);
}

/** Every key this module has ever written in this tab, for the one caller that clears them all. */
export function keysIn(storage: Pick<Storage, 'length' | 'key'>): readonly string[] {
  const keys: string[] = [];
  for (let index = 0; index < storage.length; index += 1) {
    const key = storage.key(index);
    if (key?.startsWith(PREFIX)) keys.push(key);
  }
  return keys;
}
