// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where on an entry each AI question is asked and each proposal is shown (UC-AI-04 check 6).
 *
 * The pure half of the entry's AI: a subject - the title, the notes, the due date, the labels, the
 * steps, the discussion - has its own ✦, and a proposal stands under the subject it is about.
 * Nothing is drawn where nothing stands, so an entry shows no empty suggestion area. Every rule here
 * is a question with an answer a test can assert without a browser.
 *
 * **Offered only where the server would answer** (P-05). Asking about an entry is changing it -
 * what writing asks for - except for the two reading aids, a translation and a summary of the
 * discussion, which everybody who may read the entry is offered (check 8). An item that cannot be
 * asked right now stands with its reason inside the menu, which moves nothing on the page.
 */

import type { Suggestion, WorkItem } from '@hubtask/sync-engine';

import { isSummary, shapeOf, type Said, type Shape } from './suggestions.ts';

/** The subjects of an entry that have a ✦ of their own. */
export type Place = 'title' | 'notes' | 'due' | 'labels' | 'steps' | 'comments';

/**
 * Where a standing proposal is shown: under the one field it proposes, or - holding several, from
 * a rule, an agent or an ask of the release before - at the title, where the entry begins.
 */
export function placeOf(suggestion: Pick<Suggestion, 'kind' | 'payload' | 'dropped_nodes' | 'prompt_id'>, shape?: Shape): Place {
  const read = shape ?? shapeOf(suggestion, undefined);
  switch (read.shape) {
    case 'breakdown':
      return 'steps';
    case 'classification':
      return 'labels';
    case 'duplicates':
      return 'title';
    case 'fields': {
      if (suggestion.prompt_id === 'summarize-thread') return 'comments';
      if (isSummary(read)) return 'notes';
      const fields = read.proposals.map((proposal) => proposal.field);
      const [only] = fields;
      if (fields.length === 1 && only) return only === 'due_date' ? 'due' : only;
      return 'title';
    }
    default:
      return 'title';
  }
}

/** One question a ✦ asks. */
export type Ask =
  | 'suggest-title'
  | 'duplicates'
  | 'translate'
  | 'suggest-notes'
  | 'summarize'
  | 'suggest-due'
  | 'classify'
  | 'decompose'
  | 'summarize-thread';

/** What each question is called in its menu (voice-and-tone.md §2.1: the verb). */
export const ASK_CODE: Readonly<Record<Ask, string>> = {
  'suggest-title': 'app.ai.ask_title',
  duplicates: 'app.ai.ask_duplicates',
  translate: 'app.ai.ask_translate',
  'suggest-notes': 'app.ai.ask_notes',
  summarize: 'app.ai.ask_summary',
  'suggest-due': 'app.ai.ask_due',
  classify: 'app.ai.ask_labels',
  decompose: 'app.ai.ask_breakdown',
  'summarize-thread': 'app.ai.ask_discussion',
};

/** The questions of each place, in the order the menu offers them. */
const ASKS: Readonly<Record<Place, readonly Ask[]>> = {
  title: ['suggest-title', 'duplicates', 'translate'],
  notes: ['suggest-notes', 'summarize'],
  due: ['suggest-due'],
  labels: ['classify'],
  steps: ['decompose'],
  comments: ['summarize-thread'],
};

/** The reading aids: offered to everybody who may read the entry (check 8). */
const READING_AIDS: ReadonlySet<Ask> = new Set(['translate', 'summarize-thread']);

/** The field a single-field question asks for, as the contract names it (`AiFieldsAsk.fields`). */
export const ASKED_FIELD: Readonly<Partial<Record<Ask, 'title' | 'notes' | 'due_date'>>> = {
  'suggest-title': 'title',
  'suggest-notes': 'notes',
  'suggest-due': 'due_date',
};

/** How today's budget stands, as `GET /ai-budget-state` answers it. */
export interface BudgetState {
  readonly state: 'OK' | 'NEARLY_SPENT' | 'SPENT' | 'OFF';
  readonly renews_at?: string | null;
}

