// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A product's empty answer links its guide and says whom to ask (user-experience.md §5.5).
 *
 * The guide is the workspace's documentation address (`docs_url`) as resolved, its root and no
 * path below it: how a workspace's own documentation is laid out is the workspace's, so a path
 * Hubtask made up could lead nowhere. Where no address is set there is no link.
 */

/** The products whose first use links the guide. */
const GUIDED: ReadonlySet<string> = new Set([
  'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS',
]);

/** The documentation address for a product's guide, or none where either is missing. */
export function guideOf(docsUrl: string | undefined, module: string): string | undefined {
  if (!docsUrl || !GUIDED.has(module)) return undefined;
  return docsUrl;
}

/** Whether a module links the guide at all. */
export function hasGuide(module: string): boolean {
  return GUIDED.has(module);
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
