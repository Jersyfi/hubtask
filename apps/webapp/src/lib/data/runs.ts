// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

const RUNS = '/automation/runs';

/**
 * What the listing can be narrowed by: the three the contract had, and the window F8-02 added -
 * `from` inclusive and `to` exclusive on the run's own moment, as RFC 3339 instants.
 */
export interface RunFilter {
  readonly ruleId?: string;
  readonly status?: string;
  readonly trigger?: string;
  readonly from?: string;
  readonly to?: string;
}

/** The listing's path for a filter. Built here so the store and its callers agree on one string. */
export function runsPath(filter: RunFilter = {}, cursor?: string): string {
  const query = new URLSearchParams();
  if (filter.ruleId) query.set('rule_id', filter.ruleId);
  if (filter.status) query.set('status', filter.status);
  if (filter.trigger) query.set('trigger', filter.trigger);
  if (filter.from) query.set('from', filter.from);
  if (filter.to) query.set('to', filter.to);
  if (cursor) query.set('cursor', cursor);
  const written = query.toString();
  return written ? `${RUNS}?${written}` : RUNS;
}
