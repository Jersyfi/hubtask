// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The templates a product's first visit offers (user-experience.md §4): the catalogue's own, read
 * by the module each switches on in its document, so a product never ships a second template
 * mechanism. Only templates with a hub part - the first act makes a hub from one.
 */

import { PRODUCTS } from '../navigation.ts';
import { hubScreenOf } from './home.ts';

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

/**
 * Which product a template is mainly for, among those it switches on: a service template's desk
 * before the knowledge it brings beside it, a register's assets, a project's plans. Workflows is
 * none: every shipped template switches it on.
 */
const MAIN_FIRST = ['SERVICE_DESK', 'ASSETS', 'PLANNING', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD', 'TIME', 'AGENTS'] as const;

/**
 * Where the app goes once a template is taken (user-experience.md §3.2): its main product's home,
 * the hub's screen of it where a hub took it; undefined where it brings no product of its own.
 */
export function afterTaking(template: Pick<Offered, 'document'>, hubId?: string): string | undefined {
  const main = MAIN_FIRST.find((module) => template.document?.modules?.[module.toLowerCase()]?.switch === 'ON');
  if (!main) return undefined;
  if (hubId) return hubScreenOf(main, hubId) ?? `/hubs/${encodeURIComponent(hubId)}`;
  return PRODUCTS.find((row) => row.module === main)?.path;
}
