// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A product's empty answer links its guide and says whom to ask (user-experience.md §5.5).
 *
 * The guide is the workspace's documentation address (`docs_url`) with the product's own path;
 * where no address is set there is no link, never one to a place nobody wrote.
 */

/** The path of each product's guide below the documentation address. */
const GUIDES: Readonly<Record<string, string>> = {
  PLANNING: 'products/plans',
  KNOWLEDGE: 'products/pages',
  WHITEBOARD: 'products/whiteboards',
  SERVICE_DESK: 'products/service-desk',
  ASSETS: 'products/assets',
  TIME: 'products/time',
  GOALS: 'products/goals',
  AGENTS: 'products/agents',
};

/** The product's guide at the documentation address, or none where either is missing. */
export function guideOf(docsUrl: string | undefined, module: string): string | undefined {
  const path = GUIDES[module];
  if (!docsUrl || !path) return undefined;
  return `${docsUrl.replace(/\/+$/, '')}/${path}`;
}

/** Whether a module has a guide path at all. */
export function hasGuide(module: string): boolean {
  return GUIDES[module] !== undefined;
}

/**
 * Whom a reader who may not create asks: the first administrator the place's explanation names, in
 * the sentence *Ask Anna Keller or another administrator*, or any administrator where none is named.
 */
export function whoToAsk(
  administrators: readonly { readonly name: string }[] | undefined,
  t: (code: string, params?: Record<string, string>) => string,
): string {
  const first = administrators?.find((each) => each.name.trim() !== '');
  return first ? t('app.product_empty.ask_named', { name: first.name }) : t('app.product_empty.ask_any');
}
