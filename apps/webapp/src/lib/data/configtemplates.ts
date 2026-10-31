// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The *Templates* gallery's arithmetic (configuration.md §6): the four origins in the order
 * the gallery draws them, what a template offers the workspace - to take it, to update it, or
 * nothing because the version in force is the newest -, and which of an upgrade's changes are
 * taken before the person chooses: every one the workspace did not change itself, never a conflict.
 */

export type Origin = 'HUBTASK' | 'INSTALLATION' | 'WORKSPACE' | 'OWN';

export const ORIGINS: readonly Origin[] = ['HUBTASK', 'INSTALLATION', 'WORKSPACE', 'OWN'];

interface Offered {
  readonly origin: Origin;
  readonly taken_version?: number | null;
  readonly update_available: boolean;
}

/** The templates by origin, each origin in the gallery's order, an empty origin left out. */
export function byOrigin<T extends { readonly origin: Origin }>(templates: readonly T[]): { origin: Origin; templates: T[] }[] {
  return ORIGINS.map((origin) => ({ origin, templates: templates.filter((each) => each.origin === origin) }))
    .filter((group) => group.templates.length > 0);
}

/** What the gallery offers for a template. */
export function offerOf(template: Offered): 'TAKE' | 'UPDATE' | 'IN_USE' {
  if (template.taken_version === null || template.taken_version === undefined) return 'TAKE';
  return template.update_available ? 'UPDATE' : 'IN_USE';
}

/** The upgrade's changes taken before the person chooses: every TAKE, no CONFLICT. */
export function takenFirst(upgrade: readonly { readonly path: string; readonly state: 'TAKE' | 'CONFLICT' }[]): string[] {
  return upgrade.filter((change) => change.state === 'TAKE').map((change) => change.path);
}

interface Asked {
  readonly key: string;
  readonly default?: string | null;
}

/**
 * The answers a plan is asked with: each question's own answer, its default where the person left
 * it, and nothing for one left open with no default - the server then leaves out what needs it.
 */
export function answersOf(questions: readonly Asked[] | undefined, given: Readonly<Record<string, string>>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const question of questions ?? []) {
    const value = (given[question.key] ?? '').trim() || (question.default ?? '');
    if (value) out[question.key] = value;
  }
  return out;
}
