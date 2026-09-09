// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The two things a provider sign-in needs that are not requests: reading what came back in the
 * address, and deciding whether an authorization URL may be navigated to (H-04).
 *
 * Pure, and separate from `oidc.svelte.ts` for the reason `capability.ts` and `stepup.ts` are:
 * the interesting part is the decisions, and a module that reached for `location` itself could
 * only be tested by mounting the application.
 */

/** What the provider's redirect carried, as `OidcCallback` needs it. */
export interface Handoff {
  readonly code: string;
  readonly state: string;
}

/**
 * What a callback address amounts to.
 *
 * Three answers rather than two, because the provider has three things to say. `handoff` is the
 * ordinary one. `refused` is the person pressing "cancel" at the provider, or a provider declining
 * for its own reasons — an `error` parameter and no code, so there is nothing to exchange and
 * nothing to ask the server about. `nothing` is an address reached without a flow at all: a
 * bookmark, a reload after the exchange, somebody typing the path.
 */
export type Arrival =
  | { readonly kind: 'handoff'; readonly handoff: Handoff }
  | { readonly kind: 'refused' }
  | { readonly kind: 'nothing' };

/**
 * Reads the callback's query.
 *
 * **The provider's own `error_description` is deliberately not read.** It is a sentence written by
 * a third party, in a language nobody chose, and rendering it would put foreign text on this
 * screen — which is the same rule that keeps display text out of the backend (ADR-0011). The
 * refusal is said in the catalogue's words instead.
 *
 * A `state` with no `code` is not a handoff: there is nothing to exchange, and sending an empty
 * code would be asking the server to refuse something this client can already see is not there.
 */
export function readArrival(search: string): Arrival {
  const query = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search);
  const code = query.get('code') ?? '';
  const state = query.get('state') ?? '';
  if (code !== '' && state !== '') return { kind: 'handoff', handoff: { code, state } };
  if (query.has('error')) return { kind: 'refused' };
  return { kind: 'nothing' };
}

/**
 * The authorization URL, if it is one a browser may be sent to.
 *
 * The value comes from this installation's own API, so this is not distrust of the answer — it is
 * that `location.assign` is one of the few places where a string becomes executable: a
 * `javascript:` URL there runs in this origin, with this origin's storage, and the content
 * security policy does not govern a navigation (ADR-0028). Two schemes are enough for a redirect
 * to a provider, and anything else is refused rather than followed.
 */
export function navigableUrl(candidate: string): string | undefined {
  let parsed: URL;
  try {
    parsed = new URL(candidate);
  } catch {
    return undefined;
  }
  return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.href : undefined;
}
