// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's and a collection's settings as one section list (user-experience.md §5.7): General ·
 * Templates and presets · Types & forms · Workflow · Views · Rules · People, filtered to what the level
 * configures, with a level's own sections (a hub's products, a collection's labels) at a fixed
 * place among them. The structure's settings are spread over the sections they belong to; one
 * draft and one save bar still hold them all.
 */

/** A section the structure's settings are drawn in, and the settings it holds. */
export const STRUCTURE_GROUPS = {
  template: [],
  types: ['levels', 'fields', 'entry_types'],
  workflow: ['completion'],
  views: ['views', 'counts', 'capture', 'celebration'],
  rules: ['automation'],
} as const satisfies Readonly<Record<string, readonly string[]>>;

export type StructureGroup = keyof typeof STRUCTURE_GROUPS;

export const GROUP_CODES: Readonly<Record<StructureGroup, string>> = {
  template: 'app.settings.section_template',
  types: 'app.settings.section_types',
  workflow: 'app.settings.section_workflow',
  views: 'app.settings.section_views',
  rules: 'app.settings.section_rules',
};

/** The order per level; a section the place does not draw is left out, never a gap. */
export const SECTION_ORDER: Readonly<Record<'HUB' | 'COLLECTION', readonly string[]>> = {
  HUB: ['general', 'modules', 'template', 'types', 'workflow', 'views', 'rules', 'calendars', 'code', 'chat', 'people'],
  COLLECTION: ['general', 'template', 'types', 'fields', 'labels', 'templates', 'workflow', 'views', 'rules', 'people'],
};

/** Addresses of sections that were merged into another, kept so a saved link still opens. */
const MOVED: Readonly<Record<string, string>> = { structure: 'types', automation: 'rules' };

/** The section an address names, an old one answered by where its content went. */
export const sectionOf = (requested: string): string => MOVED[requested] ?? requested;

export const isGroup = (id: string): id is StructureGroup => Object.hasOwn(STRUCTURE_GROUPS, id);

/** The section a structure setting is drawn in. */
export function groupOf(setting: string): StructureGroup {
  for (const [group, settings] of Object.entries(STRUCTURE_GROUPS) as [StructureGroup, readonly string[]][]) {
    if (settings.includes(setting)) return group;
  }
  return 'types';
}

/**
 * The sections in their level's order, one per id: where a structure group and a caller's section
 * share an id (*Rules*), they are one section. An id the order does not know goes last.
 */
export function orderSections<T extends { readonly id: string }>(level: 'HUB' | 'COLLECTION', sections: readonly T[]): T[] {
  const order = SECTION_ORDER[level];
  const seen = new Set<string>();
  const unique = sections.filter((each) => (seen.has(each.id) ? false : (seen.add(each.id), true)));
  const rank = (id: string) => {
    const at = order.indexOf(id);
    return at === -1 ? order.length : at;
  };
  return unique.map((each, index) => ({ each, index })).sort((a, b) => rank(a.each.id) - rank(b.each.id) || a.index - b.index).map(({ each }) => each);
}
