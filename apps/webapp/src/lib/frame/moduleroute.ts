// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which workspace module an administration page belongs to, and whether it is off: a page of a
 * module switched off is reached by an old link or a typed address, and says so before it asks
 * the server anything - every read there would be refused with `module.off`.
 */

const ROUTE_MODULES: Readonly<Record<string, string>> = {
  'service-desk': 'SERVICE_DESK',
  'entry-types': 'WORKFLOWS',
  workflows: 'WORKFLOWS',
  'time-settings': 'TIME',
};

/** The module the route's page belongs to, or none. */
export const moduleOfRoute = (route: string): string | undefined => ROUTE_MODULES[route];

/**
 * The module the page belongs to where it is off in the workspace; none while the manifest is
 * not read, since a page withheld on a guess is a page taken away.
 */
export function offModuleOf(route: string, modulesOn: readonly string[] | undefined): string | undefined {
  const module = moduleOfRoute(route);
  return module && modulesOn && !modulesOn.includes(module) ? module : undefined;
}
