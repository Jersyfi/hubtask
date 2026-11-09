// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the administration's *Templates* list page narrows by (page-system §9.1): its strip of three
 * sibling lists, each its own address, and the catalogue's family chip over the templates the
 * client holds - `GET /config/templates` has no filter of its own.
 */

import { matchesSearch } from '../page/list.ts';
import { familyOf, type Family } from '../workspace/hubGallery.ts';

export const TEMPLATE_TABS = [
  { id: 'catalogue', code: 'app.templates.tab_catalogue' },
  { id: 'in-use', code: 'app.templates.tab_in_use' },
  { id: 'presets', code: 'app.templates.tab_presets' },
] as const;

export type TemplateTab = (typeof TEMPLATE_TABS)[number]['id'];

export const TEMPLATES_PATH = '/administration/templates';

/** The tab an address names; the catalogue where it names none or an unknown one. */
export function tabOf(tab: string | undefined): TemplateTab {
  return TEMPLATE_TABS.find((each) => each.id === tab)?.id ?? 'catalogue';
}

/** A tab's address: the catalogue at the screen's own, the others by `?tab=`. */
export function tabPath(tab: TemplateTab): string {
  return tab === 'catalogue' ? TEMPLATES_PATH : `${TEMPLATES_PATH}?tab=${tab}`;
}

interface Listed {
  readonly key: string;
  readonly origin: 'HUBTASK' | 'INSTALLATION' | 'WORKSPACE' | 'OWN';
  readonly taken_version?: number | null;
  readonly update_available?: boolean;
}

/** Whether the workspace took a version of the template. */
export const isInUse = (template: Listed): boolean => template.taken_version !== null && template.taken_version !== undefined;

/** What the strip and the head count: the catalogue, the templates in use, the updates waiting. */
export function figures(templates: readonly Listed[]): { catalogue: number; inUse: number; updates: number } {
  const used = templates.filter(isInUse);
  return { catalogue: templates.length, inUse: used.length, updates: used.filter((each) => each.update_available === true).length };
}

/** The templates a tab shows, narrowed by the family chip (none chosen is all) and the search. */
export function narrowed<T extends Listed>(
  templates: readonly T[],
  options: { readonly tab: TemplateTab; readonly family?: Family; readonly search: string; readonly words: (template: T) => readonly (string | undefined)[] },
): T[] {
  return templates.filter((each) =>
    (options.tab !== 'in-use' || isInUse(each))
    && (options.family === undefined || familyOf(each) === options.family)
    && matchesSearch(options.search, options.words(each)));
}
