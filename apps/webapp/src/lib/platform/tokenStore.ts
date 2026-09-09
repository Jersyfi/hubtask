// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where the browser keeps the session, and the reasoning that decides it.
 *
 * **A pair, not a token.** `POST /auth/sessions` answers an access token of fifteen minutes and a
 * refresh token of thirty days (H-01, security.md §5). The access token is the bearer on every
 * call; the refresh token is presented once, at `/auth/sessions:refresh`, and is retired by that
 * very call.
 *
 * **`sessionStorage`, not `localStorage` and not memory.** F1-11 requires that a valid session
 * survives a reload, which rules memory out; and it requires no more than that, which rules
 * `localStorage` out - a credential that outlives the tab is a credential still sitting on a
 * shared machine tomorrow morning.
 *
 * **The browser session is as long as the tab, and the refresh token's thirty days are
 * deliberately not used here.** What the refresh token buys this client is not longevity but
 * *rotation*: the access token it keeps alive lives fifteen minutes, so a copy stolen out of this
 * origin is worth a quarter of an hour rather than a month. Persisting the pair past the tab would
 * trade that away for the convenience of not signing in again, and F1-11's answer to that trade
 * has not changed.
 *
 * **What F1 predicted, and what actually arrived.** This file used to say the real answer was "a
 * session the browser holds and script cannot read, which is the OIDC connection in `0.6.0`". The
 * contract mints no such thing: `/auth/oidc:callback` answers the same bearer pair a password
 * sign-in does, and every route takes a bearer. So there is no cookie session to move to, and this
 * file is what `0.6.0` produced rather than what it replaced. What protects the pair is what
 * protected the token: ADR-0028's content security policy - no `'unsafe-inline'`, no
 * `'unsafe-eval'`, `connect-src 'self'` - and the fact that the bundle is served from the same
 * origin as the API.
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

/**
 * The session as this client holds it: the bearer, and the credential that renews it.
 *
 * The expiry moments the server answers are deliberately not kept. The client does not schedule a
 * refresh against a clock it does not control - it refreshes when a request is refused, which is
 * the only moment that is certainly right whatever the two clocks disagree about.
 */
export interface SessionPair {
  readonly access: string;
  readonly refresh: string;
}

export interface TokenStore {
  /** The bearer, or `undefined` when nobody is signed in. */
  read(): string | undefined;
  /** The credential the exchange presents, or `undefined` when there is none to present. */
  readRefresh(): string | undefined;
  write(pair: SessionPair): void;
  clear(): void;
}

/**
 * The keys. Prefixed because the origin is shared with anything else served from it, and named
 * for what they are rather than for what they hold.
 */
export const TOKEN_KEY = 'hubtask.bearer';
export const REFRESH_KEY = 'hubtask.refresh';

/**
 * A store over the storage given, or over nothing.
 *
 * `undefined` storage is the honest case rather than an error: the application then holds the pair
 * in memory for as long as the page is open, which is a worse experience and a working one.
 */
export function tokenStore(storage: TokenStorage | undefined): TokenStore {
  let access: string | undefined;
  let refresh: string | undefined;

  const load = (key: string, held: string | undefined): string | undefined => {
    if (held !== undefined) return held;
    let value: string | undefined;
    try {
      value = storage?.getItem(key) ?? undefined;
    } catch {
      // A storage that refuses to be read is a storage that holds nothing, for our purposes.
      value = undefined;
    }
    // An empty string is not a credential; it is a cleared one that was written badly.
    return value === '' ? undefined : value;
  };

  const put = (key: string, value: string): void => {
    try {
      storage?.setItem(key, value);
    } catch {
      // Held in memory instead. The session lasts until the page is closed or reloaded, which
      // is what a browser that refuses storage is asking for anyway.
    }
  };

  return {
    read(): string | undefined {
      access = load(TOKEN_KEY, access);
      return access;
    },

    readRefresh(): string | undefined {
      refresh = load(REFRESH_KEY, refresh);
      return refresh;
    },

    write(pair: SessionPair): void {
      access = pair.access;
      refresh = pair.refresh;
      put(TOKEN_KEY, pair.access);
      put(REFRESH_KEY, pair.refresh);
    },

    clear(): void {
      access = undefined;
      refresh = undefined;
      for (const key of [TOKEN_KEY, REFRESH_KEY]) {
        try {
          storage?.removeItem(key);
        } catch {
          // Nothing left to do: the copy this module could reach is gone.
        }
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
