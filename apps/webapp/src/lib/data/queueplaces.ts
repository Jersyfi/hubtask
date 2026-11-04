// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where each entity a change can be queued for is shown (offline-sync.md §4.4, user-experience.md
 * §5.11): the replica's collection its name is read from, and the screen a queued or refused
 * change of it opens. A `Record` over every queueable entity, so a new one does not typecheck
 * until it has its place; its words are `app.sync.what.entity.<entity>.write` and `.delete`.
 */

import type { RefusedField, VocabularyEntity } from '@hubtask/sync-engine';

export interface EntityPlace {
  /** The replica's collection the entity's name is read from; none for one shown by its entry. */
  readonly collection?: string;
  /** The screen it opens; `undefined` for a link, which opens the entry it is drawn from. */
  readonly href?: string;
}

export const ENTITY_PLACES: Readonly<Record<VocabularyEntity, EntityPlace>> = {
  entry_type: { collection: 'entry_types', href: '/administration/types' },
  link_type: { collection: 'link_types', href: '/administration/link-types' },
  workflow: { collection: 'workflows', href: '/administration/workflows' },
  custom_role: { collection: 'custom_roles', href: '/administration/people?tab=permissions' },
  entry_link: {},
  time_timer: { href: '/time' },
};

/** Every queueable entity, for the catalogue's test and nothing else. */
export const QUEUEABLE_ENTITIES = Object.keys(ENTITY_PLACES) as VocabularyEntity[];

/** The fields of a refused change a person can edit in its draft: its text, by field. */
export function draftFields(local: unknown): Record<string, string> {
  if (!local || typeof local !== 'object' || Array.isArray(local)) return {};
  const fields: Record<string, string> = {};
  for (const [field, value] of Object.entries(local as Record<string, unknown>)) {
    if (typeof value === 'string' && ['title', 'notes', 'name', 'description', 'body'].includes(field)) fields[field] = value;
  }
  return fields;
}

/** One draft field's reason, as a message code and its values. */
export interface FieldRefusal {
  readonly code: string;
  readonly params: Readonly<Record<string, string>>;
}

/**
 * The draft fields a refusal names, keyed by field: a JSON Pointer of one segment (`/title`) is a
 * field of the change's values; a deeper or empty one is about something a draft does not show and
 * stays with the refusal's own reason.
 */
export function fieldRefusals(fields: readonly RefusedField[] | undefined): Record<string, FieldRefusal> {
  const refusals: Record<string, FieldRefusal> = {};
  for (const field of fields ?? []) {
    const segments = field.path.split('/');
    if (segments.length !== 2 || segments[0] !== '' || segments[1] === '') continue;
    const name = segments[1]!.replaceAll('~1', '/').replaceAll('~0', '~');
    refusals[name] ??= { code: field.code, params: field.params ?? {} };
  }
  return refusals;
}
