// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a board's columns mean at a collection's level (build-plan §5, domain-model.md §3.5a): which
 * column a carried card may enter, and which statuses fall in *Ohne Spalte*. The server decides every
 * drop (`:drop`); this only reads ahead of it so a column that would refuse is dimmed, with the
 * server's own reason code, while the card is carried.
 */

import type { BoardPreview, EntrySteps, StateMode } from '@hubtask/sync-engine';

/** A level switch a person asks for: L0 → L1, L1 → L0, L1 → L2. */
export type LevelSwitch = 'board' | 'list' | 'workflow';

/** The key a board uses for the column of entries no column shows. */
export const NO_COLUMN = 'none';

export interface ColumnShape {
  readonly id: string;
  /** Under a workflow, the statuses the column shows; empty without one. */
  readonly statuses?: readonly string[];
}

/** The statuses of a workflow no column shows: *Ohne Spalte* (folded, never hidden). */
export function unmappedStatuses(columns: readonly ColumnShape[], all: readonly string[]): string[] {
  const shown = new Set(columns.flatMap((column) => column.statuses ?? []));
  return all.filter((status) => !shown.has(status));
}

/**
 * The refusal code per column key a carried card would meet, where it would meet one.
 *
 * At L1 the entries stand in a column, so *no column* refuses (`board.column_required`). At L2 a
 * column showing no status refuses (`board.column_without_status`); otherwise a column is open when
 * one of its statuses is the card's own or a step the reader may take reaches it, refused as
 * `board.step_blocked` when only a blocked step does, and `board.no_step` when none. Until the
 * card's steps are read, nothing is dimmed - the server still answers the drop.
 */
export function columnRefusals(
  level: StateMode | undefined,
  columns: readonly ColumnShape[],
  unmapped: readonly string[],
  steps: Pick<EntrySteps, 'status' | 'steps' | 'blocked'> | undefined,
  from: string,
): Map<string, string> {
  const refusals = new Map<string, string>();
  if (level === 'COLUMNS') {
    if (from !== NO_COLUMN) refusals.set(NO_COLUMN, 'board.column_required');
    return refusals;
  }
  if (level !== 'WORKFLOW') return refusals;
  const reach = new Set((steps?.steps ?? []).filter((step) => step.allowed).map((step) => step.to));
  if (steps?.status) reach.add(steps.status);
  const blocked = new Set([
    ...(steps?.blocked ?? []).map((step) => step.to),
    ...(steps?.steps ?? []).filter((step) => !step.allowed).map((step) => step.to),
  ]);
  const judge = (key: string, statuses: readonly string[]) => {
    if (key === from) return;
    if (statuses.length === 0) {
      refusals.set(key, key === NO_COLUMN ? 'board.no_step' : 'board.column_without_status');
      return;
    }
    if (!steps) return;
    if (statuses.some((status) => reach.has(status))) return;
    refusals.set(key, statuses.some((status) => blocked.has(status)) ? 'board.step_blocked' : 'board.no_step');
  };
  for (const column of columns) judge(column.id, column.statuses ?? []);
  judge(NO_COLUMN, unmapped);
  return refusals;
}

/** A status standing in no column, in a scheme preview's mapping. */
export const UNMAPPED = '';

/** Where each status stands on a previewed board: its column, or `UNMAPPED`. */
export function statusPlaces(board: Pick<BoardPreview, 'columns' | 'unmapped'>): Record<string, string> {
  const places: Record<string, string> = {};
  for (const column of board.columns) for (const status of column.statuses) places[status] = column.bucket_id;
  for (const each of board.unmapped) places[each.status] = UNMAPPED;
  return places;
}

/** The `columns` a scheme change sends for one board: each column with the statuses placed in it. */
export function mappedColumns(board: Pick<BoardPreview, 'columns'>, places: Readonly<Record<string, string>>): { bucket_id: string; statuses: string[] }[] {
  return board.columns.map((column) => ({
    bucket_id: column.bucket_id,
    statuses: Object.entries(places).filter(([, at]) => at === column.bucket_id).map(([status]) => status),
  }));
}
