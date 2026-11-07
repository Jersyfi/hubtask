// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a configuration plan reaches beyond the configuration (configuration.md §2), as the
// lines a person reads before applying it: every count the server answered, none invented. A
// reach the reader may not see is said once and counts nothing.

import type { ConfigPlan } from '@hubtask/sync-engine';

import { sectionWord } from './configtemplates.ts';

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
    if (each.rules) {
      lines.push({ code: 'app.changes.reach_rules', params: { path, count: each.rules } });
    }
    for (const door of each.intake ?? []) {
      const fields = door.fields.join(', ');
      const channel = door.channel.toLowerCase();
      lines.push(
        door.doors === undefined
          ? { code: `app.changes.reach_intake_${channel}`, params: { path, fields } }
          : { code: `app.changes.reach_intake_${channel}_counted`, params: { path, fields, count: door.doors } },
      );
    }
  }
  return lines;
}

/**
 * The block a reach is about, for its sentence instead of the key path: the section's word and the
 * block's key, or the product's word for a module (user-experience.md §5.4). A path of the section
 * alone names the section.
 */
export function reachPlace(path: string): { section: string; key?: string; module?: string } {
  const [section = '', key] = path.split(/[./]/).filter(Boolean);
  if (section === 'modules' && key) return { section: sectionWord(section), module: `app.modules.name_${key.toLowerCase()}` };
  return key ? { section: sectionWord(section), key } : { section: sectionWord(section) };
}
