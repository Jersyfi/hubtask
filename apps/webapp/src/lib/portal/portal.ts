// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The requester portal's arithmetic, without a screen: what a typed answer becomes for a question
 * of the catalogue, and how a goal of the request stands for the requester (UC-SVC-05).
 */

/** A question as the catalogue asks it: a custom field's key, kind and the values a choice offers. */
export interface Question {
  readonly key: string;
  readonly kind: string;
  readonly options: readonly string[];
}

/** The kinds the portal draws as one line of text; every other kind it draws by its own control. */
const TEXT_KINDS = new Set(['TEXT', 'URL', 'RICH_TEXT']);

/** How the portal draws a question. A kind it cannot answer (a person, a reference) is left out. */
export type Control = 'text' | 'number' | 'date' | 'choice' | 'choices' | 'yes-no';

export function controlOf(question: Question): Control | undefined {
  if (TEXT_KINDS.has(question.kind)) return 'text';
  switch (question.kind) {
    case 'NUMBER':
    case 'DURATION':
      return 'number';
    case 'DATE':
      return 'date';
    case 'SELECT':
      return 'choice';
    case 'MULTI_SELECT':
      return 'choices';
    case 'BOOL':
      return 'yes-no';
    default:
      return undefined;
  }
}

/** The questions the portal can draw, in the catalogue's order. */
export const askable = (questions: readonly Question[]): readonly Question[] => questions.filter((q) => controlOf(q) !== undefined);

/**
 * The answers as `custom_fields`: an unanswered question is left out, so the form's own default
 * or preset applies and a required one is refused by the server where it belongs. A number that
 * is not one is left out for the same reason.
 */
export function answersOf(questions: readonly Question[], typed: Readonly<Record<string, unknown>>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const question of questions) {
    const value = typed[question.key];
    switch (controlOf(question)) {
      case 'number': {
        const text = typeof value === 'string' ? value.trim().replace(',', '.') : '';
        const number = Number(text);
        if (text !== '' && Number.isFinite(number)) out[question.key] = number;
        break;
      }
      case 'choices':
        if (Array.isArray(value) && value.length > 0) out[question.key] = value.filter((v) => question.options.includes(v as string));
        break;
      case 'yes-no':
        if (typeof value === 'boolean') out[question.key] = value;
        break;
      case 'choice':
        if (typeof value === 'string' && question.options.includes(value)) out[question.key] = value;
        break;
      case 'text':
      case 'date':
        if (typeof value === 'string' && value.trim() !== '') out[question.key] = value.trim();
        break;
      default:
        break;
    }
  }
  return out;
}

/** A goal of the request as its requester reads it. */
export interface Goal {
  readonly standing: 'RUNNING' | 'PAUSED' | 'MET' | 'BREACHED';
  readonly due_at: string | null;
  readonly at_risk_at: string | null;
}

export type GoalTone = 'info' | 'success' | 'warning' | 'danger' | 'neutral';

/** The tone a goal is drawn in at a moment: a running goal past its warning is a warning. */
export function goalTone(goal: Goal, now: Date): GoalTone {
  switch (goal.standing) {
    case 'MET':
      return 'success';
    case 'BREACHED':
      return 'danger';
    case 'PAUSED':
      return 'neutral';
    default:
      return goal.at_risk_at !== null && Date.parse(goal.at_risk_at) <= now.getTime() ? 'warning' : 'info';
  }
}

/** The words typed so far that are worth asking articles for: nothing below three letters. */
export function deflectionText(title: string, notes: string): string {
  const text = `${title} ${notes}`.trim().replace(/\s+/gu, ' ');
  return text.length < 3 ? '' : text.slice(0, 500);
}
