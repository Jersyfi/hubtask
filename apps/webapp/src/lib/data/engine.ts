// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The application's one engine, and the only place a Transport is constructed.
 *
 * The base URL is relative because the bundle and the API come from one origin (ADR-0028) — there
 * is no host here to configure, and a configurable one would be a way to point a client at a
 * server it was not served by.
 *
 * The bearer is read from the platform seam rather than held: it is refreshed behind the engine's
 * back, and a copy taken once is a copy that keeps working after a sign-out.
 * `lib/session.svelte.ts` is what puts a pair there and takes it away again.
 */
import { FetchTransport, SyncEngine } from '@hubtask/sync-engine';

import { platform } from '../platform/index.ts';

/** How long the exchange may take. Short: nothing on screen can proceed until it answers. */
const REFRESH_TIMEOUT_MS = 15_000;

/** The shape `POST /auth/sessions:refresh` answers, narrowed to what this file uses. */
interface SessionTokens {
  readonly access_token: string;
  readonly refresh_token: string;
}

/**
 * What to do when the server refuses the credential, registered rather than imported.
 *
 * The session decides (clear the pair, remember the path, ask again) and the session also *uses*
 * the engine, so importing it here would be a cycle. A late binding breaks it in the direction
 * that costs nothing: this module knows there is a handler, and not what it does.
 */
let onRefused: () => void = () => {};

export function whenCredentialRefused(handler: () => void): void {
  onRefused = handler;
}

/**
 * The transport, held rather than only handed over, because the exchange below is a request too.
 *
 * It goes through the transport rather than through the engine deliberately: the engine's own
 * calls are what get retried after an exchange, and an exchange made through them would be an
 * exchange that could trigger an exchange. Every `fetch` in this client still happens in exactly
 * one place — `FetchTransport` — which is the rule that matters (`packages/CLAUDE.md`).
 */
const transport = new FetchTransport({ baseUrl: '/api/v1' });

/**
 * Exchanges the refresh token for the next pair, and answers whether the session survived.
 *
 * The engine calls this at most once at a time and retries the refused request once after it
 * (F4-03). Nothing here decides what a failure means for the screen: `false` sends the engine to
 * `onUnauthorized`, which is where the session ends and the path is remembered.
 *
 * **No bearer on this call**, and that is the contract's shape rather than an omission: the refresh
 * token in the body is the whole credential, and demanding the very thing this call exists to
 * replace would make it useless exactly when it is needed.
 */
async function renew(): Promise<boolean> {
  const refresh = platform.refreshToken();
  if (refresh === undefined) return false;

  try {
    const answer = await transport.send<SessionTokens>(
      'POST',
      '/auth/sessions:refresh',
      { refresh_token: refresh },
      { timeoutMs: REFRESH_TIMEOUT_MS },
    );
    // Both halves, always. The exchange retires the token it was given, so keeping the old one
    // beside a new access token would leave the client holding a credential the server has
    // already treated as spent - and presenting it again is what it reads as theft.
    platform.holdSession({
      access: answer.body.access_token,
      refresh: answer.body.refresh_token,
    });
    return true;
  } catch {
    // Refused, unreachable, or malformed: all three mean the same thing to the caller, which is
    // that there is no credential to retry with. Which of the three it was is the server's to
    // record, not this client's to distinguish.
    return false;
  }
}

export const engine = new SyncEngine({
  transport,
  token: () => platform.bearer(),
  onUnauthorized: () => onRefused(),
  onRefresh: renew,
});
