// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where each part of the time policy comes from (UC-TIM-03): a part the installation or the plan
 * locked is not the workspace's to change, so its control is drawn unavailable with the reason.
 */

import type { TimePolicy } from '@hubtask/sync-engine';

/** The two parts by their settings catalogue key. */
export type PolicyPart = 'time.locked_through' | 'time.visibility';

/** The part's source and lock as the server named them; nothing from a server or copy that names none. */
export function partOf(policy: Pick<TimePolicy, 'settings'> | undefined, part: PolicyPart): { source?: string; locked_by?: string | null } {
  return policy?.settings?.[part] ?? {};
}

/** The level above the workspace that locked the part, or undefined where the workspace decides it. */
export function lockedAbove(policy: Pick<TimePolicy, 'settings'> | undefined, part: PolicyPart): 'INSTANCE' | 'PLAN' | undefined {
  const lock = partOf(policy, part).locked_by;
  return lock === 'INSTANCE' || lock === 'PLAN' ? lock : undefined;
}
