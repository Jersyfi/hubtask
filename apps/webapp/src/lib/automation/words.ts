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
import { FLOW_KINDS, isGroup, nameSeed, readNode, type Node, type Sentence } from './model.ts';

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
 * An event type read as its segments: `de.hubtask.work.item.overdue.v1` is "item overdue". The
 * fallback of `eventWords` for a type the verb table does not know, and what the type reads as
 * without its namespace and version.
 */
export function eventWord(type: string): string {
  return type.replace(/^de\.hubtask\./, '').replace(/\.v\d+$/, '').replace(/^work\./, '').replace(/[._]/g, ' ');
}

/** An event type, said in words (decision 13). */
export interface EventWords {
  /** The entity's segment: `item`, `container`, `rule_run` - what the group is keyed by. */
  entity: string;
  /** The heading the event sorts under: "Entries", "Hubs and collections". */
  group: string;
  /** The event as a clause, lower-case: "an entry is created". */
  clause: string;
  /** The same, capitalised, for an option or a card. */
  said: string;
}

/**
 * The words come from the type's own name: `de.hubtask.<area>.<entity>.<verb>.v1` gives the
 * entity and the verb, and the catalogue has a word for each - `app.flow.event_entity_item`,
 * `app.flow.event_verb_created` - so "an entry is created" is composed, never listed per type.
 * Nothing is compiled in that the manifest does not serve: an entity or a verb the catalogue has
 * no word for is said as its segment, never hidden, and sorts under "everything else".
 */
export function eventWords(words: Catalogue, type: string): EventWords {
  const segments = type.replace(/^de\.hubtask\./, '').replace(/\.v\d+$/, '').split('.');
  const verb = segments[segments.length - 1] ?? '';
  const entity = segments[segments.length - 2] ?? '';
  const entityCode = `app.flow.event_entity_${entity}`;
  const verbCode = `app.flow.event_verb_${verb}`;
  const groupCode = `app.flow.event_group_${entity}`;
  const known = words.has(entityCode) && words.has(verbCode);
  const clause = known ? `${words.t(entityCode)} ${words.t(verbCode)}` : eventWord(type);
  return {
    entity,
    group: words.has(groupCode) ? words.t(groupCode) : words.t('app.flow.event_group_other'),
    clause,
    said: clause.charAt(0).toUpperCase() + clause.slice(1),
  };
}

/** The order the groups are shown in: the entities a rule is most often about first. */
const EVENT_GROUP_ORDER = ['item', 'container', 'bucket', 'label', 'comment', 'attachment', 'entry', 'recurrence', 'template', 'rule_run'];

/** The manifest's event types arranged for a select: grouped by entity, said in words. */
export function eventGroups(words: Catalogue, types: readonly string[]): { label: string; options: { value: string; label: string }[] }[] {
  const byEntity = new Map<string, { label: string; options: { value: string; label: string }[] }>();
  for (const type of types) {
    const said = eventWords(words, type);
    const key = words.has(`app.flow.event_group_${said.entity}`) ? said.entity : '';
    const group = byEntity.get(key) ?? { label: said.group, options: [] };
    group.options.push({ value: type, label: said.said });
    byEntity.set(key, group);
  }
  const rank = (key: string): number => (key === '' ? EVENT_GROUP_ORDER.length : EVENT_GROUP_ORDER.indexOf(key) === -1 ? EVENT_GROUP_ORDER.length - 1 : EVENT_GROUP_ORDER.indexOf(key));
  return [...byEntity.entries()].sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b)).map(([, group]) => group);
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

/**
 * The manifest's kinds arranged for a menu: the common groups, then everything else sorted. The
 * palette shows the groups alone (decision 12) - ninety names of which `Confirm TOTP` is one are
 * not a palette - and the `+` menu's search is what finds the rest.
 */
