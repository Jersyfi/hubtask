// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The templates a product's first visit offers (user-experience.md §4): the catalogue's own, read
 * by the module each switches on in its document, so a product never ships a second template
 * mechanism. Only templates with a hub part - the first act makes a hub from one.
 */

interface Offered {
  readonly key: string;
  readonly origin: string;
  readonly document?: { readonly modules?: Readonly<Record<string, { readonly switch?: string } | null>> | null };
  readonly hub_document?: unknown;
}

const ORDER = ['HUBTASK', 'INSTALLATION', 'WORKSPACE', 'OWN'];

/** Hubtask's first, then the installation's and the workspace's, in the catalogue's order; three at most. */
export function productTemplates<T extends Offered>(templates: readonly T[], module: string, limit = 3): T[] {
  // Every shipped template switches workflows on; for that module the offer would say nothing.
  if (module === 'WORKFLOWS') return [];
  const name = module.toLowerCase();
  return templates
    .filter((each) => each.hub_document != null && each.document?.modules?.[name]?.switch === 'ON')
    .map((each, at) => ({ each, at }))
    .sort((a, b) => ORDER.indexOf(a.each.origin) - ORDER.indexOf(b.each.origin) || a.at - b.at)
    .slice(0, limit)
    .map(({ each }) => each);
}
