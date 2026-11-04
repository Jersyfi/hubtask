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
  'desk-settings': 'SERVICE_DESK',
  'entry-types': 'WORKFLOWS',
  workflows: 'WORKFLOWS',
  'time-settings': 'TIME',
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
};

// A product's home reached by a link while its module is off draws the neutral screen with its way
// forward (user-experience.md §2.8), not an empty list.
export const moduleOfRoute = (route: string): string | undefined => ROUTE_MODULES[route] ?? PRODUCT_HOMES[route];

/**
 * The module the page belongs to where it is off in the workspace; none while the manifest is
 * not read, since a page withheld on a guess is a page taken away.
 */
export function offModuleOf(route: string, modulesOn: readonly string[] | undefined): string | undefined {
  const module = moduleOfRoute(route);
  return module && modulesOn && !modulesOn.includes(module) ? module : undefined;
}

/**
 * Whether a section of a module is drawn: only once the manifest says the module is on, since a
 * section's read while it is off is refused with `module.off`.
 */
export function isModuleOn(module: string, modulesOn: readonly string[] | undefined): boolean {
  return modulesOn?.includes(module) ?? false;
}
