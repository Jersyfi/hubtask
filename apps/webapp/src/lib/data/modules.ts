// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the *Modules* section decides before it sends anything (UC-INS-21): whether a switch asks
 * first, why a switch is not offered, and whether the place may hand a module back to the level
 * above. The server refuses the same cases; this only keeps a control from promising what it would
 * refuse (P-05).
 */

import type { ModuleState } from '@hubtask/sync-engine';

/** The place the section switches for: the workspace, or one hub. */
export type ModulePlace = 'WORKSPACE' | 'HUB';

/** Switching off asks first and says the data is kept and hidden (check 4); switching on does not. */
export function asksFirst(state: ModuleState, next: boolean): boolean {
  return state.on && !next;
}

/**
 * The message code of why a module cannot be switched here, or none: a lock above (check 6), or -
 * at a hub - a module the workspace has off, which a hub never widens (check 3).
 */
export function unavailableBecause(state: ModuleState, place: ModulePlace): string | undefined {
  if (state.locked_by === 'INSTANCE') return 'app.modules.locked_installation';
  if (state.locked_by === 'PLAN') return 'app.modules.locked_plan';
  if (state.locked_by === 'WORKSPACE' && place === 'HUB') return 'app.modules.locked_workspace';
  if (place === 'HUB' && !state.on && state.source !== 'HUB') return 'app.modules.off_in_workspace';
  return undefined;
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
