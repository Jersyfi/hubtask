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