export function grouped(served: readonly string[], rest: 'listed' | 'folded' = 'listed'): { code: string; kinds: string[] }[] {
  const have = new Set(served);
  const groups = COMMON_GROUPS.map((group) => ({ code: group.code, kinds: group.kinds.filter((kind) => have.has(kind)) })).filter(
    (group) => group.kinds.length > 0,
  );
  if (rest === 'folded') return groups;
  const placed = new Set(groups.flatMap((group) => group.kinds));
  const others = served.filter((kind) => !placed.has(kind)).sort();
  if (others.length > 0) groups.push({ code: 'app.flow.group_other', kinds: others });
  return groups;
}

/**
 * The icon a kind is drawn with (decision 12), in the palette, the `+` menu and on its card. One
 * table, so the three cannot disagree; a kind it does not name gets its group's, and a kind in
 * no group a plain mark. The names are the design system's declared set and nothing else.
 */
export type KindIconName =
  | 'tag' | 'calendar' | 'calendar-x' | 'square-check' | 'rotate-ccw' | 'arrow-right-left' | 'archive' | 'trash' | 'copy'
  | 'image' | 'sliders-horizontal' | 'plus' | 'user-plus' | 'user-minus' | 'users' | 'user' | 'columns-3' | 'folder-plus'
  | 'layout-template' | 'repeat' | 'skip-forward' | 'message-square' | 'inbox' | 'circle-x' | 'send' | 'globe' | 'sparkles'
  | 'pause' | 'git-branch' | 'square' | 'check';

const KIND_ICON: Readonly<Record<string, KindIconName>> = {
  ADD_LABEL: 'tag', REMOVE_LABEL: 'tag', SET_DUE_DATE: 'calendar', CLEAR_DUE_DATE: 'calendar-x',
  COMPLETE_WORK_ITEM: 'square-check', REOPEN_WORK_ITEM: 'rotate-ccw', MOVE_WORK_ITEM: 'arrow-right-left',
  ARCHIVE_WORK_ITEM: 'archive', TRASH_WORK_ITEM: 'trash', DUPLICATE_WORK_ITEM: 'copy', SET_COVER: 'image',
  SET_CUSTOM_FIELD: 'sliders-horizontal', CREATE_WORK_ITEM: 'plus',
  ASSIGN_WORK_ITEM: 'user-plus', AUTO_ASSIGN_WORK_ITEM: 'users', UNASSIGN_WORK_ITEM: 'user-minus',
  ADD_MEMBER: 'user-plus', REMOVE_MEMBER: 'user-minus',
  CREATE_BUCKET: 'columns-3', CREATE_CONTAINER: 'folder-plus', INSTANTIATE_TEMPLATE: 'layout-template',
  SET_RECURRENCE: 'repeat', SKIP_OCCURRENCE: 'skip-forward',
  ADD_COMMENT: 'message-square', CONVERT_JUMBLE_ENTRY: 'inbox', DISMISS_JUMBLE_ENTRY: 'circle-x',
  SEND_WEBHOOK: 'send', HTTP_REQUEST: 'globe',
  AI_CLASSIFY: 'sparkles', AI_SUMMARIZE: 'sparkles', AI_SUGGEST_FIELDS: 'sparkles',
  WAIT: 'pause', BRANCH: 'git-branch', STOP: 'square',
};

const GROUP_ICON: Readonly<Record<string, KindIconName>> = {
  'app.flow.group_entries': 'check', 'app.flow.group_people': 'user', 'app.flow.group_structure': 'folder-plus',
  'app.flow.group_content': 'message-square', 'app.flow.group_outbound': 'globe', 'app.flow.group_ai': 'sparkles',
};

/** The trigger kinds' icons, the same table on the card and in the palette. */
export const TRIGGER_ICONS: Readonly<Record<string, 'zap' | 'clock' | 'calendar' | 'globe' | 'hand' | 'inbox'>> = {
  EVENT: 'zap',
  SCHEDULE: 'clock',
  RELATIVE_DATE: 'calendar',
  INBOUND_WEBHOOK: 'globe',
  MANUAL: 'hand',
  JUMBLE_ENTRY: 'inbox',
};

