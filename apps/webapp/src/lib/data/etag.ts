// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The entity tag for a version, spelled once.
 *
 * The client holds pages, which carry no tag; every document it can write to carries a `version`,
 * and the server forms its `ETag` from exactly that — `etag(version)` in the controllers — and
 * reads it back by trimming the quotes. One place, because a second spelling of the same tag is a
 * precondition that never matches, and a precondition that never matches is an optimistic lock
 * that silently does nothing.
 *
 * It left `containers.svelte.ts` when entries began writing against a version too. Nothing about
 * it was ever about a container.
 */
export function etagFor(version: number): string {
  return `"${version}"`;
}
