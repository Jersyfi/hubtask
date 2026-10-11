// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a screen needs to know before a hub is made private (UC-ID-16), from the memberships the
 * people store already read along the hub's path.
 *
 * Predictions, not decisions: the server answers who may mark a hub and who loses it. These say
 * which control to offer and what the confirmation should name, and the server's refusal is what a
 * reader meets when they are wrong.
 */

/** The slice of a membership these questions read. */
export interface Held {
  readonly account_id?: string | null;
  readonly group_id?: string | null;
  readonly scope_type: string;
  readonly scope_id?: string | null;
  readonly role: string;
}

/**
 * Whether the account holds `OWNER` on the hub itself - what marking and unmarking take. A role on
 * the workspace does not count: it is exactly what a private hub stops reaching.
 */
export function ownsHub(memberships: readonly Held[], accountId: string | undefined, hubId: string): boolean {
  if (!accountId) return false;
  return memberships.some(
    (each) => each.account_id === accountId && each.scope_type === 'HUB' && each.scope_id === hubId && each.role === 'OWNER',
  );
}

/**
 * The people who read the hub through the workspace and hold nothing on the hub itself: who loses
 * it when it turns private.
 *
 * A role on one of its collections or entries keeps what it reaches, and is not on the hub's path,
 * so a person named here may keep part of it - the confirmation says so in its own sentence. Each
 * person once, in the order the memberships came.
 */
export function shutOut(memberships: readonly Held[], hubId: string): readonly string[] {
  const onHub = new Set(
    memberships
      .filter((each) => each.scope_type === 'HUB' && each.scope_id === hubId && each.account_id)
      .map((each) => each.account_id as string),
  );
  const out: string[] = [];
  for (const each of memberships) {
    if (each.scope_type !== 'TENANT' || !each.account_id) continue;
    if (onHub.has(each.account_id) || out.includes(each.account_id)) continue;
    out.push(each.account_id);
  }
  return out;
}
