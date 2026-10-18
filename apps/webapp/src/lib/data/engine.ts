// SPDX-License-Identifier: Apache-2.0
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
import { FetchTransport, SyncEngine, TransportError } from '@hubtask/sync-engine';

import { platform } from '../platform/index.ts';
import { mutationFor, storeFor } from './replica.ts';

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
 * one place — `FetchTransport` — which is the rule that matters (`project-structure.md` §2.1).
 */
/**
 * What to do when the server answers that the workspace is paused - suspended or pending deletion -
 * registered rather than imported, for the reason `onRefused` is.
 */
let onPaused: (cause: TransportError) => void = () => {};

export function whenWorkspacePaused(handler: (cause: TransportError) => void): void {
  onPaused = handler;
}

/** The refusals a paused workspace answers every request with (multi-tenancy.md §5). */
const PAUSED = new Set(['access.tenant_suspended', 'access.tenant_pending_deletion']);

export function isPaused(cause: unknown): cause is TransportError {
  return cause instanceof TransportError && cause.status === 403 && PAUSED.has(cause.detailCode ?? '');
}

/**
 * The transport, watched for the paused refusal on every call it makes. A `403` is not a refused
 * credential, so the engine keeps the session through it - and a person whose workspace was paused
 * under them would meet the same refusal on every screen. Each call's answer passes through as it
 * came; the refusal is only reported on the way.
 */
function watchedForPause<T extends object>(inner: T): T {
  return new Proxy(inner, {
    get(target, key, receiver) {
      const value: unknown = Reflect.get(target, key, receiver);
      if (typeof value !== 'function') return value;
      return (...args: unknown[]) => {
        const answer: unknown = value.apply(target, args);
        if (!(answer instanceof Promise)) return answer;
        return answer.catch((cause: unknown) => {
          if (isPaused(cause)) onPaused(cause);
          throw cause;
        });
      };
    },
  });
}

const transport = watchedForPause(new FetchTransport({ baseUrl: '/api/v1' }));

/**
 * Exchanges the refresh token for the next pair, and answers whether the session survived.
 *
 * The engine calls this at most once at a time and retries the refused request once after it.
 * Nothing here decides what a failure means for the screen: `false` sends the engine to
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
    // The same session's next pair, so what belongs to the session - its hour at the installation -
    // stays with it.
    platform.renewSession({
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
  // What the replica answers while the server cannot be reached: this application's
  // paths, mapped in `replica.ts` the way `live.ts` maps a record - the engine learns neither.
  storeFor,
  // What a write becomes when it has to be queued: the seven mutation kinds, mapped from
  // this application's writes in the same file.
  mutationFor,
});
