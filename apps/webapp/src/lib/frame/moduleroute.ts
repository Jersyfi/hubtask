// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which workspace module an administration page belongs to, and whether it is off: a page of a
 * module switched off is reached by an old link or a typed address, and says so before it asks
 * the server anything - every read there would be refused with `module.off`.
 */

const ROUTE_MODULES: Readonly<Record<string, string>> = {
  'service-desk': 'SERVICE_DESK',
  'desk-desks': 'SERVICE_DESK',
  'desk-reports': 'SERVICE_DESK',
  'desk-settings': 'SERVICE_DESK',
  'desk-settings-section': 'SERVICE_DESK',
  'entry-types': 'WORKFLOWS',
  workflows: 'WORKFLOWS',
  'time-settings': 'TIME',
  'assets-classes': 'ASSETS',
  'assets-sources': 'ASSETS',
  'assets-settings': 'ASSETS',
  'assets-settings-section': 'ASSETS',
  approvals: 'SERVICE_DESK',
  notes: 'KNOWLEDGE',
  // The hub tools' pages (the navigation concept §2.4): an old link to a tool switched off says so.
  'hub-plan': 'PLANNING',
  'hub-plan-table': 'PLANNING',
  'hub-plan-dependencies': 'PLANNING',
  'hub-plan-releases': 'PLANNING',
  'hub-plan-sprints': 'PLANNING',
  'hub-inventory': 'ASSETS',
  'hub-inventory-section': 'ASSETS',
  'hub-time': 'TIME',
  'hub-time-workload': 'TIME',
  'hub-time-budget': 'TIME',
  'hub-goals': 'GOALS',
  'hub-goals-dashboards': 'GOALS',
};

/** The module the route's page belongs to, or none. */
/** The products' homes drawn by the one home view, by route (user-experience.md §2.2). */
export const PRODUCT_HOMES: Readonly<Record<string, string>> = {
  'plans-home': 'PLANNING',
  'pages-home': 'KNOWLEDGE',
  'whiteboards-home': 'WHITEBOARD',
  'desk-home': 'SERVICE_DESK',
  'assets-home': 'ASSETS',
  'agents-home': 'AGENTS',
  'agents-settings': 'AGENTS',
  'agents-settings-section': 'AGENTS',
};

/** The products whose home is a page of its own rather than the one home view. */
const OWN_HOMES: Readonly<Record<string, string>> = {
  time: 'TIME',
  goals: 'GOALS',
};

// A product's home reached by a link while its module is off draws the neutral screen with its way
// forward (user-experience.md §2.8), not an empty list.
export const moduleOfRoute = (route: string): string | undefined => ROUTE_MODULES[route] ?? PRODUCT_HOMES[route] ?? OWN_HOMES[route];

/** Whether the route is a product's home, whose trail is *Products* rather than the administration's. */
export const isProductHome = (route: string): boolean => route in PRODUCT_HOMES || route in OWN_HOMES;

/**
 * The module the page belongs to where it is off in the workspace; none while the manifest is
 * not read, since a page withheld on a guess is a page taken away.
 */
export function offModuleOf(route: string, modulesOn: readonly string[] | undefined): string | undefined {
  const module = moduleOfRoute(route);
  if (!module || !modulesOn) return undefined;
  if (!modulesOn.includes(module)) return module;
  return (NEEDS[module] ?? []).find((needed) => !modulesOn.includes(needed));
}

/** What a module cannot work without, as the server's `Key.Needs` answers it. */
const NEEDS: Readonly<Record<string, readonly string[]>> = { SERVICE_DESK: ['WORKFLOWS'] };

/**
 * Whether a section of a module is drawn: only once the manifest says the module is on, since a
 * section's read while it is off is refused with `module.off`.
 */
export function isModuleOn(module: string, modulesOn: readonly string[] | undefined): boolean {
  return modulesOn?.includes(module) ?? false;
}
