// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A product's tour (user-experience.md §3.4): three steps on its home - where to create, where to
 * find it again, where to configure it - and the first visit's mark, which is the first of them,
 * once per person. Pure, tested beside; `producttour.svelte.ts` walks it.
 *
 * Where to configure is the product's switch on *Modules*, which carries the link to the product's
 * own settings where it has them; a reader who does not reach the administration never sees that
 * screen, and the step is passed over as every step whose element does not arrive.
 */

import { productOf } from '../navigation.ts';

export type ProductStepId = 'create' | 'find' | 'configure';

export interface ProductStep {
  readonly id: ProductStepId;
  readonly route: string;
  readonly selector: string;
  readonly side: 'block-end' | 'inline-end';
}

/** The three steps of `module`'s tour, or none for a module without a home. */
export function productTourSteps(module: string): readonly ProductStep[] {
  const row = productOf(module);
  if (!row) return [];
  return [
    // The product's create act: the page head's one primary, or the empty home's where the head
    // leaves it to the empty state.
    { id: 'create', route: row.path, selector: 'header.head .primary button, [data-tour="product-create"]', side: 'block-end' },
    // Its row in the column, which only `expanded` pins; elsewhere the step is passed over.
    { id: 'find', route: row.path, selector: `[data-tour="hubs"] [data-node="${row.id}"]`, side: 'inline-end' },
    { id: 'configure', route: '/administration/modules', selector: `li[data-module="${module}"]`, side: 'block-end' },
  ];
}

/** The module whose home `routeName` is, where it is one. */
export function homeModuleOf(routeName: string | null | undefined, homes: readonly { readonly module: string; readonly routes: readonly string[] }[]): string | undefined {
  return routeName ? homes.find((row) => row.routes[0] === routeName)?.module : undefined;
}

/**
 * Whether the first visit's mark stands for `module` now: never seen by this person, the account
 * known, and nothing else leading them - the starter and the base tour come first (§3.1).
 */
export function marksFirstVisit(
  module: string | undefined,
  account: { readonly products_seen?: readonly string[] } | undefined,
  busy: { readonly baseTourOpen: boolean; readonly baseTourMayStart: boolean; readonly baseTourTaken: boolean },
): boolean {
  if (!module || !account) return false;
  if (busy.baseTourOpen || !busy.baseTourMayStart || !busy.baseTourTaken) return false;
  return !(account.products_seen ?? []).includes(module);
}

/** The list written back once `module`'s mark was seen: the account's, with it at the end, once. */
export function withSeen<M extends string>(seen: readonly M[] | undefined, module: M): M[] {
  const list = [...(seen ?? [])];
  return list.includes(module) ? list : [...list, module];
}
