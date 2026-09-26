// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which way in this browser was used last, so a person finds it first.
 *
 * **It belongs to the device, not to the account** — the same category as the theme and reduced
 * motion (ADR-0043): it is true of this browser rather than of the person, and it has to be
 * readable *before* anybody signs in, which is precisely when the account's copy does not exist.
 * So `localStorage`, beside those two, and not the replica — which is the account's and is
 * discarded at sign-out (`offline-sync.md` §9.6).
 *
 * **The address is not remembered.** It is personal data, it is what a shared machine would hand
 * to the next person, and it saves one field of typing. The *method* is neither: a provider's
 * identifier says nothing about who used it.
 *
 * A value that is no longer offered is ignored rather than cleared: a provider switched off for a
 * week and back on should still be the one this browser remembers.
 */

const KEY = 'hubtask.signin.method';

/** `localStorage` where it can be reached; nothing where it cannot (a sandbox, a test). */
function storage(): Storage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

/** The identifier of the provider used last, or nothing - which means the password. */
export function readLastProvider(): string | undefined {
  try {
    return storage()?.getItem(KEY) ?? undefined;
  } catch {
    return undefined;
  }
}

export function rememberProvider(id: string): void {
  try {
    storage()?.setItem(KEY, id);
  } catch {
    // A browser with storage blocked keeps working and forgets; nothing here is worth an error.
  }
}

/** Forgets it. What signing out with "this is not my device" would call. */
export function forgetProvider(): void {
  try {
    storage()?.removeItem(KEY);
  } catch {
    // As above.
  }
}

/**
 * The providers in the order to draw them: the one this browser used last, then the rest in the
 * order the server gave them.
 *
 * Pure, so the ordering has a test without a browser - and stable, because a list that reordered
 * itself on every render would move a button out from under somebody's finger.
 */
export function ordered<T extends { readonly id: string }>(
  providers: readonly T[],
  lastUsed: string | undefined,
): T[] {
  if (lastUsed === undefined) return [...providers];
  const first = providers.filter((provider) => provider.id === lastUsed);
  return [...first, ...providers.filter((provider) => provider.id !== lastUsed)];
}
