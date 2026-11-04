// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the card under a switch offers once a module is on (user-experience.md §2.8), the same for
 * every module: where to find it - the product's home, or the hub's tab where it was switched at a
 * hub - and the settings it still needs before it is of use, where it has any.
 */

import { productOf } from '../navigation.ts';
import { placeTabs } from '../workspace/placeTabs.ts';

export interface SwitchedOn {
  /** Where the product is now: its row's home, or the hub's tab; none for a module without one. */
  readonly find?: { readonly code: string; readonly href: string };
  /** The settings page it still needs, where it has one. */
  readonly setup?: { readonly code: string; readonly href: string };
}

/** The settings each module needs before it is of use, where it has a page of them. */
const SETUP: Readonly<Record<string, { code: string; href: string }>> = {
  SERVICE_DESK: { code: 'app.modules.on_setup_desk', href: '/desk/settings' },
  TIME: { code: 'app.modules.on_setup_time', href: '/time?tab=settings' },
  WORKFLOWS: { code: 'app.modules.on_setup_workflows', href: '/administration/workflows' },
};

export function switchedOn(module: string, hubId?: string): SwitchedOn {
  const tab = hubId ? placeTabs({ type: 'HUB', id: hubId, modulesOn: [module] }).slice(1)[0] : undefined;
  const row = productOf(module);
  const find = tab ? { code: tab.code, href: tab.href } : row ? { code: row.code, href: row.path } : undefined;
  return { ...(find ? { find } : {}), ...(SETUP[module] ? { setup: SETUP[module] } : {}) };
}

/**
 * Where a module's settings live at its home (user-experience.md §8.2), for a reader that home
 * offers its *Settings* tab to - the same rule each home applies to its own strip (P-05). The
 * assets' tab is offered to whoever reaches the administration, as the Modules screen's reader
 * does.
 */
export function settingsOf(module: string, holds: (permission: string) => boolean): string | undefined {
  if (module === 'SERVICE_DESK') return '/desk/settings';
  if (module === 'ASSETS') return '/assets/settings';
  if (module === 'TIME') {
    return ['STRUCTURE', 'APPROVE_TIME', 'MANAGE_COSTS', 'MANAGE_MEMBERS'].some(holds) ? '/time?tab=settings' : undefined;
  }
  if (module === 'AGENTS') return holds('MANAGE_MEMBERS') ? '/agents?tab=settings' : undefined;
  return undefined;
}
