// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The administration column's figures that its screens' own reads already answer (the navigation
 * concept §2.6: a figure on the danger surface only where something asks for action). Pure, so
 * each has a test without the frame; `AppFrame` holds the reads.
 */

type Translate = (code: string, params?: Record<string, string>) => string;

/** No backup target at all: nothing is being kept anywhere. Unknown while the list is read. */
export function backupColumnFigure(targets: readonly unknown[] | undefined, t: Translate): string | undefined {
  return targets !== undefined && targets.length === 0 ? t('app.admin.figure_no_target') : undefined;
}

/** The failed runs among the latest page the runs screen reads. */
export function runsColumnFigure(runs: readonly { readonly status: string }[], t: Translate): string | undefined {
  const failed = runs.filter((run) => run.status === 'FAILED').length;
  return failed > 0 ? t('app.admin.figure_failed', { count: String(failed) }) : undefined;
}

/** The privacy requests at or past their statutory deadline (`standingOf`'s *overdue* and *soon*). */
export function privacyColumnFigure(standings: readonly (string | undefined)[], t: Translate): string | undefined {
  const owed = standings.filter((standing) => standing === 'overdue' || standing === 'soon').length;
  return owed > 0 ? t('app.admin.figure_owed', { count: String(owed) }) : undefined;
}
