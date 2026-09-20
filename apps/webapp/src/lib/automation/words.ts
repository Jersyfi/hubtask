// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The words of the rule flow: what a kind is called, how a rule reads as a sentence, and what
 * name it gives itself (F8-04, decision 4). Every word is a catalogue entry; what this module
 * decides is which entry, from what the rule holds.
 *
 * Nothing here is compiled in about what the manifest serves. A kind the catalogue has no word
 * for is shown by its own name, spelled the way a person reads it - which is what the old form
 * did for every kind, and what a new use case gets until somebody writes its word.
 */

import type { Draft, Step, TriggerDraft } from './model.ts';
import { nameSeed, readSentence, type Sentence } from './model.ts';

/** How the catalogue is asked: the renderer, and whether it has an entry. */
export interface Catalogue {
  t: (code: string, params?: Record<string, string | number>) => string;
  has: (code: string) => boolean;
}

/** What the words need from the workspace: the name of what an identifier refers to. */
export interface Names {
  scope: (scope: Draft['scope']) => string;
  account: (id: string) => string;
  event: (type: string) => string;
  bucket?: (id: string) => string | undefined;
  changedField?: (field: string) => string;
}

/**
 * An event type read as words: `de.hubtask.work.item.overdue.v1` is "item overdue". The catalogue
 * has no word per event type - the webhooks screen shows the type itself - so the type is read
 * without its namespace and version rather than shown as a namespace.
 */
export function eventWord(type: string): string {
  return type.replace(/^de\.hubtask\./, '').replace(/\.v\d+$/, '').replace(/^work\./, '').replace(/[._]/g, ' ');
}

/** A kind's word, or the kind read as words. */
export function kindWord(words: Catalogue, kind: string): string {
  const code = `app.flow.action.${kind.toLowerCase()}`;
  if (words.has(code)) return words.t(code);
  const spelled = kind.toLowerCase().replace(/_/g, ' ');
  return spelled.charAt(0).toUpperCase() + spelled.slice(1);
}

export function triggerWord(words: Catalogue, kind: string): string {
  const code = `app.rules.trigger_${kind.toLowerCase()}`;
  return words.has(code) ? words.t(code) : kind;
}

/**
 * The groups the palette shows the common actions under. A presentation grouping of what the
 * manifest serves, not a list of what exists: a kind that is in no group is still offered, under
 * "everything else", so an installation that serves one more loses nothing.
 */
export const COMMON_GROUPS: readonly { code: string; kinds: readonly string[] }[] = [
  {
    code: 'app.flow.group_entries',
    kinds: ['ADD_LABEL', 'REMOVE_LABEL', 'SET_DUE_DATE', 'CLEAR_DUE_DATE', 'COMPLETE_WORK_ITEM', 'REOPEN_WORK_ITEM', 'MOVE_WORK_ITEM', 'ARCHIVE_WORK_ITEM', 'TRASH_WORK_ITEM', 'DUPLICATE_WORK_ITEM', 'SET_COVER', 'SET_CUSTOM_FIELD', 'CREATE_WORK_ITEM'],
  },
  { code: 'app.flow.group_people', kinds: ['ASSIGN_WORK_ITEM', 'AUTO_ASSIGN_WORK_ITEM', 'UNASSIGN_WORK_ITEM', 'ADD_MEMBER', 'REMOVE_MEMBER'] },
  { code: 'app.flow.group_structure', kinds: ['CREATE_BUCKET', 'CREATE_CONTAINER', 'INSTANTIATE_TEMPLATE', 'SET_RECURRENCE', 'SKIP_OCCURRENCE'] },
  { code: 'app.flow.group_content', kinds: ['ADD_COMMENT', 'CONVERT_JUMBLE_ENTRY', 'DISMISS_JUMBLE_ENTRY'] },
  { code: 'app.flow.group_outbound', kinds: ['SEND_WEBHOOK', 'HTTP_REQUEST'] },
  { code: 'app.flow.group_ai', kinds: ['AI_CLASSIFY', 'AI_SUMMARIZE', 'AI_SUGGEST_FIELDS'] },
];

/** The manifest's kinds arranged for the palette: the common groups, then everything else sorted. */
export function grouped(served: readonly string[]): { code: string; kinds: string[] }[] {
  const have = new Set(served);
  const groups = COMMON_GROUPS.map((group) => ({ code: group.code, kinds: group.kinds.filter((kind) => have.has(kind)) })).filter(
    (group) => group.kinds.length > 0,
  );
  const placed = new Set(groups.flatMap((group) => group.kinds));
  const rest = served.filter((kind) => !placed.has(kind)).sort();
  if (rest.length > 0) groups.push({ code: 'app.flow.group_other', kinds: rest });
  return groups;
}

