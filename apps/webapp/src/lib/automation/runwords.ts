// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** A rule run in words (page-system §7.6): its outcome, what started it, and why it stopped. */

import type { Run } from '../data/runs.svelte.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';

/** A run that says it is running and started this long ago is a crash, not progress. */
const STALE_AFTER_MS = 15 * 60 * 1000;

/** A `RUNNING` row whose start is long past is a process that died, not one still working. */
export const isStale = (run: Pick<Run, 'status' | 'started_at'>, now: number = Date.now()): boolean =>
  run.status === 'RUNNING' && now - new Date(run.started_at).getTime() > STALE_AFTER_MS;

export const statusWord = (status: string): string =>
  messages.has(`app.runs.status_${status.toLowerCase()}`) ? t(`app.runs.status_${status.toLowerCase()}`) : status;

export const triggerWord = (trigger: string): string =>
  messages.has(`app.rules.trigger_${trigger.toLowerCase()}`) ? t(`app.rules.trigger_${trigger.toLowerCase()}`) : trigger;

/** Which condition stopped a skipped run - the first that did not hold. */
export function stoppedBy(run: Pick<Run, 'condition_results'>): number | undefined {
  return run.condition_results.find((result) => !result.matched)?.index;
}
