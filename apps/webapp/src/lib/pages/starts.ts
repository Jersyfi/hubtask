// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// How a new page starts (S-6, scene pnew): the page opens at once, and while its body is empty the
// templates stand as chips under its title, one previewed beside it before it is used.

import { SHIPPED_TEMPLATES, type ShippedSpace, type ShippedTemplate } from '../data/pages.svelte.ts';

/** What a chip starts the page from: Hubtask's own template, the space's own, or a whole space's pages. */
export type PageStart =
  | { readonly kind: 'shipped'; readonly key: ShippedTemplate }
  | { readonly kind: 'own'; readonly id: string; readonly title: string; readonly icon: string | null }
  | { readonly kind: 'space'; readonly key: ShippedSpace };

/** Each of Hubtask's templates has its mark, given to a page started from it that has none. */
export const START_ICONS: Readonly<Record<ShippedTemplate, string>> = {
  meeting: '🗓️', decision: '⚖️', retro: '🔄', howto: '📘', brief: '🎯',
};

/** The order the chips stand in, the most used first. */
export const CHIP_ORDER: readonly ShippedTemplate[] = ['meeting', 'decision', 'retro', 'howto', 'brief'];

/** How many of the space's own templates are chips; the rest wait under *All templates*. */
export const OWN_CHIPS = 2;

/** The chips under an empty page's title: Hubtask's templates, then the first of the space's own. */
export function startChips(own: readonly { id: string; title: string; icon: string | null }[]): PageStart[] {
  const shipped = CHIP_ORDER.filter((key) => SHIPPED_TEMPLATES.includes(key)).map((key) => ({ kind: 'shipped', key }) as const);
  return [...shipped, ...own.slice(0, OWN_CHIPS).map((row) => ({ kind: 'own', id: row.id, title: row.title, icon: row.icon }) as const)];
}

/** A page offers its starts while its body holds nothing but blank lines. */
export const isBlankBody = (body: string | undefined): boolean => (body ?? '').trim() === '';

/** A start's identity, for the chip that is chosen. */
export const startKey = (start: PageStart): string => (start.kind === 'own' ? `own:${start.id}` : `${start.kind}:${start.key}`);

/**
 * What using a start writes: its body, its title where the page still has none of its own (the
 * untitled one or nothing), its mark where the page has none.
 */
export function startChange(
  page: { readonly title: string; readonly icon: string | null },
  untitled: string,
  start: { readonly title: string; readonly body: string; readonly icon?: string | null },
): { body: string; title?: string; icon?: string } {
  const change: { body: string; title?: string; icon?: string } = { body: start.body };
  const title = page.title.trim();
  if ((title === '' || title === untitled) && start.title.trim() !== '') change.title = start.title;
  if (!page.icon && start.icon) change.icon = start.icon;
  return change;
}
