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

/** What a sign-in through a provider begins with, as `OidcStart` takes it. */
export interface StartRequest {
  readonly login_hint?: string;
  readonly provider_id?: string;
  readonly invitation_token?: string;
  readonly connect_token?: string;
}

/**
 * The body of `:start`, with only what was given in it.
 *
 * **The invitation travels when the sign-in begins on the invitation card** (ADR-0078 §1). It is the
 * second proof that lets the provider activate the invited account — the server binds it to the
 * flow without spending it — and a sign-in begun anywhere else carries none. It goes to this
 * installation only: the provider never sees it.
 *
 * **So does the connect link**, when the sign-in begins on the card that link opens (ADR-0078 §1):
 * where a workspace switched the password off, the link *Forgot your password?* mailed is the
 * account's proof at the provider's first arrival, together with a fresh sign-in there. One or the
 * other, never both.
 */
export function startRequest(
  loginHint?: string,
  providerId?: string,
  invitationToken?: string,
  connectToken?: string,
): StartRequest {
  const hint = loginHint?.trim();
  return {
    ...(hint ? { login_hint: hint } : {}),
    // The provider is named where a workspace has more than one (§ the sign-in rules); without a
    // name the server takes the only one, which is what every installation with one has.
    ...(providerId ? { provider_id: providerId } : {}),
    ...(invitationToken ? { invitation_token: invitationToken } : {}),
    ...(connectToken ? { connect_token: connectToken } : {}),
  };
}
