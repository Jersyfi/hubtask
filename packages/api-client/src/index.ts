// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The Hubtask API, as TypeScript types.
 *
 * Everything here is re-exported from `dist/schema.d.ts`, which `make api-client` generates from
 * `api/openapi.yaml`. This file adds no description of its own: the specification is the source
 * and a second description of it is a second thing to keep in step (ADR-0004).
 *
 * There is no runtime value here. The TypeScript client for third parties lives in
 * sdk/typescript, typed against its own generation of the same document (ADR-0057); the
 * first-party apps' fetch layer is the sync engine's Transport port (ADR-0033). Keeping
 * this package to generated types is what keeps it first-party and what keeps the SDK a thing
 * that can be taken.
 */

export type { components, operations, paths, webhooks } from '../dist/schema.js';
