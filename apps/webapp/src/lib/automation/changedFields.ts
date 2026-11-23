// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The fields a rule's update may wait for: the wire names the trigger carries, each with the code
 * of the name the product already gives that field elsewhere, so the rule reads as the entry does.
 */
export const CHANGED_FIELDS = {
  title: 'app.entry_forms.own_title',
  notes: 'app.entry_forms.own_notes',
  due_at: 'app.entry_forms.own_due_date',
  start_at: 'app.entry_forms.own_start_at',
  assignee_id: 'app.structure.field.ASSIGNMENT',
  bucket_id: 'app.structure.field.BUCKET',
  parent_id: 'app.qkey.parent',
  completed: 'app.structure.field.COMPLETION',
} as const;

export type ChangedField = keyof typeof CHANGED_FIELDS;