/** One condition, as words: the sentence's subject and value, or "the expression holds". */
export function conditionWords(words: Catalogue, names: Names, expr: string): string {
  const sentence = readSentence(expr);
  if (!sentence) return words.t('app.flow.sentence_expression');
  return sentenceWords(words, names, sentence);
}

export function sentenceWords(words: Catalogue, names: Names, sentence: Sentence): string {
  const subject = words.t(`app.flow.in_sentence_subject_${sentence.subject}`);
  const op = words.t(`app.flow.op_${sentence.op}`);
  switch (sentence.subject) {
    case 'hour':
      return `${subject} ${op} ${words.t('app.flow.hour_range', { from: sentence.a ?? '', to: sentence.b ?? '' })}`;
    case 'field':
      return `${subject} „${sentence.a ?? ''}“ ${op} „${sentence.b ?? ''}“`;
    case 'assignee':
    case 'actor':
      return sentence.a ? `${subject} ${op} ${names.account(sentence.a)}` : `${subject} ${op}`;
    case 'bucket':
      return `${subject} ${op} ${names.bucket?.(sentence.a ?? '') ?? sentence.a ?? ''}`;
    case 'type':
      return `${subject} ${op} ${sentence.a ?? ''}`;
    default:
      return `${subject} ${op}`;
  }
}

function triggerSentence(words: Catalogue, names: Names, trigger: TriggerDraft): string {
  switch (trigger.kind) {
    case 'EVENT': {
      const event = names.event(trigger.event_type ?? '');
      if (trigger.changed_fields?.length) {
        return words.t('app.flow.sentence_trigger_event_fields', {
          event,
          fields: trigger.changed_fields.map((field) => names.changedField?.(field) ?? field).join(', '),
        });
      }
      return words.t('app.flow.sentence_trigger_event', { event });
    }
    case 'SCHEDULE':
      return words.t('app.flow.sentence_trigger_schedule', { rrule: trigger.rrule ?? '' });
    case 'RELATIVE_DATE':
      return words.t('app.flow.sentence_trigger_relative_date', {
        offset: trigger.offset ?? '',
        anchor: words.t(`app.flow.anchor_${(trigger.anchor ?? 'DUE_DATE').toLowerCase()}`),
      });
    default:
      return words.t(`app.flow.sentence_trigger_${trigger.kind.toLowerCase()}`);
  }
}

function stepsWords(words: Catalogue, names: Names, steps: readonly Step[]): string {
  return steps
    .map((step) =>
      step.kind === 'BRANCH'
        ? words.t('app.flow.sentence_branch', {
            condition: conditionWords(words, names, String(step.params.condition ?? '')),
            then: stepsWords(words, names, step.then ?? []) || words.t('app.flow.sentence_nothing'),
            else: stepsWords(words, names, step.else ?? []) || words.t('app.flow.sentence_nothing'),
          })
        : kindWord(words, step.kind).toLowerCase(),
    )
    .join(words.t('app.flow.sentence_then_sep'));
}

/** The whole rule as one sentence: the second view of the same rule (decision 4). */
export function sentence(words: Catalogue, names: Names, draft: Draft): string {
  const conditions = draft.conditions.filter((expr) => expr.trim() !== '');
  return words.t('app.flow.sentence', {
    when: words.t('app.flow.sentence_when'),
    trigger: triggerSentence(words, names, draft.trigger),
    scope: names.scope(draft.scope),
    conditions:
      conditions.length > 0
        ? words.t('app.flow.sentence_only_when', {
            conditions: conditions.map((expr) => conditionWords(words, names, expr)).join(words.t('app.flow.sentence_and')),
          })
        : '',
    then: words.t('app.flow.sentence_then'),
    steps: stepsWords(words, names, draft.actions) || words.t('app.flow.sentence_nothing'),
    account: names.account(draft.runAs),
  });
}

/** The name a rule gives itself: the trigger and its first two steps. */
export function generatedName(words: Catalogue, names: Names, draft: Draft): string {
  const seed = nameSeed(draft);
  let trigger: string;
  switch (seed.trigger.kind) {
    case 'EVENT':
      trigger = words.t('app.flow.name_trigger_event', { event: names.event(seed.trigger.event_type ?? '').toLowerCase() });
      break;
    case 'RELATIVE_DATE':
      trigger = words.t('app.flow.name_trigger_relative_date', {
        offset: seed.trigger.offset ?? '',
        anchor: words.t(`app.flow.anchor_${(seed.trigger.anchor ?? 'DUE_DATE').toLowerCase()}`),
      });
      break;
    default:
      trigger = words.t(`app.flow.name_trigger_${seed.trigger.kind.toLowerCase()}`);
  }
  const steps = seed.kinds.map((kind) => kindWord(words, kind).toLowerCase()).join(', ');
  if (seed.kinds.length === 0) return words.t('app.flow.generated_name_empty', { trigger });
  return words.t(seed.more ? 'app.flow.generated_name_more' : 'app.flow.generated_name', { trigger, steps });
}