export function kindIcon(kind: string): KindIconName {
  const own = KIND_ICON[kind];
  if (own) return own;
  const group = COMMON_GROUPS.find((each) => each.kinds.includes(kind));
  if (group) return GROUP_ICON[group.code] ?? 'check';
  if (kind.startsWith('AI_')) return 'sparkles';
  return 'check';
}

/**
 * The family a kind belongs to, which is what its colour says (decision 18): the same word in the
 * blocks list, the `+` popover and on the card, so the three cannot disagree. The flow kinds are
 * the engine's own; a kind in no group is *other*.
 */
export type KindFamily = 'entries' | 'people' | 'structure' | 'content' | 'outbound' | 'ai' | 'flow' | 'other';

const GROUP_FAMILY: Readonly<Record<string, KindFamily>> = {
  'app.flow.group_entries': 'entries', 'app.flow.group_people': 'people', 'app.flow.group_structure': 'structure',
  'app.flow.group_content': 'content', 'app.flow.group_outbound': 'outbound', 'app.flow.group_ai': 'ai',
};

export function kindFamily(kind: string): KindFamily {
  if ((FLOW_KINDS as readonly string[]).includes(kind)) return 'flow';
  const group = COMMON_GROUPS.find((each) => each.kinds.includes(kind));
  if (group) return GROUP_FAMILY[group.code] ?? 'other';
  if (kind.startsWith('AI_')) return 'ai';
  return 'other';
}

/**
 * How often each kind is used across the rules the client holds (decision 17): what the blocks
 * list's *Frequent* group is counted from, arms included. Nothing is asked of the server.
 */
export function usageOf(rules: readonly { actions: readonly { kind: string; params?: Record<string, unknown> }[] }[]): Map<string, number> {
  const counts = new Map<string, number>();
  const visit = (actions: readonly { kind: string; params?: Record<string, unknown> }[]): void => {
    for (const action of actions) {
      counts.set(action.kind, (counts.get(action.kind) ?? 0) + 1);
      for (const arm of ['then', 'else'] as const) {
        const inner = action.params?.[arm];
        if (Array.isArray(inner)) visit(inner as { kind: string; params?: Record<string, unknown> }[]);
      }
    }
  };
  for (const rule of rules) visit(rule.actions);
  return counts;
}

/** One condition, as words: the sentence's subject and value, or "the expression holds". */
export function conditionWords(words: Catalogue, names: Names, expr: string): string {
  const node = readNode(expr);
  if (!node) return words.t('app.flow.sentence_expression');
  return nodeWords(words, names, node);
}

/** A tree as one sentence: "the type is TASK and (a due date is set or the entry is archived)". */
export function nodeWords(words: Catalogue, names: Names, node: Node): string {
  if (!isGroup(node)) return sentenceWords(words, names, node);
  const parts = node.items.map((item) => (isGroup(item) ? `(${nodeWords(words, names, item)})` : nodeWords(words, names, item)));
  if (node.mode === 'none') return words.t('app.flow.sentence_none', { items: parts.join(words.t('app.flow.sentence_or')) });
  return parts.join(node.mode === 'all' ? words.t('app.flow.sentence_and') : words.t('app.flow.sentence_or'));
}

export function sentenceWords(words: Catalogue, names: Names, sentence: Sentence): string {
  const subject = words.t(`app.flow.in_sentence_subject_${sentence.subject}`);
  const op = words.t(`app.flow.op_${sentence.op}`);
  switch (sentence.subject) {
    case 'hour':
      return `${subject} ${op} ${words.t('app.flow.hour_range', { from: sentence.a ?? '', to: sentence.b ?? '' })}`;
    case 'field':
      return `${subject} „${sentence.a ?? ''}“ ${op} „${sentence.b ?? ''}“`;
    case 'title':
    case 'notes':
      return sentence.a !== undefined ? `${subject} ${op} „${sentence.a}“` : `${subject} ${op}`;
    case 'due':
      return sentence.op === 'within' ? `${subject} ${op} ${words.t('app.flow.days', { count: Number(sentence.a) || 0 })}` : `${subject} ${op}`;
    case 'depth':
      return `${subject} ${op} ${sentence.a ?? ''}`;
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
