// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which layouts a collection is shown in: what the screen's switch offers without a reason, and
 * what the palette offers as *Open … as …*. One function for both, so the two cannot disagree.
 *
 * Pure, and tested in `palette/sources.test.ts`, where the palette's use of it is.
 */

import type { Capabilities, Container } from '@hubtask/sync-engine';

import { narrowedTo } from '../data/offer.ts';

/** The layouts this client can actually draw. */
export const DRAWABLE: readonly string[] = ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'];

/** The two stored values of the list, which is one place with "show what is inside" as a toggle. */
const LISTS = ['LIST_COLLAPSED', 'LIST_EXPANDED'];

/** Whether a stored layout is one of the list's two. */
export const isListLayout = (layout: string): boolean => LISTS.includes(layout);

/**
 * The layouts a collection can be shown in here: those the manifest reports, the collection's
 * structure offers (UC-WRK-23 check 4) **and** this client draws, the list once (the collapsed
 * value where offered). A hub has none.
 *
 * A layout reported and not drawable is drawn by the screen's switch with its reason; it is not in
 * this list, which is what may be chosen.
 */
export function layoutsOffered(
  container: Container,
  manifest: Capabilities | undefined,
  drawable: readonly string[] = DRAWABLE,
): readonly string[] {
  if (container.type !== 'COLLECTION') return [];
  const reported = (narrowedTo(manifest, container.structure?.settings)?.view_layouts ?? []) as readonly string[];
  const offered = reported.filter((layout) => drawable.includes(layout));
  const list = offered.find((layout) => layout === 'LIST_COLLAPSED') ?? offered.find(isListLayout);
  const rest = offered.filter((layout) => !isListLayout(layout));
  return list ? [list, ...rest] : rest;
}

/**
 * The layout a collection opens in: the one its structure names (`views.default`) where it may be
 * chosen here, else the first that may - the list where it is offered. A list layout named as the
 * default opens as itself, so a project opens with what is inside shown.
 */
export function openingLayout(container: Container, offered: readonly string[]): string {
  const named = container.structure?.settings.views?.default as string | undefined;
  if (named && (offered.includes(named) || (isListLayout(named) && offered.some(isListLayout)))) return named;
  return offered[0] ?? 'LIST_COLLAPSED';
}
