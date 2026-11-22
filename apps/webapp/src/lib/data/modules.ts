// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the *Modules* section decides before it sends anything (UC-INS-21): whether a switch asks
 * first and what its question offers, why a switch is not offered, and whether the place may hand
 * a module back to the level above. The server refuses the same cases; this only keeps a control from promising what it would
 * refuse (P-05).
 */

import type { ModuleReach, ModuleState, ModuleSwitch } from '@hubtask/sync-engine';

/** The place the section switches for: the workspace, or one hub. */
export type ModulePlace = 'WORKSPACE' | 'HUB';

/** Switching off asks first and says the data is kept and hidden (check 4). */
export function asksFirst(state: ModuleState, next: boolean): boolean {
  return state.on && !next;
}

/**
 * The message code of why a module cannot be switched here, or none: a lock above (check 6), or -
 * at a hub - a module the workspace has off, which a hub never switches on (check 3). The
 * workspace's set decides that, not where the hub's value comes from: a hub keeps a value of its
 * own under a workspace that is off. Unread, the workspace's set allows nothing.
 */
export function unavailableBecause(
  state: ModuleState, place: ModulePlace, workspaceOn?: readonly string[],
): string | undefined {
  if (state.locked_by === 'INSTANCE') return 'app.modules.locked_installation';
  if (state.locked_by === 'PLAN') return 'app.modules.locked_plan';
  if (state.locked_by === 'WORKSPACE' && place === 'HUB') return 'app.modules.locked_workspace';
  if (place === 'HUB' && !state.on && !(workspaceOn ?? []).includes(state.module)) return 'app.modules.off_in_workspace';
  return undefined;
}

/**
 * The hubs the question of a workspace's switch-on offers (check 13): those the reader may shape.
 * A shared hub follows the reader's standing on the workspace; a private one only a role held on
 * it, and neither is offered while its answer is out.
 */
export function hubsOffered<H extends { readonly private?: boolean }>(
  hubs: readonly H[], shapesWorkspace: boolean | undefined, shapesPrivate: (hub: H) => boolean | undefined,
): H[] {
  return hubs.filter((hub) => (hub.private === true ? shapesPrivate(hub) : shapesWorkspace) === true);
}

/**
 * Whether switching on asks in which existing hubs (check 13): at the workspace, and only where
 * there is a hub to choose - without one the switch goes through at once (check 2).
 */
export function asksWhichHubs(state: ModuleState, next: boolean, place: ModulePlace, offered: readonly unknown[]): boolean {
  return place === 'WORKSPACE' && next && !state.on && offered.length > 0;
}

/** What a switch sends: the hubs chosen go with a workspace's switch-on only, as the server asks. */
export function switchBody(to: ModuleSwitch['switch'], hubId?: string, chosen: readonly string[] = []): ModuleSwitch {
  if (hubId) return { switch: to, hub_id: hubId };
  return to === 'ON' && chosen.length > 0 ? { switch: to, hub_ids: [...chosen] } : { switch: to };
}

/** Whether this place decided the module itself, so that it may leave it to the level above. */
export function isSetHere(state: ModuleState, place: ModulePlace): boolean {
  return state.source === place && state.locked_by === undefined;
}

/** The message code of a module's name and of the sentence beneath it. */
export function moduleCodes(state: ModuleState): { readonly name: string; readonly hint: string } {
  const key = state.module.toLowerCase();
  return { name: `app.modules.name_${key}`, hint: `app.modules.hint_${key}` };
}

/**
 * The modules a hub's or a collection's screens are drawn for: the place's own set once it is read,
 * never wider than the workspace's (check 3), and the workspace's until then.
 */
export function modulesHere<K extends string>(
  workspace: readonly K[] | undefined, place: readonly string[] | undefined,
): K[] {
  const outer = [...(workspace ?? [])];
  return place ? outer.filter((key) => place.includes(key)) : outer;
}

/**
 * What the switch-off's question says it reaches, as the server counted it (check 4): the people
 * left without the module and the switched-on rules that go quiet. A count of nothing is not said.
 */
export function offReach(reach: ModuleReach | undefined): { code: string; params: Record<string, number> }[] {
  if (!reach?.on) return [];
  const lines: { code: string; params: Record<string, number> }[] = [];
  if (reach.people > 0) lines.push({ code: 'app.modules.off_people', params: { count: reach.people } });
  if ((reach.rules ?? 0) > 0) lines.push({ code: 'app.modules.off_rules', params: { count: reach.rules ?? 0 } });
  return lines;
}
