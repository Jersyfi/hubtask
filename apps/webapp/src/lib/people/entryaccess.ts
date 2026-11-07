// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { AccessExplanation } from '@hubtask/sync-engine';

/** The decision `GET /access/explain` answers at an entry. */
export type EntryDecision = NonNullable<AccessExplanation['entry']>;

/** What a person can be asked about at an entry, in the contract's order. */
export const ENTRY_ACTIONS = ['READ', 'CHANGE', 'COMMENT'] as const satisfies readonly EntryDecision['action'][];

const BY_REASON: Readonly<Record<EntryDecision['reason'], string>> = {
  THROUGH_ROLE: 'app.access.entry_through_role',
  THROUGH_SHARE: 'app.access.entry_through_share',
  AS_ASSIGNEE: 'app.access.entry_as_assignee',
  NOT_ASSIGNEE: 'app.access.entry_not_assignee',
  ROLE_REFUSES: 'app.access.entry_role_refuses',
  NOTHING_HELD: 'app.access.entry_nothing_held',
  OWNER_REFUSES: 'app.access.entry_owner_refuses',
};

/** The sentence for a decision: its reason's, or the bare verdict for a reason added later. */
export function entryVerdictCode(decision: EntryDecision): string {
  return BY_REASON[decision.reason] ?? (decision.permitted ? 'app.access.entry_yes' : 'app.access.entry_no');
}
