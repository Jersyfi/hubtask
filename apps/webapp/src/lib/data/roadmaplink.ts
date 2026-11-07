// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A portfolio roadmap's public link (UC-GOL-03 check 15), the parts that hold no state: where the
 * link points and how the shared screen reads the token back.
 *
 * The token rides in the fragment, `/shared-roadmap#hbt_road_…`, as a public form's does: a
 * fragment never reaches a server log or a Referer, and the token is the link's whole credential.
 */

import type { RoadmapRow } from './roadmap.ts';

const PREFIX = 'hbt_road_';

/** The address a roadmap's public link opens, on this server. */
export const roadmapLinkAddress = (origin: string, token: string) => `${origin.replace(/\/+$/u, '')}/shared-roadmap#${token}`;

/** The token a fragment carries, or empty for anything that is not one. */
export function roadmapTokenOf(fragment: string): string {
  const token = fragment.replace(/^#/u, '').trim();
  return token.startsWith(PREFIX) && /^[A-Za-z0-9_-]+$/u.test(token) ? token : '';
}

/** Where the shared screen reads the roadmap, in the visitor's own zone. */
export const sharedRoadmapPath = (token: string, zone: string) =>
  `/public/roadmaps/${encodeURIComponent(token)}?zone=${encodeURIComponent(zone)}`;

/**
 * The rows as somebody outside the workspace sees them: none offers a press into a workspace the
 * visitor cannot enter.
 */
export const outsiderRows = (rows: readonly RoadmapRow[]): RoadmapRow[] => rows.map((row) => ({ ...row, href: undefined }));
