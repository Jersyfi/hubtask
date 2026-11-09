// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The scopes a token may carry, grouped by area for *New token* (page-system §9.5): each area is
 * one row whose choices are the verbs its scopes share (*Read*, *Write*), and a verb ticks every
 * scope of the area that carries it. The configuration's scopes are an area of their own, folded
 * on the page. The grouping is by a scope's resource, the part before its first colon; a resource
 * this list does not know lands in `other`, so a scope the server adds is still offered.
 */

/** The areas in the order the page draws them. */
export const AREAS = ['work', 'people', 'records', 'automation', 'workspace', 'other'] as const;
export type Area = (typeof AREAS)[number] | 'configuration';

const AREA_OF: Readonly<Record<string, Area>> = {
  items: 'work',
  containers: 'work',
  comments: 'work',
  recurrence: 'work',
  reminders: 'work',
  media: 'work',
  templates: 'work',
  trash: 'work',
  accounts: 'people',
  members: 'people',
  identity_provider: 'people',
  oauth: 'people',
  audit: 'records',
  privacy: 'records',
  retention: 'records',
  backup: 'records',
  automation: 'automation',
  agents: 'automation',
  agent: 'automation',
  jobs: 'automation',
  ai: 'automation',
  workspace: 'workspace',
  quotas: 'workspace',
  ops: 'workspace',
  config: 'configuration',
};

/** The area a scope belongs to. */
export function areaOf(scope: string): Area {
  return AREA_OF[scope.split(':')[0] ?? ''] ?? 'other';
}

/**
 * What a scope lets its holder do: the part after the resource (`read`, `write`, `manage`). A
 * configuration part (`config:apply:roles`) is the verb `apply` of the part it names, and is
 * offered one by one rather than under a verb.
 */
export function verbOf(scope: string): string {
  return scope.split(':')[1] ?? scope;
}

/** Whether a scope is one of the configuration's parts, offered one by one. */
export const isConfigurationPart = (scope: string): boolean => scope.startsWith('config:apply:');

export interface AreaRow {
  readonly area: Area;
  /** The verbs in a stable order (`read` first), each with the scopes it ticks. */
  readonly verbs: readonly { readonly verb: string; readonly scopes: readonly string[] }[];
}

const VERB_ORDER = ['read', 'write', 'manage', 'export', 'plan', 'apply', 'cancel', 'destructive'];
const rank = (verb: string): number => {
  const at = VERB_ORDER.indexOf(verb);
  return at === -1 ? VERB_ORDER.length : at;
};

/** The offered scopes as rows by area, the configuration's parts left out (`configurationParts`). */
export function areaRows(scopes: readonly string[]): AreaRow[] {
  const rows: AreaRow[] = [];
  for (const area of [...AREAS, 'configuration'] as const) {
    const inArea = scopes.filter((scope) => areaOf(scope) === area && !isConfigurationPart(scope));
    if (inArea.length === 0) continue;
    const verbs = [...new Set(inArea.map(verbOf))].sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
    rows.push({ area, verbs: verbs.map((verb) => ({ verb, scopes: inArea.filter((scope) => verbOf(scope) === verb) })) });
  }
  return rows;
}

/** The configuration's parts, each a scope of its own. */
export const configurationParts = (scopes: readonly string[]): string[] => scopes.filter(isConfigurationPart);

/** Whether every scope a verb ticks is chosen: what its box shows; some but not all is mixed. */
export function verbState(chosen: readonly string[], scopes: readonly string[]): 'all' | 'some' | 'none' {
  const held = scopes.filter((scope) => chosen.includes(scope)).length;
  return held === 0 ? 'none' : held === scopes.length ? 'all' : 'some';
}

/** The choice after a verb's box is pressed: all of its scopes on, or all of them off. */
export function toggleVerb(chosen: readonly string[], scopes: readonly string[], isOn: boolean): string[] {
  const rest = chosen.filter((scope) => !scopes.includes(scope));
  return isOn ? [...rest, ...scopes] : rest;
}
