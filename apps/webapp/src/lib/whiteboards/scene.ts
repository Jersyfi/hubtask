// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether a board's scene is the one the server already holds. The live document draws a scene
 * with its fields in its own order and without the defaults the server writes back (`locked:
 * false`, an empty text, a `null` colour), so a plain comparison sees a change where there is none -
 * and opening a board saved it again, gave it a new `updated_at` and re-read the hub's contents.
 */

/**
 * One text per scene: the elements in their order, each element's fields sorted, a field that holds
 * only its default (`null`, `false`, an empty text) left out.
 */
export function sceneKey(elements: readonly object[]): string {
  return JSON.stringify(elements, (_, value: unknown) => {
    if (value === null || typeof value !== 'object' || Array.isArray(value)) return value;
    const fields = value as Record<string, unknown>;
    const kept: Record<string, unknown> = {};
    for (const name of Object.keys(fields).sort()) {
      const field = fields[name];
      if (field === null || field === undefined || field === false || field === '') continue;
      kept[name] = field;
    }
    return kept;
  });
}
