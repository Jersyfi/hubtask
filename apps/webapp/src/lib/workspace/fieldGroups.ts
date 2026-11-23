// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a level carries, as the structure panel's field draws it (design-system.md §4): the
// capabilities in four groups, and the sentence the closed field says when nearly all are on.

import type { ChoiceGroup, ChoiceOption } from '@hubtask/design-system/components';

/** The four groups, in their order; a capability is a member of one. */
export const FIELD_GROUPS = {
  STATUS_TIME: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'RECURRENCE'],
  PEOPLE: ['ASSIGNMENT', 'MEMBERS'],
  CONTENT: ['NOTES', 'LABELS', 'COMMENTS', 'COVER', 'ATTACHMENTS', 'HISTORY'],
  EXTENSIONS: ['BUCKET', 'CUSTOM_FIELDS', 'AMOUNT'],
} as const satisfies Record<string, readonly string[]>;

export type FieldGroup = keyof typeof FIELD_GROUPS;

/**
 * The group of a capability. The list is the installation's manifest, not a fixed fifteen: one the
 * groups do not name is an extension.
 */
export function fieldGroupOf(capability: string): FieldGroup {
  for (const [group, members] of Object.entries(FIELD_GROUPS) as [FieldGroup, readonly string[]][]) {
    if (members.includes(capability)) return group;
  }
  return 'EXTENSIONS';
}

/** The options under their groups' names, in the groups' order; a group with none is left out. */
export function fieldGroups(options: readonly ChoiceOption[], nameOf: (group: FieldGroup) => string): ChoiceGroup[] {
  return (Object.keys(FIELD_GROUPS) as FieldGroup[])
    .map((group) => ({ label: nameOf(group), options: options.filter((option) => fieldGroupOf(option.value) === group) }))
    .filter((group) => group.options.length > 0);
}

type Translate = (code: string, params?: Record<string, string | number>) => string;

/**
 * The closed field's sentence: "All 15 fields" with all on, "All except Amount" with one or two
 * off, and `undefined` - the chips - otherwise (decision Q8: a sentence when nearly all are on).
 */
export function fieldsSummary(chosen: readonly ChoiceOption[], all: readonly ChoiceOption[], t: Translate, locale: string): string | undefined {
  const off = all.filter((option) => !chosen.some((each) => each.value === option.value));
  if (off.length === 0) return t('app.structure.fields_all', { count: all.length });
  if (off.length > 2) return undefined;
  const names = new Intl.ListFormat(locale, { type: 'conjunction' }).format(off.map((option) => option.label));
  return t('app.structure.fields_all_except', { names });
}
