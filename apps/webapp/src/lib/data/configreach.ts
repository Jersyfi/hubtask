// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a configuration plan reaches beyond the configuration (configuration.md §2), as the
// lines a person reads before applying it: every count the server answered, none invented. A
// reach the reader may not see is said once and counts nothing.

import type { ConfigPlan } from '@hubtask/sync-engine';

export type ConfigReach = NonNullable<ConfigPlan['reach']>[number];

/** One line of a plan's reach: a message code and its parameters. */
export interface ReachLine {
  code: string;
  params: Record<string, string | number>;
}

/** The lines a plan's reach reads as, in the order of its key paths. */
export function reachLines(reach: readonly ConfigReach[] | null | undefined): ReachLine[] {
  const lines: ReachLine[] = [];
  for (const each of reach ?? []) {
    const path = each.path;
    if (each.withheld) {
      lines.push({ code: 'app.changes.reach_withheld', params: { path } });
      continue;
    }
    if (each.entries !== undefined) {
      lines.push({ code: 'app.changes.reach_entries', params: { path, count: each.entries } });
    }
    if (each.hidden_entries !== undefined) {
      lines.push({ code: 'app.changes.reach_hidden', params: { path, entries: each.hidden_entries, values: each.hidden_values ?? 0 } });
    } else if (each.hidden_values !== undefined) {
      lines.push({ code: 'app.changes.reach_values', params: { path, count: each.hidden_values } });
    }
    if (each.accounts !== undefined) {
      lines.push({
        code: 'app.changes.reach_people',
        params: { path, accounts: each.accounts, groups: each.groups ?? 0, gained: each.gained?.length ?? 0, lost: each.lost?.length ?? 0 },
      });
    }
    if (each.people !== undefined) {
      lines.push({ code: 'app.changes.reach_module', params: { path, count: each.people } });
    }
  }
  return lines;
}
