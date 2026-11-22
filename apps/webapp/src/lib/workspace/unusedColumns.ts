// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Columns a collection keeps without offering a board (UC-WRK-05 check 13): they still set its
 * entries' state - an entry in the done column is done -, but nobody sees them as a board. The
 * settings say so and offer to remove them, through the same `board:switch` as *Back to the list*.
 */

import type { StateMode } from '@hubtask/sync-engine';

export interface ColumnsAtPlace {
  readonly isCollection: boolean;
  /** Whether the reader holds STRUCTURE here; removing the columns is a change of shape. */
  readonly mayShape: boolean;
  /** The tasks' level as the collection's read answers it; undefined until it is read. */
  readonly level: StateMode | undefined;
  readonly columns: number;
  /** The views the collection offers, as saved - not a draft being edited. */
  readonly offered: readonly string[];
}

/**
 * How many columns the hint names, or undefined where it is not drawn: not on a hub, not for a reader
 * who may not shape the collection, not where a board is offered, and not where a workflow runs for
 * the tasks - the switch is refused there (`board.workflow_runs`).
 */
export function unusedColumns(place: ColumnsAtPlace): number | undefined {
  if (!place.isCollection || !place.mayShape) return undefined;
  if (place.level !== 'COLUMNS' || place.columns === 0) return undefined;
  if (place.offered.includes('KANBAN')) return undefined;
  return place.columns;
}
