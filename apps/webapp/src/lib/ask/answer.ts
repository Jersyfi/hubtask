// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * An answer to a question about the workspace (UC-AI-08), read off its suggestion, and the filter it
 * implies as chips a person changes and saves as a view.
 *
 * What the server recorded is already narrowed to what the model was shown: an entry it cites, a
 * place, a person and a label are identifiers the asker could read when they asked. The chips are
 * the person's from here on - each removable, the two closed ones changeable - and *Save as view*
 * saves what they hold, never what the model said.
 */

import type { Suggestion } from '@hubtask/sync-engine';

import type { Condition } from '../data/query.ts';

/** The spans a due-date chip may hold, as the answer names them. */
export const DUE_SPANS = ['overdue', 'today', 'next_7_days', 'this_month'] as const;
export type DueSpan = (typeof DUE_SPANS)[number];

/** One answer, as the person reads it. */
export interface Answer {
  readonly text: string;
  readonly sourceIds: readonly string[];
  readonly chips: Chips;
}

/** The filter an answer implies, one optional part per chip. */
export interface Chips {
  readonly place?: { readonly id: string; readonly type: 'HUB' | 'COLLECTION' };
  readonly isCompleted?: boolean;
  readonly due?: DueSpan;
  readonly assigneeId?: string;
  readonly labelIds?: readonly string[];
}

/** The answer a suggestion holds, or nothing where it is not one. */
export function answerOf(suggestion: Pick<Suggestion, 'kind' | 'payload'>): Answer | undefined {
  if (suggestion.kind !== 'ANSWER') return undefined;
  const payload = (suggestion.payload ?? {}) as Record<string, unknown>;
  const text = typeof payload['answer'] === 'string' ? payload['answer'] : '';
  if (text === '') return undefined;
  const sources = Array.isArray(payload['source_ids']) ? payload['source_ids'] : [];
  return {
    text,
    sourceIds: sources.filter((id): id is string => typeof id === 'string'),
    chips: chipsOf(payload['filter']),
  };
}

/** The chips of a recorded filter; a part the server would not have kept is not drawn. */
export function chipsOf(filter: unknown): Chips {
  if (!filter || typeof filter !== 'object') return {};
  const f = filter as Record<string, unknown>;
  const placeType = f['place_type'];
  const labels = Array.isArray(f['label_ids']) ? f['label_ids'].filter((id): id is string => typeof id === 'string') : [];
  return {
    ...(typeof f['place_id'] === 'string' && (placeType === 'HUB' || placeType === 'COLLECTION')
      ? { place: { id: f['place_id'], type: placeType } }
      : {}),
    ...(typeof f['is_completed'] === 'boolean' ? { isCompleted: f['is_completed'] } : {}),
    ...(DUE_SPANS.includes(f['due'] as DueSpan) ? { due: f['due'] as DueSpan } : {}),
    ...(typeof f['assignee_id'] === 'string' ? { assigneeId: f['assignee_id'] } : {}),
    ...(labels.length > 0 ? { labelIds: labels } : {}),
  };
}

/** Whether the answer implies a filter at all: where it does not, no chip and no *Save as view*. */
export const hasChips = (chips: Chips): boolean => Object.keys(chips).length > 0;

/** The chips without one part: the person removed it. */
export function without(chips: Chips, part: keyof Chips): Chips {
  const { [part]: _removed, ...rest } = chips;
  return rest;
}

/**
 * The conditions the chips stand for, in the item query's grammar. A span is resolved on the server
 * against the reader's today (`@today`), so a view saved this week means the same next week.
 */
export function conditionsOf(chips: Chips): Condition[] {
  const conditions: Condition[] = [];
  if (chips.isCompleted !== undefined) {
    conditions.push({ field: 'is_completed', op: 'EQ', value: String(chips.isCompleted) });
  }
  switch (chips.due) {
    case 'overdue':
      conditions.push({ field: 'due_at', op: 'LT', value: '@today' });
      break;
    case 'today':
      conditions.push({ field: 'due_at', op: 'GTE', value: '@today' });
      conditions.push({ field: 'due_at', op: 'LT', value: '@today+P1D' });
      break;
    case 'next_7_days':
      conditions.push({ field: 'due_at', op: 'LTE', value: '@today+P7D' });
      break;
    case 'this_month':
      conditions.push({ field: 'due_at', op: 'LTE', value: '@end_of_month' });
      break;
  }
  if (chips.assigneeId) conditions.push({ field: 'assignee_id', op: 'EQ', value: chips.assigneeId });
  if (chips.labelIds && chips.labelIds.length > 0) {
    conditions.push({ field: 'labels', op: 'CONTAINS_ANY', value: chips.labelIds.join(',') });
  }
  return conditions;
}

/**
 * The newest answer that arrived since the question was asked: only the asker reads their
 * listing, so that is the one this question produced.
 */
export function answerSince<T extends Pick<Suggestion, 'kind' | 'status' | 'created_at'>>(
  suggestions: readonly T[],
  askedAt: string,
): T | undefined {
  const asked = Date.parse(askedAt);
  return suggestions
    .filter((each) => each.kind === 'ANSWER' && each.status === 'PROPOSED' && Date.parse(each.created_at) >= asked)
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))[0];
}

/**
 * The query a saved view holds for these chips, or nothing without a place: a view is anchored at a
 * hub or a collection, so a place chip is what saving needs (design-system.md §4, case 3). A hub
 * reaches the entries of its collections.
 */
export function viewQueryOf(chips: Chips, filter: unknown): Record<string, unknown> | undefined {
  if (!chips.place) return undefined;
  return {
    scope_container_id: chips.place.id,
    include_descendants: chips.place.type === 'HUB',
    ...(filter ? { filter } : {}),
  };
}

/**
 * Whether the palette offers *Ask* for these words: only for typed words, and only where the
 * workspace has consented to AI - without it nothing about asking is drawn (UC-AI-08 check 1).
 */
export function offersAsk(features: { readonly ai_suggestions?: boolean } | undefined, words: string): boolean {
  return features?.ai_suggestions === true && words.trim() !== '';
}
