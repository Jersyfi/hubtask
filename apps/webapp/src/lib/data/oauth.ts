// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What an authorization request carries, and whether this client may act on it.
 *
 * Pure, and separate from the screen for the reason `oidc.ts` is: the interesting part is what the
 * address means, and a module that reached for `location` itself could only be tested by mounting
 * the application.
 *
 * **Nothing here validates on the server's behalf.** Whether the redirect URI is one the app
 * registered, whether the challenge is well formed, whether the scopes exist — all of that is the
 * server's answer, and a client that pre-empted it would be a second implementation of a rule that
 * moves. What this decides is only whether there is a request here at all: the four fields the
 * contract makes mandatory, present and non-empty.
 */

/** An authorization request, as the app put it in the address. */
export interface AuthorizationRequest {
  readonly clientId: string;
  readonly redirectUri: string;
  readonly scopes: readonly string[];
  readonly challenge: string;
  readonly method: string;
  /** Echoed back to the app untouched, for its own CSRF binding. Absent where the app sent none. */
  readonly state?: string;
}

/**
 * Reads the request out of a query string.
 *
 * `undefined` means there is no request here — a bookmark, a reload after the redirect, somebody
 * typing the path. That is a different thing from a request the server refuses, and the screen
 * says so differently.
 *
 * The scopes arrive space-separated, which is what RFC 6749 says and what an app will send.
 */
export function readRequest(search: string): AuthorizationRequest | undefined {
  const query = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search);
  const clientId = query.get('client_id') ?? '';
  const redirectUri = query.get('redirect_uri') ?? '';
  const challenge = query.get('code_challenge') ?? '';
  const scopes = (query.get('scope') ?? '').split(/[\s+]+/).filter((one) => one !== '');
  if (clientId === '' || redirectUri === '' || challenge === '' || scopes.length === 0) {
    return undefined;
  }
  return {
    clientId,
    redirectUri,
    scopes,
    challenge,
    // Absent means the app sent none, and the server refuses anything but S256 — so the value
    // travels as it arrived rather than being defaulted into looking valid.
    method: query.get('code_challenge_method') ?? '',
    state: query.get('state') ?? undefined,
  };
}

/**
 * Where the browser goes once the person has decided.
 *
 * **Built from the redirect URI the server validated**, never from anything this client composed:
 * the URI is echoed by the server's own answer path, and the code and state are appended as query
 * parameters, which is what an OAuth client expects to read.
 *
 * `undefined` where the URI is not a navigation. The server has already matched it byte for byte
 * against what the app registered, so this is not distrust of the answer — it is that
 * `location.assign` is one of the few places a string becomes executable in this origin.
 */
export function completionUrl(
  redirectUri: string,
  code: string,
  state: string | undefined,
): string | undefined {
  let url: URL;
  try {
    url = new URL(redirectUri);
  } catch {
    return undefined;
  }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return undefined;
  url.searchParams.set('code', code);
  if (state !== undefined) url.searchParams.set('state', state);
  return url.href;
}

/**
 * Where the browser goes when the person declines.
 *
 * RFC 6749 §4.1.2.1: `access_denied`, with the state echoed. Telling the app is better than
 * leaving it waiting — and it is the app's own error vocabulary rather than a sentence this client
 * would invent for it.
 */
export function declineUrl(redirectUri: string, state: string | undefined): string | undefined {
  let url: URL;
  try {
    url = new URL(redirectUri);
  } catch {
    return undefined;
  }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return undefined;
  url.searchParams.set('error', 'access_denied');
  if (state !== undefined) url.searchParams.set('state', state);
  return url.href;
}
