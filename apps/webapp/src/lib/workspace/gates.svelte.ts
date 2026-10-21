// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which gated area of a page says why the reader may not change it (UC-WRK-32 check 4).
 *
 * A page can hold several areas behind the same permission - retention's rules and its holds,
 * backup's targets and its schedules. The reason is said once per permission per page, at the
 * first such area; the others are silent. Every `WorkspaceGate` and `WorkspaceRefusal` registers
 * here while it is mounted, in mount order, which is document order for the areas of one page;
 * the first one registered for a permission draws the notice. Leaving the page unmounts them all,
 * so the next page starts empty.
 */

let next = 0;
const holders = $state<Record<string, readonly number[]>>({});

/** Registers an area behind `permission`; answers its ticket and a way to leave. */
export function enter(permission: string): { readonly ticket: number; leave(): void } {
  const ticket = next++;
  holders[permission] = [...(holders[permission] ?? []), ticket];
  return {
    ticket,
    leave() {
      holders[permission] = (holders[permission] ?? []).filter((each) => each !== ticket);
    },
  };
}

/** The context key under which a `WorkspaceGate` tells the `WriteOnly` controls inside it its verdict. */
export const GATE = Symbol('workspace-gate');

/** What a `WriteOnly` reads from its gate. */
export interface GateState {
  readonly status: 'permitted' | 'refused' | 'undetermined';
}

/** Whether this area is the one that says the reason for its permission. */
export function speaks(permission: string, ticket: number): boolean {
  return holders[permission]?.[0] === ticket;
}
