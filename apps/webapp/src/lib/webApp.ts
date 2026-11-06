// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Area } from './router.ts';

/**
 * Where an area the Android app leaves out lives (ADR-0032, UC-SYN-07 check 7): the web app of the
 * server the app is signed in to. The workspace's administration is at the workspace's own
 * address; the installation's area at the installation's, which a multi-mode installation names in
 * its manifest and a single-mode one shares with its one workspace. Pure, so the address has a test.
 */
export interface WebAppOrigins {
  /** The workspace's web app: `platform.webOrigin`. */
  readonly workspace: string;
  /** The installation's own address, where the manifest names one. */
  readonly installation?: string | null;
}

/** The origin an area's web app answers at. */
export function webAppOrigin(area: Area, origins: WebAppOrigins): string {
  if (area === 'instance' && origins.installation) {
    try {
      return new URL(origins.installation).origin;
    } catch {
      // A manifest that names no address names none: the workspace's.
    }
  }
  return new URL(origins.workspace).origin;
}

/** The host the account menu names under the row: "In the web app at tasks.acme.com". */
export function webAppHost(area: Area, origins: WebAppOrigins): string {
  return new URL(webAppOrigin(area, origins)).host;
}

/** The same address in the web app: the path and query the app was asked for, at the area's origin. */
export function webAppAddress(area: Area, address: string, origins: WebAppOrigins): URL {
  const asked = new URL(address, 'https://app.invalid');
  return new URL(`${asked.pathname}${asked.search}`, webAppOrigin(area, origins));
}
