// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A block's draft beside the one in force (ui.md A4): which draft an editor opens, and what the
 * plan of publishing it lets a person do next. Publishing is the configuration's own plan and
 * apply, so the reach a person sees is the one every other door shows.
 */

import type { ConfigDraft, ConfigDraftList, ConfigPlan } from '@hubtask/sync-engine';

/** The draft of one block, if one waits. */
export function draftOf(list: ConfigDraftList | undefined, section: ConfigDraft['section'], key: string): ConfigDraft | undefined {
  return list?.drafts.find((draft) => draft.section === section && draft.key === key);
}

/** What a publish plan allows: nothing where it refuses, an acknowledgement where it hides data or changes permissions. */
export function publishStep(plan: Pick<ConfigPlan, 'plan_hash' | 'refusals' | 'flags' | 'changes'>): 'refused' | 'unchanged' | 'acknowledge' | 'ready' {
  if (plan.refusals.length > 0 || !plan.plan_hash) return 'refused';
  if (plan.changes.length === 0) return 'unchanged';
  return plan.flags.hides_data || plan.flags.changes_permissions ? 'acknowledge' : 'ready';
}

/** The page a draft of each section is edited and published on (ui.md A4, Changes → Drafts). */
const DRAFT_PAGES: Readonly<Record<ConfigDraft['section'] | 'workflows', string>> = {
  types: '/administration/types',
  sla_policies: '/desk/settings',
  roles: '/administration/people?tab=permissions',
  workflows: '/administration/workflows',
};

export const draftPage = (section: ConfigDraft['section'] | 'workflows'): string => DRAFT_PAGES[section];

/** A row of Changes → Drafts: every drafted block, the workflows' own drafts beside the others. */
export interface DraftRow {
  readonly path: string;
  readonly name?: string;
  readonly savedAt?: string;
  readonly href: string;
}

export function draftRows(
  list: ConfigDraftList | undefined,
  workflows: readonly { key: string; name?: string | null; draft?: unknown }[],
): DraftRow[] {
  const blocks = (list?.drafts ?? []).map((draft) => ({
    path: `${draft.section}.${draft.key}`,
    savedAt: draft.saved_at,
    href: draftPage(draft.section),
  }));
  const flows = workflows
    .filter((each) => each.draft !== null && each.draft !== undefined)
    .map((each) => ({ path: `workflows.${each.key}`, name: each.name ?? undefined, href: draftPage('workflows') }));
  return [...flows, ...blocks];
}
