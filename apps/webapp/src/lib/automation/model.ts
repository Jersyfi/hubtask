// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The rule as the editor holds it (F8-04, `milestone-F8.md` decisions 1, 3, 4).
 *
 * Pure: no store, no rune, no sentence. What is here is the shape of a draft, the paths that
 * address one step of it, the edits that keep the paths honest, the compile and read-back of the
 * condition composer, and the pieces a generated name is built from. Everything a test can hold
 * without a browser lives here; the words are the view's, from the catalogue.
 *
 * **The rule model is a list with nested branches**, and the canvas draws exactly that. A step is
 * addressed by a path of indices through the arms — `2`, `2/then/0`, `2/else/1/then/0` — which is
 * the run log's address space; the server's field errors and the check's findings use the JSON
 * pointer of the same place, and `pointerOf` translates.
 */

import type { Rule, RuleAction, RuleDraft, RuleTrigger } from '../data/rules.svelte.ts';

/** One step of the chain. A `BRANCH` carries its condition in `params.condition` as the contract does. */
export interface Step {
  kind: string;
  params: Record<string, unknown>;
  then?: Step[];
  else?: Step[];
  /** Folded to one line on the canvas. The editor's own, never sent. */
  collapsed?: boolean;
}

/** What starts the rule, mutable while it is edited. */
export interface TriggerDraft {
  kind: string;
  event_type?: string;
  changed_fields?: string[];
  rrule?: string;
  timezone?: string;
  anchor?: string;
  offset?: string;
}

/** The rule while it is edited. `conditions` are the expressions, one string each. */
export interface Draft {
  name: string;
  scope: { type: string; id?: string };
  runAs: string;
  trigger: TriggerDraft;
  conditions: string[];
  actions: Step[];
  throttle: { maxRunsPerHour?: number; dedupeKeyExpr?: string };
  onError: string;
}

/** The three the contract calls the engine's own, which no catalogue lists. */
export const FLOW_KINDS = ['WAIT', 'BRANCH', 'STOP'] as const;

export const isFlow = (kind: string): boolean => (FLOW_KINDS as readonly string[]).includes(kind);

/** A fresh draft: a whole-workspace rule on the first event, nothing else decided. */
export function emptyDraft(eventType = ''): Draft {
  return {
    name: '',
    scope: { type: 'TENANT' },
    runAs: '',
    trigger: { kind: 'EVENT', event_type: eventType },
    conditions: [],
    actions: [],
    throttle: {},
    onError: 'STOP',
  };
}

function stepsFrom(actions: readonly RuleAction[] | undefined): Step[] {
  return (actions ?? []).map((action) => ({
    kind: action.kind,
    params: { ...(action.params ?? {}) },
    ...(action.kind === 'BRANCH' ? { then: stepsFrom(action.then), else: stepsFrom(action.else) } : {}),
  }));
}

/** A stored rule, opened for editing. */
export function fromRule(rule: Rule): Draft {
  return {
    name: rule.name,
    scope: { type: rule.scope.type, ...(rule.scope.id ? { id: rule.scope.id } : {}) },
    runAs: rule.run_as,
    trigger: {
      kind: rule.trigger.kind,
      ...(rule.trigger.event_type ? { event_type: rule.trigger.event_type } : {}),
      ...(rule.trigger.changed_fields ? { changed_fields: [...rule.trigger.changed_fields] } : {}),
      ...(rule.trigger.rrule ? { rrule: rule.trigger.rrule } : {}),
      ...(rule.trigger.timezone ? { timezone: rule.trigger.timezone } : {}),
      ...(rule.trigger.anchor ? { anchor: rule.trigger.anchor } : {}),
      ...(rule.trigger.offset ? { offset: rule.trigger.offset } : {}),
    },
    conditions: rule.conditions.map((condition) => condition.expr),
    actions: stepsFrom(rule.actions),
    throttle: {
      ...(rule.throttle?.max_runs_per_hour ? { maxRunsPerHour: rule.throttle.max_runs_per_hour } : {}),
      ...(rule.throttle?.dedupe_key_expr ? { dedupeKeyExpr: rule.throttle.dedupe_key_expr } : {}),
    },
    onError: rule.on_error,
  };
}

function actionsFrom(steps: readonly Step[]): RuleAction[] {
  return steps.map((step) => {
    const params = Object.fromEntries(
      Object.entries(step.params).filter(([, value]) => value !== '' && value !== undefined && value !== null),
    );
    return {
      kind: step.kind,
      ...(Object.keys(params).length > 0 ? { params } : {}),
      ...(step.kind === 'BRANCH' ? { then: actionsFrom(step.then ?? []), else: actionsFrom(step.else ?? []) } : {}),
    };
  });
}

