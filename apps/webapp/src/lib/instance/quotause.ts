// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the installation knows of a workspace's use of its limits, from `/admin/quotas` - the one
 * complete answer of which workspaces stand at 90% or more of a quota (user-experience.md §8.4).
 * A workspace the answer does not name is below 90% everywhere; its use is not known here.
 */

import type { WorkspaceNearQuota } from '@hubtask/sync-engine';

export type LimitStanding = 'near' | 'at';

/** `at` where one quota is used up, `near` where one stands at 90% or more, else nothing. */
export function limitStanding(entry: WorkspaceNearQuota | undefined): LimitStanding | undefined {
  if (!entry || entry.quotas.length === 0) return undefined;
  return entry.quotas.some((standing) => (standing.used ?? 0) >= standing.limit) ? 'at' : 'near';
}

/** The use of one quota, where the answer carries it. */
export function useOf(entry: WorkspaceNearQuota | undefined, quota: string): number | undefined {
  const used = entry?.quotas.find((standing) => standing.quota === quota)?.used;
  return typeof used === 'number' ? used : undefined;
}

/**
 * Whether a ceiling typed in the dialog lies below what is in use: allowed, and it refuses the next
 * creation without removing anything (multi-tenancy.md §4.2), which the dialog says before saving.
 */
export const isBelowUse = (typed: number | undefined, used: number | undefined): boolean =>
  typed !== undefined && used !== undefined && typed < used;
