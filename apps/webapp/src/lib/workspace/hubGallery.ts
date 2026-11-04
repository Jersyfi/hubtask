// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The template gallery of *Create hub* (user-experience.md §5.2): the catalogue's templates a hub
 * can take, grouped by the families of configuration.md §9, after the presets as the *Simple*
 * family. The contract carries no family, so Hubtask's templates are placed by key; a template
 * published in the installation or the workspace stands under its origin.
 */

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

/** Where a hub made in the dialog opens: its *Products* section on the chosen template's plan (§8.11). */
export function afterCreating(id: string, template: string | undefined, otherwise: string): string {
  return template ? `/hubs/${encodeURIComponent(id)}/settings/modules?template=${encodeURIComponent(template)}` : otherwise;
}

/**
 * The hub's name once a template is chosen (§8.11): the template's name where the reader has typed
 * none or the name is still the one the last choice filled in; a name the reader typed stays.
 * Unchoosing (`next` empty) takes a filled-in name away again.
 */
export function prefilledName(current: string, filled: string, next: string): string {
  return current.trim() === '' || current === filled ? next : current;
}