/** What the store sends. Empty strings are omissions; a rule carries what its author decided. */
export function toRuleDraft(draft: Draft, name: string): RuleDraft {
  const t = draft.trigger;
  const sent: RuleTrigger =
    t.kind === 'EVENT'
      ? {
          kind: t.kind,
          ...(t.event_type ? { event_type: t.event_type } : {}),
          ...(t.changed_fields?.length ? { changed_fields: [...t.changed_fields] } : {}),
        }
      : t.kind === 'SCHEDULE'
        ? { kind: t.kind, ...(t.rrule ? { rrule: t.rrule } : {}), ...(t.timezone ? { timezone: t.timezone } : {}) }
        : t.kind === 'RELATIVE_DATE'
          ? { kind: t.kind, ...(t.anchor ? { anchor: t.anchor } : {}), ...(t.offset ? { offset: t.offset } : {}) }
          : { kind: t.kind };
  return {
    name,
    scope: { type: draft.scope.type, ...(draft.scope.id ? { id: draft.scope.id } : {}) },
    run_as: draft.runAs,
    trigger: sent,
    conditions: draft.conditions.map((expr) => expr.trim()).filter((expr) => expr !== '').map((expr) => ({ expr })),
    actions: actionsFrom(draft.actions),
    ...(draft.throttle.maxRunsPerHour || draft.throttle.dedupeKeyExpr
      ? {
          throttle: {
            ...(draft.throttle.maxRunsPerHour ? { max_runs_per_hour: draft.throttle.maxRunsPerHour } : {}),
            ...(draft.throttle.dedupeKeyExpr ? { dedupe_key_expr: draft.throttle.dedupeKeyExpr } : {}),
          },
        }
      : {}),
    on_error: draft.onError,
  };
}

/* ---------- Paths ---------- */

/** A step's address: indices through the arms, `2/then/0`. A list's address is the same without the last index. */
export type Path = string;

const parentOf = (path: Path): { list: string; index: number } => {
  const at = path.lastIndexOf('/');
  return at < 0 ? { list: '', index: Number(path) } : { list: path.slice(0, at), index: Number(path.slice(at + 1)) };
};

/** The list a path names: `''` is the chain, `2/then` a branch's arm. Undefined where nothing is. */
export function listAt(actions: readonly Step[], list: string): Step[] | undefined {
  if (list === '') return actions as Step[];
  const parts = list.split('/');
  let current: Step[] | undefined = actions as Step[];
  for (let at = 0; at < parts.length; at += 2) {
    const step: Step | undefined = current?.[Number(parts[at])];
    const arm = parts[at + 1];
    if (!step || (arm !== 'then' && arm !== 'else')) return undefined;
    current = step[arm];
  }
  return current;
}

export function stepAt(actions: readonly Step[], path: Path): Step | undefined {
  const { list, index } = parentOf(path);
  return listAt(actions, list)?.[index];
}

/** Every step, depth first, with its path. */
export function walk(actions: readonly Step[], visit: (step: Step, path: Path) => void, prefix = ''): void {
  actions.forEach((step, index) => {
    const path = prefix ? `${prefix}/${index}` : String(index);
    visit(step, path);
    if (step.kind === 'BRANCH') {
      walk(step.then ?? [], visit, `${path}/then`);
      walk(step.else ?? [], visit, `${path}/else`);
    }
  });
}

/** How many steps a list holds, arms included. */
export function countSteps(actions: readonly Step[]): number {
  let count = 0;
  walk(actions, () => {
    count += 1;
  });
  return count;
}

/** The nesting depth a list sits at: the chain is 0, a first-level arm 1. */
export const depthOf = (list: string): number => (list === '' ? 0 : list.split('/').length / 2);

/**
 * The server's address for a step's field: what a refusal's `field_errors[].path` and a finding's
 * `path` carry. A branch's arms are inside its `params`, so `2/then/0` is `/actions/2/params/then/0`.
 */
export function pointerOf(path: Path, field = ''): string {
  const pointer = `/actions/${path.split('/').map((part) => (part === 'then' || part === 'else' ? `params/${part}` : part)).join('/')}`;
  return field ? `${pointer}/${field}` : pointer;
}

