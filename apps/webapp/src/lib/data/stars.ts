// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A person's stars as one ordered list (the navigation concept §6.2, §7.1): starring appends,
 * starring again removes, and the whole list is written at once. Pure, so the order is tested
 * without a server.
 */

import type { Star } from '@hubtask/sync-engine';

/** The most stars a person keeps; the server refuses more (`stars.too_many`). */
export const MAX_STARS = 100;

export const isStarred = (stars: readonly Star[], kind: Star['kind'], id: string): boolean =>
  stars.some((star) => star.kind === kind && star.id === id);

/** The list with the place starred at its end, or taken out where it was starred; nothing past the bound. */
export function toggled(stars: readonly Star[], kind: Star['kind'], id: string): readonly Star[] | undefined {
  if (isStarred(stars, kind, id)) return stars.filter((star) => !(star.kind === kind && star.id === id));
  return stars.length >= MAX_STARS ? undefined : [...stars, { kind, id }];
}
