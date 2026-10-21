// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The actions a screen offers the palette: exactly what its head offers, and of that only what can
 * be done now (UC-WRK-27 check 2, P-05).
 *
 * **The head has already decided.** Each screen builds its head's lists - the primary action with
 * its own list, the secondary ones, the menu - with a role refusal left out and a "not possible
 * right now" carrying its `disabledReason` (design-system.md §4, the eight cases). So the palette
 * computes nothing: it takes the same lists, flattens them, and drops every item with a reason. A
 * palette that asked its own questions would be a second answer to "may I", and the two would
 * drift.
 *
 * Pure, and tested beside itself; `commands.svelte.ts` holds what the screens offer.
 */

import type { IconName, MenuItem, PageAction } from '@hubtask/design-system/components';

/** One action the palette may list: what it says, and what pressing it does. */
export interface Act {
  /** Unique among everything offered at once: the source's name, then the item's. */
  readonly id: string;
  readonly label: string;
  readonly icon?: IconName;
  /** The shortcut that does the same, by its id in `lib/shortcuts.ts`. */
  readonly shortcut?: string;
  readonly run: () => void;
}

/** A head's three lists, in the shape `PageHeader` takes them. */
export interface HeadLists {
  readonly primary?: PageAction;
  readonly secondary?: readonly PageAction[];
  readonly menu?: { readonly items: readonly MenuItem[]; readonly onselect: (id: string) => void };
}

/**
 * The head's lists as actions, in the head's own order - the primary, its list, the secondary
 * ones, the menu - each item with a reason left out.
 */
export function actionsOfHead(source: string, head: HeadLists): readonly Act[] {
  const acts: Act[] = [];
  const { primary, secondary = [], menu } = head;
  if (primary && !primary.disabledReason) {
    acts.push({ id: `${source}:primary`, label: primary.label, icon: primary.icon, run: primary.onclick });
  }
  const primaryMenu = primary?.menu;
  if (primaryMenu) {
    for (const item of primaryMenu.items) {
      if (item.disabledReason) continue;
      acts.push({ id: `${source}:primary-${item.id}`, label: item.label, icon: item.icon, run: () => primaryMenu.onselect(item.id) });
    }
  }
  secondary.forEach((action, index) => {
    if (action.disabledReason) return;
    acts.push({ id: `${source}:secondary-${index}`, label: action.label, icon: action.icon, run: action.onclick });
  });
  if (menu) {
    for (const item of menu.items) {
      if (item.disabledReason) continue;
      acts.push({ id: `${source}:menu-${item.id}`, label: item.label, icon: item.icon, run: () => menu.onselect(item.id) });
    }
  }
  return acts;
}
