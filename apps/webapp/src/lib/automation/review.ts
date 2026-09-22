// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What is missing from the draft, said before the probe is pressed (F8-26, `milestone-F8.md`
 * decision 29).
 *
 * ADR-0060's check runs on a *stored* rule against what exists in the workspace - a label that was
 * deleted, an action kind a later version no longer serves - and it cannot speak about the draft
 * under the hands, which is where writing a rule actually goes wrong: an event trigger with no
 * event, an action whose required parameter is empty, a branch with two empty arms, a schedule
 * feeding a step that needs the entry no schedule has.
 *
 * This reads the draft against the manifest the client already holds and answers notes at the same
 * cards the check's findings use. Pure: no store, no words, no request. The codes are the
 * client's own (`app.flow.review_*`) because it is the draft they are about and no server was
 * asked; where a server finding says the same thing at the same card, the finding wins.
 *
 * **Nothing here refuses anything.** The probe still runs and the rule still saves; the review
 * says what will happen, it does not decide.
 */

import { walk, type Draft, type Step } from './model.ts';
import { SUPPLIED } from './words.ts';

/** One thing the draft is missing, at the card it is about. */
export interface Note {
  /** `broken`: it cannot run as it stands. `attention`: it runs and probably not as meant. */
  readonly level: 'broken' | 'attention';
  /** The card: `trigger`, `run_as`, `gate`, `conditions/1`, a step's path, or `''` for the rule itself. */
  readonly card: string;
  readonly code: string;
  readonly params?: Record<string, string | number>;
}

/** A field of an action kind, as the manifest declares it (F8-01, F8-15). */
export interface ReviewField {
  readonly name: string;
  readonly required: boolean;
  /** False for the caller's plumbing, which a rule never carries (F8-15). */
  readonly rule?: boolean;
}

/** What the view knows beside the draft: whether an inbound rule has an address yet. */
export interface ReviewContext {
  readonly hasInboundAddress?: boolean;
}

/** Whether a parameter carries something the run can use. */
function carried(step: Step, name: string): boolean {
  const value = step.params[name];
  if (value === undefined || value === null) return false;
  if (typeof value === 'string') return value.trim() !== '';
  if (Array.isArray(value)) return value.length > 0;
  return true;
}

/** Whether the trigger can bring the entry a step acts on: a schedule never can. */
const bringsEntry = (kind: string): boolean => kind !== 'SCHEDULE';

/**
 * The draft read against the manifest: every note, in the order the canvas draws the cards, so
 * that a list of them reads down the rule.
 */
export function review(
  draft: Draft,
  fields: Readonly<Record<string, readonly ReviewField[]>>,
  context: ReviewContext = {},
): Note[] {
  const notes: Note[] = [];

  switch (draft.trigger.kind) {
    case 'EVENT':
      if (!draft.trigger.event_type) notes.push({ level: 'broken', card: 'trigger', code: 'app.flow.review_event_missing' });
      break;
    case 'SCHEDULE':
      if (!draft.trigger.rrule?.trim()) notes.push({ level: 'broken', card: 'trigger', code: 'app.flow.review_schedule_missing' });
      if (!draft.trigger.timezone?.trim()) notes.push({ level: 'attention', card: 'trigger', code: 'app.flow.review_timezone_missing' });
      break;
    case 'RELATIVE_DATE':
      if (!draft.trigger.offset?.trim()) notes.push({ level: 'broken', card: 'trigger', code: 'app.flow.review_offset_missing' });
      break;
    case 'INBOUND_WEBHOOK':
      // An inbound rule with no address minted is a rule nothing can reach.
      if (context.hasInboundAddress === false) notes.push({ level: 'attention', card: 'trigger', code: 'app.flow.review_address_missing' });
      break;
    default:
      break;
  }

  if (!draft.runAs) notes.push({ level: 'broken', card: 'run_as', code: 'app.flow.review_runner_missing' });

  draft.conditions.forEach((expr, index) => {
    if (expr.trim() === '') notes.push({ level: 'broken', card: `conditions/${index}`, code: 'app.flow.review_condition_empty' });
  });

  if (draft.actions.length === 0) {
    notes.push({ level: 'attention', card: '', code: 'app.flow.review_no_steps' });
  }

  walk(draft.actions, (step, path) => {
    if (step.kind === 'BRANCH') {
      if (String(step.params.condition ?? '').trim() === '') {
        notes.push({ level: 'broken', card: path, code: 'app.flow.review_branch_condition_empty' });
      }
      if ((step.then ?? []).length === 0 && (step.else ?? []).length === 0) {
        notes.push({ level: 'attention', card: path, code: 'app.flow.review_branch_empty' });
      }
      return;
    }
    if (step.kind === 'WAIT') {
      if (String(step.params.duration ?? '').trim() === '') {
        notes.push({ level: 'broken', card: path, code: 'app.flow.review_parameter_missing', params: { kind: step.kind, parameter: 'duration' } });
      }
      return;
    }
    if (step.kind === 'STOP') return;

    for (const field of fields[step.kind] ?? []) {
      if (!field.required || field.rule === false || carried(step, field.name)) continue;
      if (SUPPLIED.has(field.name)) {
        // The run supplies it - unless the trigger brings no entry to supply it from, and then
        // the step fails every time it is reached however the rule is written.
        if (!bringsEntry(draft.trigger.kind)) {
          notes.push({ level: 'broken', card: path, code: 'app.flow.review_needs_entry', params: { kind: step.kind } });
        }
        continue;
      }
      notes.push({ level: 'attention', card: path, code: 'app.flow.review_parameter_missing', params: { kind: step.kind, parameter: field.name } });
    }
  });

  return notes;
}

/** Whether anything in the review says the rule cannot run as it stands. */
export const cannotRun = (notes: readonly Note[]): boolean => notes.some((note) => note.level === 'broken');
