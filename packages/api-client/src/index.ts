// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The Hubtask API, as TypeScript types.
 *
 * Everything here is re-exported from `dist/schema.d.ts`, which `make api-client` generates from
 * `api/openapi.yaml`. This file adds no description of its own: the specification is the source
 * and a second description of it is a second thing to keep in step (ADR-0004).
 *
 * There is deliberately no runtime client yet. The fetch layer belongs to the sync engine's
 * Transport port (ADR-0033) and arrives with that work package; keeping this package to generated
 * types means the extraction ADR-0027 defers to before 1.0.0 stays a move rather than a rewrite.
 */

export type { components, operations, paths, webhooks } from '../dist/schema.js';
