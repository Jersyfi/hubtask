// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The template gallery of *Create hub* (user-experience.md §5.2): the catalogue's templates a hub
 * can take, grouped by the families of configuration.md §9, after the presets as the *Simple*
 * family. The contract carries no family, so Hubtask's templates are placed by key; a template
 * published in the installation or the workspace stands under its origin.
 */

import type { IconName } from '@hubtask/design-system/components';

export const FAMILIES = ['personal', 'teams', 'projects', 'programmes', 'service', 'processes', 'registers'] as const;
export type Family = (typeof FAMILIES)[number] | 'installation' | 'workspace' | 'own' | 'other';

const FAMILY_OF: Readonly<Record<string, (typeof FAMILIES)[number]>> = {
  household: 'personal', shopping: 'personal', habits: 'personal', travel: 'personal',
  kanban_team: 'teams', scrum_team: 'teams', bug_tracking: 'teams', content_calendar: 'teams', event_planning: 'teams',
  classic_project: 'projects', hybrid_project: 'projects', client_project: 'projects',
  portfolio: 'programmes', okr: 'programmes', scaled_agile: 'programmes',
  itil_service_desk: 'service', internal_it: 'service', facility_requests: 'service', hr_requests: 'service', customer_support: 'service',
  approvals: 'processes', onboarding: 'processes', procurement: 'processes', capa: 'processes', risk_controls: 'processes',
  records_of_processing: 'processes',
  it_asset_register: 'registers', risk_register: 'registers', contact_register: 'registers', vendor_contracts: 'registers',
};

const ORDER: readonly Family[] = [...FAMILIES, 'installation', 'workspace', 'own', 'other'];

interface Offered {
  readonly key: string;
  readonly origin: 'HUBTASK' | 'INSTALLATION' | 'WORKSPACE' | 'OWN';
  readonly hub_document?: unknown;
}

export function familyOf(template: Offered): Family {
  if (template.origin !== 'HUBTASK') return template.origin.toLowerCase() as Family;
  return FAMILY_OF[template.key] ?? 'other';
}

/**
 * A template's mark on its card (user-experience.md §5.2): its own drawing, so thirty cards are
 * told apart at a glance; else its family's; one published here the catalogue's own mark.
 */
const MARK_OF: Readonly<Record<string, IconName>> = {
  household: 'house', shopping: 'shopping-cart', habits: 'repeat', travel: 'plane',
  kanban_team: 'columns-3', scrum_team: 'rocket', bug_tracking: 'circle-alert', content_calendar: 'calendar', event_planning: 'flag',
  classic_project: 'briefcase', hybrid_project: 'shapes', client_project: 'users',
  portfolio: 'folder', okr: 'gauge', scaled_agile: 'dependency',
  itil_service_desk: 'inbox', internal_it: 'wrench', facility_requests: 'hammer', hr_requests: 'user-check', customer_support: 'message-square',
  approvals: 'vote', onboarding: 'user-plus', procurement: 'wallet', capa: 'square-check', risk_controls: 'shield',
  records_of_processing: 'file-text',
  it_asset_register: 'container', risk_register: 'triangle-alert', contact_register: 'file-user', vendor_contracts: 'paperclip',
};

const FAMILY_MARK: Readonly<Partial<Record<Family, IconName>>> = {
  personal: 'user', teams: 'users', projects: 'briefcase', programmes: 'folder', service: 'inbox', processes: 'automation', registers: 'archive',
};

/** A family's mark on its card in the starter (user-experience.md §3.2). */
export function familyMark(family: Family): IconName {
  return FAMILY_MARK[family] ?? 'layout-template';
}

export function markOf(template: Offered): IconName {
  return (template.origin === 'HUBTASK' ? MARK_OF[template.key] : undefined) ?? FAMILY_MARK[familyOf(template)] ?? 'layout-template';
}

/** The templates with a hub part by family, each family in the gallery's order, an empty one left out. */
export function hubFamilies<T extends Offered>(templates: readonly T[]): { family: Family; templates: T[] }[] {
  const withHub = templates.filter((each) => each.hub_document != null);
  return ORDER.map((family) => ({ family, templates: withHub.filter((each) => familyOf(each) === family) }))
    .filter((group) => group.templates.length > 0);
}

/** The families the templates fall in, in the gallery's order: the catalogue's filter (§5.2). */
export function familiesOf(templates: readonly Offered[]): Family[] {
  return ORDER.filter((family) => templates.some((each) => familyOf(each) === family));
}

/** The products a template switches on, as module keys, workflows left out: every template has it. */
export function switchedOn(template: { readonly document?: { readonly modules?: Readonly<Record<string, { readonly switch?: string } | null>> | null } }): string[] {
  return Object.entries(template.document?.modules ?? {})
    .filter(([name, each]) => name !== 'workflows' && each?.switch === 'ON')
    .map(([name]) => name.toUpperCase());
}


/**
 * The hub's name once a template is chosen (§8.11): the template's name where the reader has typed
 * none or the name is still the one the last choice filled in; a name the reader typed stays.
 * Unchoosing (`next` empty) takes a filled-in name away again.
 */
export function prefilledName(current: string, filled: string, next: string): string {
  return current.trim() === '' || current === filled ? next : current;
}

/** The parts a template's *Preview* names, in the order a person meets them. */
export const CREATES = [
  'types', 'workflows', 'fields', 'roles', 'views', 'rules', 'sla_policies', 'business_calendars', 'reply_templates', 'template_pages', 'samples',
] as const;
export type Creates = (typeof CREATES)[number];

type Block = Readonly<Record<string, object | null>> | null | undefined;
type Document = Readonly<Record<string, Block>> | null | undefined;

/**
 * What taking a template creates, in words (user-experience.md §5.2, *Preview*): per part, the
 * names its blocks carry - from the workspace's part, the hub's and the collection's, each key
 * once - and a part it holds nothing of left out. A block without a name is named by its key.
 */
export function createsOf(template: {
  readonly document?: Document;
  readonly hub_document?: Document;
  readonly collection_document?: Document;
}): { readonly part: Creates; readonly names: readonly string[] }[] {
  const documents = [template.document, template.hub_document, template.collection_document];
  return CREATES.map((part) => {
    const named = new Map<string, string>();
    for (const document of documents) {
      for (const [key, block] of Object.entries(document?.[part] ?? {})) {
        if (block === null || named.has(key)) continue;
        const { name, title } = block as { readonly name?: unknown; readonly title?: unknown };
        named.set(key, typeof name === 'string' ? name : typeof title === 'string' ? title : key);
      }
    }
    return { part, names: [...named.values()] };
  }).filter((each) => each.names.length > 0);
}
