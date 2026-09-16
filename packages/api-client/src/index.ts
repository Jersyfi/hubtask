// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The Hubtask API, as TypeScript types.
 *
 * Everything here is re-exported from `dist/schema.d.ts`, which `make api-client` generates from
 * `api/openapi.yaml`. This file adds no description of its own: the specification is the source
 * and a second description of it is a second thing to keep in step (ADR-0004).
 *
 * The one runtime value is the generated client in `client.gen.ts` (tools/sdkgen, P-03): a
 * class over `fetch` for a third party, typed against the same generated `operations`. The
 * first-party apps do not use it - their fetch layer is the sync engine's Transport port
 * (ADR-0033) - and keeping this package to generated output means the extraction ADR-0027 defers
 * to before 1.0.0, and ADR-0057 puts to the owner, stays a move rather than a rewrite.
 */

export type { components, operations, paths, webhooks } from '../dist/schema.js';
export { HubtaskClient, ProblemError, type CallOptions, type ClientOptions, type Problem } from './client.gen.js';
