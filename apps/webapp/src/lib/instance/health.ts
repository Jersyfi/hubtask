// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's health report as the overview says it (user-experience.md §8.4): each
 * degraded feature by name with the way to the screen that changes it, the schema as one state,
 * and the backlogs as tiles that warn where a snapshot can tell.
 */

import type { HealthReport } from '@hubtask/sync-engine';

/** The features the server's probes name (`Impact` of each probe); another is drawn by its key. */
export const KNOWN_FEATURES = new Set([
  'all', 'media', 'notifications', 'event_bus', 'ai_suggestions', 'semantic_search',
]);

/**
 * Where the installation changes what a degraded feature depends on, where it has a screen. Mail,
 * storage, the bus and the database are set where the process runs, so they carry no link.
 */
const FEATURE_PAGES: Readonly<Record<string, string>> = {
  ai_suggestions: '/instance/ai',
  semantic_search: '/instance/ai',
};

/** The message code that names a feature, where this build has one; otherwise its key is drawn. */
export const featureNameCode = (feature: string): string | undefined =>
  KNOWN_FEATURES.has(feature) ? `app.instance.feature.${feature}` : undefined;

export const instancePageOfFeature = (feature: string): string | undefined => FEATURE_PAGES[feature];

export type SchemaState = 'up_to_date' | 'waiting' | 'newer' | 'too_old';

/** The report's `migration.status` as the *Upgrade* row says it; absent when it was not answered. */
export function schemaState(report: HealthReport | undefined): SchemaState | undefined {
  switch (report?.migration?.status) {
    case 'ok': return 'up_to_date';
    case 'behind': return 'waiting';
    case 'ahead': return 'newer';
    case 'too_old': return 'too_old';
    default: return undefined;
  }
}

export interface BacklogTile {
  readonly id: 'dead_letters' | 'outbox_lag' | 'job_queue' | 'webhook_retries';
  readonly value: number;
  /** Above the line its alert draws, where one snapshot can tell (A-05, A-07). */
  readonly isAbove: boolean;
  /** The screen that acts on it; absent where no screen does. */
  readonly href?: string;
}

/**
 * The backlogs as tiles, absent until the server has counted once.
 *
 * Only two warn: a dead letter waits for a person whatever the count (A-07 fires on any new one),
 * and the outbox is late past A-05's minute. A-06 and A-09 fire on a trend over a window, which a
 * single count cannot show, so those two tiles never claim a threshold they cannot see.
 */
export function backlogTiles(report: HealthReport | undefined): BacklogTile[] {
  const backlogs = report?.backlogs;
  if (!backlogs) return [];
  const tiles: BacklogTile[] = [];
  const add = (id: BacklogTile['id'], value: number | undefined, isAbove: boolean, href?: string) => {
    if (typeof value === 'number') tiles.push({ id, value, isAbove, ...(href ? { href } : {}) });
  };
  add('dead_letters', backlogs.dead_letter_total, (backlogs.dead_letter_total ?? 0) > 0, '/instance/dead-letters');
  add('outbox_lag', backlogs.outbox_lag_seconds, (backlogs.outbox_lag_seconds ?? 0) > OUTBOX_LATE_SECONDS);
  add('job_queue', backlogs.job_queue_depth, false);
  add('webhook_retries', backlogs.webhook_retry_backlog, false);
  return tiles;
}

/** A-05's line: the outbox is late once its oldest event has waited longer than a minute. */
export const OUTBOX_LATE_SECONDS = 60;