/** What the entry and the reader are, as far as the questions of a place depend on it. */
export interface PlaceContext {
  readonly features: Readonly<Record<string, unknown>> | undefined;
  readonly item: Pick<WorkItem, 'archived_at'>;
  /** The reader may change this entry (the role along its path, UC-WRK-22). */
  readonly mayChange: boolean;
  /** The discussion has at least one comment. */
  readonly hasComments: boolean;
  /** Whether a classification has anything to choose from (`hasChoices`). */
  readonly choices: boolean | 'reading';
  readonly budget?: BudgetState;
  /** The kinds of question that already stand as a proposal at this place, and which one. */
  readonly standing?: Readonly<Partial<Record<Ask, string>>>;
}

/** One item of a ✦ menu. */
export interface AskOffer {
  readonly ask: Ask;
  /** Why it cannot be asked now, rendered by the caller; absent where it can. */
  readonly reason?: Said;
  /** A proposal of this kind stands: the item shows it rather than asking again. */
  readonly standing?: string;
}

/**
 * The items of one place's ✦, or none: absent when the manifest announces no AI (ai-first.md §2,
 * absence is absence), and every question the reader's role would be refused left out (P-05).
 */
export function offersAt(place: Place, context: PlaceContext): readonly AskOffer[] {
  if (context.features?.ai_suggestions !== true) return [];
  const spent: Said | undefined =
    context.budget?.state === 'SPENT'
      ? { code: 'ai.unavailable', params: { reason: 'budget_spent', renews_at: context.budget.renews_at ?? '' } }
      : undefined;
  const offers: AskOffer[] = [];
  for (const ask of ASKS[place]) {
    const reads = READING_AIDS.has(ask);
    if (!reads && !context.mayChange) continue;
    if (ask === 'summarize-thread' && !context.hasComments) continue;
    const standing = context.standing?.[ask];
    if (standing) {
      offers.push({ ask, standing });
      continue;
    }
    offers.push({ ask, reason: spent ?? reasonOf(ask, context) });
  }
  return offers;
}

/** The reason an ask cannot be put now, apart from the budget. */
function reasonOf(ask: Ask, context: PlaceContext): Said | undefined {
  if (ask === 'duplicates' && context.features?.semantic_search !== true) return { code: 'app.suggestions.no_semantic_search' };
  if (READING_AIDS.has(ask)) return undefined;
  // An archived entry is not written to, so nothing that is accepted into it is asked.
  if (context.item.archived_at) return { code: 'app.entries.archived' };
  if (ask === 'classify' && context.choices !== true) {
    return { code: context.choices === 'reading' ? 'app.suggestions.choices_reading' : 'app.suggestions.no_choices' };
  }
  return undefined;
}

/** Which question a standing proposal answers, so the ✦ of its place can show it instead. */
export function askOf(suggestion: Pick<Suggestion, 'kind' | 'payload' | 'dropped_nodes' | 'prompt_id'>): Ask | undefined {
  const shape = shapeOf(suggestion, undefined);
  switch (shape.shape) {
    case 'breakdown':
      return 'decompose';
    case 'classification':
      return 'classify';
    case 'duplicates':
      return 'duplicates';
    case 'fields': {
      if (suggestion.prompt_id === 'summarize-thread') return 'summarize-thread';
      if (suggestion.prompt_id === 'summarize') return 'summarize';
      if (shape.proposals.length !== 1) return undefined;
      const field = shape.proposals[0]?.field;
      if (!field) return undefined;
      return field === 'title' ? 'suggest-title' : field === 'notes' ? 'suggest-notes' : 'suggest-due';
    }
    default:
      return undefined;
  }
}

/** Whether the note "nearly used up" belongs in every menu's head. */
export const isBudgetNearlySpent = (budget: BudgetState | undefined): boolean => budget?.state === 'NEARLY_SPENT';
