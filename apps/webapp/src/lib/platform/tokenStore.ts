// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where the browser keeps the bearer, and the reasoning that decides it.
 *
 * **`sessionStorage`, not `localStorage` and not memory.** F1-11 requires that a valid token
 * survives a reload, which rules memory out; and it requires no more than that, which rules
 * `localStorage` out - a credential that outlives the tab is a credential still sitting on a
 * shared machine tomorrow morning. `sessionStorage` is exactly the promise `platform/index.ts`
 * already made: the browser holds it for the tab's lifetime.
 *
 * **It is not a hiding place, and this file does not pretend otherwise.** Anything that can run
 * script in this origin can read it. What actually protects it is ADR-0028's content security
 * policy - no `'unsafe-inline'`, no `'unsafe-eval'`, `connect-src 'self'`, no foreign origin - and
 * the fact that this bundle is served from the same origin as the API. The real answer is a
 * session the browser holds and script cannot read, which is the OIDC connection in `0.6.0`; this
 * module is what that milestone replaces.
 *
 * Every access is guarded. A browser can refuse storage entirely - private mode, a policy, a
 * cleared origin - and a client that threw on that would be a client that cannot sign in at all
 * where it could at least sign in for the page.
 */

/** The little of `Storage` this needs, so that a test can hand it something else. */
export interface TokenStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

export interface TokenStore {
  read(): string | undefined;
  write(token: string): void;
  clear(): void;
}

/**
 * The key. Prefixed because the origin is shared with anything else served from it, and named for
 * what it is rather than for what it holds - a key called `token` invites a second one.
 */
export const TOKEN_KEY = 'hubtask.bearer';

/**
 * A store over the storage given, or over nothing.
 *
 * `undefined` storage is the honest case rather than an error: the application then holds the
 * token in memory for as long as the page is open, which is a worse experience and a working one.
 */
export function tokenStore(storage: TokenStorage | undefined): TokenStore {
  let held: string | undefined;

  return {
    read(): string | undefined {
      if (held !== undefined) return held;
      try {
        held = storage?.getItem(TOKEN_KEY) ?? undefined;
      } catch {
        // A storage that refuses to be read is a storage that holds nothing, for our purposes.
        held = undefined;
      }
      // An empty string is not a credential; it is a cleared one that was written badly.
      return held === '' ? undefined : held;
    },

    write(token: string): void {
      held = token;
      try {
        storage?.setItem(TOKEN_KEY, token);
      } catch {
        // Held in memory instead. The session lasts until the page is closed or reloaded, which
        // is what a browser that refuses storage is asking for anyway.
      }
    },

    clear(): void {
      held = undefined;
      try {
        storage?.removeItem(TOKEN_KEY);
      } catch {
        // Nothing left to do: the copy this module could reach is gone.
      }
    },
  };
}

/**
 * `window.sessionStorage`, or nothing where it cannot be reached. Touching the property is what
 * throws under a blocking policy, so the guard is around the access rather than around the use.
 */
export function browserStorage(): TokenStorage | undefined {
  try {
    return globalThis.sessionStorage;
  } catch {
    return undefined;
  }
}
