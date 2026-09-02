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
 * back, and a copy taken once is a copy that keeps working after a sign-out. F1-11 puts a real
 * token behind this; until then it answers `undefined` and every call is anonymous, which is the
 * honest state rather than a placeholder that pretends otherwise.
 */
import { FetchTransport, SyncEngine } from '@hubtask/sync-engine';

import { platform } from '../platform/index.ts';

export const engine = new SyncEngine({
  transport: new FetchTransport({ baseUrl: '/api/v1' }),
  token: () => platform.bearer(),
});