/** The step's own path from the server's pointer, where the pointer names one. */
export function pathOf(pointer: string): Path | undefined {
  const match = /^\/actions\/(.+?)(?:\/(?:kind|params\/(?!then|else)[^/]+.*))?$/.exec(pointer);
  const inner = match?.[1];
  if (!inner) return undefined;
  return inner.replace(/\/params\/(then|else)/g, '/$1');
}

const clone = (steps: readonly Step[]): Step[] =>
  steps.map((step) => ({
    ...step,
    params: { ...step.params },
    ...(step.then ? { then: clone(step.then) } : {}),
    ...(step.else ? { else: clone(step.else) } : {}),
  }));

/** The chain with a step inserted at `index` of the list `list`. Always a copy. */
export function insertAt(actions: readonly Step[], list: string, index: number, step: Step): Step[] {
  const next = clone(actions);
  const target = listAt(next, list);
  if (!target) return next;
  target.splice(Math.max(0, Math.min(index, target.length)), 0, step);
  return next;
}

/** The chain without the step at `path`. */
export function removeAt(actions: readonly Step[], path: Path): Step[] {
  const next = clone(actions);
  const { list, index } = parentOf(path);
  listAt(next, list)?.splice(index, 1);
  return next;
}

/** The chain with the step at `path` replaced. */
export function replaceAt(actions: readonly Step[], path: Path, step: Step): Step[] {
  const next = clone(actions);
  const { list, index } = parentOf(path);
  const target = listAt(next, list);
  if (target && index < target.length) target[index] = step;
  return next;
}

/**
 * The chain with the step at `from` moved to `index` of `list`, or undefined where the move is
 * refused: a branch cannot enter its own arm, and a step cannot go where nothing is.
 */
export function moveStep(actions: readonly Step[], from: Path, list: string, index: number): Step[] | undefined {
  if (list === from || list.startsWith(`${from}/`)) return undefined;
  const step = stepAt(actions, from);
  if (!step || listAt(actions, list) === undefined) return undefined;
  const source = parentOf(from);
  let target = index;
  if (source.list === list && source.index < index) target -= 1;
  return insertAt(removeAt(actions, from), list, target, step);
}

/** A fresh step of a kind, with a branch's two empty arms. */
export function newStep(kind: string): Step {
  return kind === 'BRANCH' ? { kind, params: { condition: '' }, then: [], else: [] } : { kind, params: {} };
}

/* ---------- The generated name ---------- */

/** What a generated name is built from: the trigger, and the first two steps that are not branches. */
export interface NameSeed {
  trigger: TriggerDraft;
  kinds: string[];
  more: boolean;
}

export function nameSeed(draft: Draft): NameSeed {
  const kinds: string[] = [];
  walk(draft.actions, (step) => {
    if (step.kind !== 'BRANCH') kinds.push(step.kind);
  });
  return { trigger: draft.trigger, kinds: kinds.slice(0, 2), more: kinds.length > 2 };
}

/** A name equal to what the rule generates for itself counts as automatic and follows every edit (decision 4). */
export const isAutomatic = (name: string, generated: string): boolean => name.trim() === '' || name.trim() === generated;

/* ---------- The condition composer ---------- */

/**
 * A condition as a sentence: a bounded set of subjects with the operators each takes. Compiled
 * to the CEL the server stores, and read back from it by matching the shapes this compiles to -
 * an expression the composer did not write is shown as an expression (decision 3).
 *
 * The subjects are the fields the run's `item` document carries (`condition.ItemDocument`), the
 * actor, and the hour of `now`. Labels are not among them: the run's document has no `labels` key
 * today (#807), and a subject the run cannot answer would compile into a rule that fails.
 */
export type Subject = 'type' | 'completed' | 'due' | 'assignee' | 'bucket' | 'parent' | 'actor' | 'hour' | 'field';

export interface Sentence {
  subject: Subject;
  op: string;
  a?: string;
  b?: string;
}

/** The operators each subject takes, in the order a picker offers them. */
export const OPERATORS: Record<Subject, readonly string[]> = {
  type: ['is', 'is_not'],
  completed: ['yes', 'no'],
  due: ['has', 'lacks'],
  assignee: ['has', 'lacks', 'is'],
  bucket: ['is', 'is_not'],
  parent: ['has', 'lacks'],
  actor: ['is', 'is_not'],
  hour: ['between'],
  field: ['is', 'is_not'],
};

