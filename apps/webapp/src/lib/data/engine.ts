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
 * back, and a copy taken once is a copy that keeps working after a sign-out. `lib/session.svelte.ts`
 * is what puts a token there and takes it away again.
 */
import { FetchTransport, SyncEngine } from '@hubtask/sync-engine';

import { platform } from '../platform/index.ts';

/**
 * What to do when the server refuses the credential, registered rather than imported.
 *
 * The session decides (clear the token, remember the path, ask again) and the session also *uses*
 * the engine, so importing it here would be a cycle. A late binding breaks it in the direction
 * that costs nothing: this module knows there is a handler, and not what it does.
 */
let onRefused: () => void = () => {};

export function whenCredentialRefused(handler: () => void): void {
  onRefused = handler;
}

export const engine = new SyncEngine({
  transport: new FetchTransport({ baseUrl: '/api/v1' }),
  token: () => platform.bearer(),
  onUnauthorized: () => onRefused(),
});
