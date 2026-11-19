// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether the session bar, centred, stands level with the top islands (W-5): only where it clears
 * the wider of the two by `room` (the islands' inset from the edge) on either side; else it stands
 * just below them.
 */
export function isLevelWithIslands(stage: number, bar: number, islands: readonly number[], room: number): boolean {
  if (bar <= 0) return true;
  return (stage - bar) / 2 >= Math.max(0, ...islands) + 2 * room;
}