/** Whether the operator takes a value, and which. */
export function takes(subject: Subject, op: string): 'none' | 'value' | 'range' | 'key_value' {
  if (subject === 'hour') return 'range';
  if (subject === 'field') return 'key_value';
  if (op === 'has' || op === 'lacks' || op === 'yes' || op === 'no') return 'none';
  return 'value';
}

const quote = (value: string): string => `'${value.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`;
const unquote = (value: string | undefined): string => (value ?? '').replace(/\\'/g, "'").replace(/\\\\/g, '\\');

export function compileSentence(sentence: Sentence): string {
  const { subject, op, a = '', b = '' } = sentence;
  switch (subject) {
    case 'type':
      return `item.type ${op === 'is' ? '==' : '!='} ${quote(a)}`;
    case 'completed':
      return `item.completed == ${op === 'yes' ? 'true' : 'false'}`;
    case 'due':
      return op === 'has' ? 'has(item.due_at)' : '!has(item.due_at)';
    case 'parent':
      return op === 'has' ? 'has(item.parent_id)' : '!has(item.parent_id)';
    case 'assignee':
      if (op === 'has') return 'has(item.assignee_id)';
      if (op === 'lacks') return '!has(item.assignee_id)';
      return `item.assignee_id == ${quote(a)}`;
    case 'bucket':
      return `item.bucket_id ${op === 'is' ? '==' : '!='} ${quote(a)}`;
    case 'actor':
      return `actor.id ${op === 'is' ? '==' : '!='} ${quote(a)}`;
    case 'hour':
      return `now.getHours() >= ${Number(a) || 0} && now.getHours() < ${Number(b) || 0}`;
    case 'field':
      return `item.custom_fields[${quote(a)}] ${op === 'is' ? '==' : '!='} ${quote(b)}`;
  }
}

const QUOTED = "'((?:[^'\\\\]|\\\\.)*)'";

const SHAPES: readonly { pattern: RegExp; read: (m: RegExpExecArray) => Sentence }[] = [
  { pattern: new RegExp(`^item\\.type (==|!=) ${QUOTED}$`), read: (m) => ({ subject: 'type', op: m[1] === '==' ? 'is' : 'is_not', a: unquote(m[2]) }) },
  { pattern: /^item\.completed == (true|false)$/, read: (m) => ({ subject: 'completed', op: m[1] === 'true' ? 'yes' : 'no' }) },
  { pattern: /^(!?)has\(item\.due_at\)$/, read: (m) => ({ subject: 'due', op: m[1] ? 'lacks' : 'has' }) },
  { pattern: /^(!?)has\(item\.parent_id\)$/, read: (m) => ({ subject: 'parent', op: m[1] ? 'lacks' : 'has' }) },
  { pattern: /^(!?)has\(item\.assignee_id\)$/, read: (m) => ({ subject: 'assignee', op: m[1] ? 'lacks' : 'has' }) },
  { pattern: new RegExp(`^item\\.assignee_id == ${QUOTED}$`), read: (m) => ({ subject: 'assignee', op: 'is', a: unquote(m[1]) }) },
  { pattern: new RegExp(`^item\\.bucket_id (==|!=) ${QUOTED}$`), read: (m) => ({ subject: 'bucket', op: m[1] === '==' ? 'is' : 'is_not', a: unquote(m[2]) }) },
  { pattern: new RegExp(`^actor\\.id (==|!=) ${QUOTED}$`), read: (m) => ({ subject: 'actor', op: m[1] === '==' ? 'is' : 'is_not', a: unquote(m[2]) }) },
  { pattern: /^now\.getHours\(\) >= (\d+) && now\.getHours\(\) < (\d+)$/, read: (m) => ({ subject: 'hour', op: 'between', a: m[1], b: m[2] }) },
  { pattern: new RegExp(`^item\\.custom_fields\\[${QUOTED}\\] (==|!=) ${QUOTED}$`), read: (m) => ({ subject: 'field', op: m[2] === '==' ? 'is' : 'is_not', a: unquote(m[1]), b: unquote(m[3]) }) },
];

/** The sentence an expression is, where the composer could have written it; undefined otherwise. */
export function readSentence(expr: string): Sentence | undefined {
  const trimmed = expr.trim();
  for (const shape of SHAPES) {
    const match = shape.pattern.exec(trimmed);
    if (match) return shape.read(match);
  }
  return undefined;
}

/** A sentence's first shape, for a new condition. */
export const defaultSentence = (): Sentence => ({ subject: 'type', op: 'is', a: 'TASK' });
