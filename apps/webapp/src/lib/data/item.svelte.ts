// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One entry, read by its own address, and its history.
 *
 * The entry itself is a plain `resource()` in the view. Its **history** is here because it pages,
 * and because what a page of it means needs saying: `GET /items/{id}/activity` is newest first and
 * walks by cursor, which is what `LoadMore` is for — one press, one page, and what was already
 * read stays on the screen.
 *
 * **A container has no history**, and this module could not read one if a screen asked: the entity
 * is keyed on `itemId` and `/items/{id}/activity` is the only reader the contract declares. What a
 * hub or a collection changed lives in the audit trail, which is a different read with a different
 * permission (`audit.md` §1) and F4's work.
 */

/** The path a history is read from. One place, because a paged path spelled twice is two walks. */
export function activityPath(itemId: string): string {
  return `/items/${itemId}/activity?page_size=25`;
}

export function itemPath(itemId: string): string {
  return `/items/${itemId}`;
}
