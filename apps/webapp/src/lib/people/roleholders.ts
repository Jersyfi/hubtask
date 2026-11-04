// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How many hold each role at the workspace (user-experience.md §8.7): the people and the groups
 * of its memberships, each once. `undefined` until every page of the workspace's memberships is
 * read, so a role card never says a first page's count as if it were all of them.
 */

export interface HeldBy {
  readonly people: number;
  readonly groups: number;
}

interface Row {
  readonly role: string;
  readonly account_id?: string | null;
  readonly group_id?: string | null;
}

export function holdersByRole(rows: readonly Row[], isComplete: boolean): ReadonlyMap<string, HeldBy> | undefined {
  if (!isComplete) return undefined;
  const people = new Map<string, Set<string>>();
  const groups = new Map<string, Set<string>>();
  for (const row of rows) {
    const into = row.group_id ? groups : people;
    const id = row.group_id ?? row.account_id;
    if (!id) continue;
    if (!into.has(row.role)) into.set(row.role, new Set());
    into.get(row.role)?.add(id);
  }
  const roles = new Set([...people.keys(), ...groups.keys()]);
  return new Map([...roles].map((role) => [role, { people: people.get(role)?.size ?? 0, groups: groups.get(role)?.size ?? 0 }]));
}
