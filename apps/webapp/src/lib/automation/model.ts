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

/**
 * A branch's arms live inside its `params` - `then` and `else` are what the kind *takes*, beside
 * `condition` - which is where the domain reads them and where a finding's path points
 * (`/actions/2/params/then/0/kind`). The canvas keeps them as `Step.then` / `Step.else` for its
 * own paths, and this is the one place the two shapes meet (issue 853).
 */
function armFrom(params: Record<string, unknown> | undefined, arm: 'then' | 'else'): RuleAction[] {
  const rows = params?.[arm];
  return Array.isArray(rows) ? (rows as RuleAction[]) : [];
}

function stepsFrom(actions: readonly RuleAction[] | undefined): Step[] {
  return (actions ?? []).map((action) => {
    const params = Object.fromEntries(Object.entries(action.params ?? {}).filter(([key]) => key !== 'then' && key !== 'else'));
    return {
      kind: action.kind,
      params,
      ...(action.kind === 'BRANCH' ? { then: stepsFrom(armFrom(action.params, 'then')), else: stepsFrom(armFrom(action.params, 'else')) } : {}),
    };
  });
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
    const params: Record<string, unknown> = Object.fromEntries(
      Object.entries(step.params).filter(([, value]) => value !== '' && value !== undefined && value !== null),
    );
    if (step.kind === 'BRANCH') {
      params.then = actionsFrom(step.then ?? []);
      params.else = actionsFrom(step.else ?? []);
    }
    return {
      kind: step.kind,
      ...(Object.keys(params).length > 0 ? { params } : {}),
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

export const parentOf = (path: Path): { list: string; index: number } => {
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
  const without = removeAt(actions, from);
  if (!canPlace(without, list, target, step.kind)) return undefined;
  return insertAt(without, list, target, step);
}

/** The chain with the step at `path` moved one place up or down inside its own list; unchanged at the end. */
export function nudge(actions: readonly Step[], path: Path, direction: -1 | 1): Step[] {
  const { list, index } = parentOf(path);
  const siblings = listAt(actions, list);
  if (!siblings) return clone(actions);
  const target = index + direction;
  if (target < 0 || target >= siblings.length) return clone(actions);
  const step = siblings[index];
  // What ends the run stays the terminus (decision 19): it does not move up past a step, and no
  // step moves down past it.
  if (endsAllPaths(step) || endsAllPaths(siblings[target])) return clone(actions);
  return moveStep(actions, path, list, direction > 0 ? target + 1 : target) ?? clone(actions);
}

/**
 * Whether every path through a step ends the run (decision 19): *End the run* does; a branch does
 * when each of its arms does - every rung of a ladder and the else. Nothing may follow such a
 * step in its list, and the canvas draws the list's end right there.
 */
export function endsAllPaths(step: Step | undefined): boolean {
  if (!step) return false;
  if (step.kind === 'STOP') return true;
  if (step.kind !== 'BRANCH') return false;
  return endsRun(step.then ?? []) && endsRun(step.else ?? []);
}

/** Whether a list ends the run on every path: its last step does. */
export const endsRun = (list: readonly Step[]): boolean => list.length > 0 && endsAllPaths(list[list.length - 1]);

/**
 * Whether a step of `kind` may take the gap at `index` of `list` (decision 19): *End the run*
 * only as the last step of an arm, once - the chain's end ends the run anyway - and nothing
 * after a step that ends the run on every path, because the run would never reach it and the
 * canvas cannot draw "never" honestly. A list the chain does not have takes nothing.
 */
export function canPlace(actions: readonly Step[], list: string, index: number, kind: string): boolean {
  const target = listAt(actions, list);
  if (!target) return false;
  const at = Math.max(0, Math.min(index, target.length));
  const ended = endsRun(target);
  if (kind === 'STOP') return list !== '' && at === target.length && !ended;
  return !(ended && at === target.length);
}

/** The first index of a list a run never reaches - the step after one that ends every path - or -1 for none. */
export function unreachableFrom(steps: readonly Step[]): number {
  const end = steps.findIndex((step) => endsAllPaths(step));
  return end === -1 || end === steps.length - 1 ? -1 : end + 1;
}

/* ---------- The ladder: if / else if / else ---------- */

/** A rung: an else arm whose only step is a branch (decision 19). The reader and the canvas know the shape alike. */
export const isRung = (step: Step | undefined): boolean => step?.kind === 'BRANCH' && (step.else?.length ?? 0) === 1 && step.else?.[0]?.kind === 'BRANCH';

/**
 * The rungs of the ladder that starts at `path`: the branch itself, then every branch that is the
 * sole step of the previous one's else arm, with their paths. A plain branch is a ladder of one.
 */
export function rungsOf(step: Step, path: Path): { step: Step; path: Path }[] {
  const rungs = [{ step, path }];
  let current = step;
  let at = path;
  while (isRung(current)) {
    at = `${at}/else/0`;
    current = current.else![0]!;
    rungs.push({ step: current, path: at });
  }
  return rungs;
}

/**
 * The chain with an *else if* added under the ladder at `path` (decision 19): a fresh branch
 * becomes the sole step of the last rung's else arm, and whatever that arm held becomes the new
 * rung's else - the steps keep their place as the last resort, and the engine runs the shape
 * today. Unchanged where `path` is not a branch.
 */
export function addRung(actions: readonly Step[], path: Path): Step[] {
  const step = stepAt(actions, path);
  if (!step || step.kind !== 'BRANCH') return clone(actions);
  const last = rungsOf(step, path).at(-1)!;
  const rung = newStep('BRANCH');
  rung.else = last.step.else ?? [];
  return replaceAt(actions, last.path, { ...last.step, else: [rung] });
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
 * today (issue 807), and a subject the run cannot answer would compile into a rule that fails.
 */
export type Subject =
  | 'type' | 'title' | 'notes' | 'completed' | 'archived' | 'due' | 'assignee' | 'bucket' | 'parent' | 'depth' | 'actor' | 'hour' | 'field';

export interface Sentence {
  subject: Subject;
  op: string;
  a?: string;
  b?: string;
}

/** The operators each subject takes, in the order a picker offers them. */
export const OPERATORS: Record<Subject, readonly string[]> = {
  type: ['is', 'is_not'],
  title: ['contains', 'not_contains', 'starts_with'],
  notes: ['empty', 'not_empty', 'contains'],
  completed: ['yes', 'no'],
  archived: ['yes', 'no'],
  due: ['has', 'lacks', 'past', 'future', 'within'],
  assignee: ['has', 'lacks', 'is', 'is_not'],
  bucket: ['is', 'is_not'],
  parent: ['has', 'lacks'],
  depth: ['is', 'at_most'],
  actor: ['is', 'is_not'],
  hour: ['between'],
  field: ['is', 'is_not'],
};

/** Whether the operator takes a value, and which. */
export function takes(subject: Subject, op: string): 'none' | 'value' | 'number' | 'days' | 'range' | 'key_value' {
  if (subject === 'hour') return 'range';
  if (subject === 'field') return 'key_value';
  if (subject === 'depth') return 'number';
  if (subject === 'due') return op === 'within' ? 'days' : 'none';
  if (op === 'has' || op === 'lacks' || op === 'yes' || op === 'no' || op === 'empty' || op === 'not_empty') return 'none';
  return 'value';
}

const quote = (value: string): string => `'${value.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`;
const unquote = (value: string | undefined): string => (value ?? '').replace(/\\'/g, "'").replace(/\\\\/g, '\\');

const days = (value: string): number => Math.max(0, Math.floor(Number(value) || 0));

/**
 * "Contains" and "starts with" are case-insensitive: a person who writes *permit* means *Permit*
 * too. CEL's standard library has no case fold and this installation compiles no extension in, so
 * the sentence becomes an RE2 match with the `(?i)` flag over the value with every metacharacter
 * escaped - and reads back by unescaping it.
 */
const escapeRegex = (value: string): string => value.replace(/[\\^$.|?*+()[\]{}]/g, '\\$&');
const unescapeRegex = (value: string): string => value.replace(/\\([\\^$.|?*+()[\]{}])/g, '$1');
const insensitive = (value: string, anchored: boolean): string => quote(`(?i)${anchored ? '^' : ''}${escapeRegex(value)}`);

export function compileSentence(sentence: Sentence): string {
  const { subject, op, a = '', b = '' } = sentence;
  switch (subject) {
    case 'type':
      return `item.type ${op === 'is' ? '==' : '!='} ${quote(a)}`;
    case 'title':
      if (op === 'starts_with') return `item.title.matches(${insensitive(a, true)})`;
      return `${op === 'not_contains' ? '!' : ''}item.title.matches(${insensitive(a, false)})`;
    case 'notes':
      if (op === 'contains') return `item.notes.matches(${insensitive(a, false)})`;
      return `item.notes ${op === 'empty' ? '==' : '!='} ''`;
    case 'completed':
      return `item.completed == ${op === 'yes' ? 'true' : 'false'}`;
    case 'archived':
      return `item.archived == ${op === 'yes' ? 'true' : 'false'}`;
    case 'due':
      // The three that compare need the date to be there first: `dyn < timestamp` on a missing
      // key is an error at the run, not a false. `now` is the run's one instant.
      if (op === 'has') return 'has(item.due_at)';
      if (op === 'lacks') return '!has(item.due_at)';
      if (op === 'past') return 'has(item.due_at) && item.due_at < now';
      if (op === 'future') return 'has(item.due_at) && item.due_at > now';
      return `has(item.due_at) && item.due_at < now + duration(${quote(`${days(a) * 24}h`)})`;
    case 'parent':
      return op === 'has' ? 'has(item.parent_id)' : '!has(item.parent_id)';
    case 'depth':
      return `item.depth ${op === 'is' ? '==' : '<='} ${Number(a) || 0}`;
    case 'assignee':
      if (op === 'has') return 'has(item.assignee_id)';
      if (op === 'lacks') return '!has(item.assignee_id)';
      return `item.assignee_id ${op === 'is' ? '==' : '!='} ${quote(a)}`;
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
  { pattern: new RegExp(`^(!?)item\\.title\\.matches\\('\\(\\?i\\)\\^((?:[^'\\\\]|\\\\.)*)'\\)$`), read: (m) => ({ subject: 'title', op: 'starts_with', a: unescapeRegex(unquote(m[2])) }) },
  { pattern: new RegExp(`^(!?)item\\.title\\.matches\\('\\(\\?i\\)((?:[^'\\\\]|\\\\.)*)'\\)$`), read: (m) => ({ subject: 'title', op: m[1] ? 'not_contains' : 'contains', a: unescapeRegex(unquote(m[2])) }) },
  { pattern: new RegExp(`^item\\.notes\\.matches\\('\\(\\?i\\)((?:[^'\\\\]|\\\\.)*)'\\)$`), read: (m) => ({ subject: 'notes', op: 'contains', a: unescapeRegex(unquote(m[1])) }) },
  { pattern: /^item\.notes (==|!=) ''$/, read: (m) => ({ subject: 'notes', op: m[1] === '==' ? 'empty' : 'not_empty' }) },
  { pattern: /^item\.completed == (true|false)$/, read: (m) => ({ subject: 'completed', op: m[1] === 'true' ? 'yes' : 'no' }) },
  { pattern: /^item\.archived == (true|false)$/, read: (m) => ({ subject: 'archived', op: m[1] === 'true' ? 'yes' : 'no' }) },
  { pattern: /^(!?)has\(item\.due_at\)$/, read: (m) => ({ subject: 'due', op: m[1] ? 'lacks' : 'has' }) },
  { pattern: /^has\(item\.due_at\) && item\.due_at (<|>) now$/, read: (m) => ({ subject: 'due', op: m[1] === '<' ? 'past' : 'future' }) },
  { pattern: /^has\(item\.due_at\) && item\.due_at < now \+ duration\('(\d+)h'\)$/, read: (m) => ({ subject: 'due', op: 'within', a: String(Math.floor(Number(m[1]) / 24)) }) },
  { pattern: /^(!?)has\(item\.parent_id\)$/, read: (m) => ({ subject: 'parent', op: m[1] ? 'lacks' : 'has' }) },
  { pattern: /^item\.depth (==|<=) (\d+)$/, read: (m) => ({ subject: 'depth', op: m[1] === '==' ? 'is' : 'at_most', a: m[2] }) },
  { pattern: /^(!?)has\(item\.assignee_id\)$/, read: (m) => ({ subject: 'assignee', op: m[1] ? 'lacks' : 'has' }) },
  { pattern: new RegExp(`^item\\.assignee_id (==|!=) ${QUOTED}$`), read: (m) => ({ subject: 'assignee', op: m[1] === '==' ? 'is' : 'is_not', a: unquote(m[2]) }) },
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

/* ---------- The tree (decision 15) ---------- */

/**
 * A condition as a tree: sentences under *all of*, *any of* or *none of*, nested as deep as the
 * writer likes. Compiled to CEL with parentheses and read back by the same grammar - top-level
 * `||` first, then `&&`, a leading `!(…)` for *none*, a pair of parentheses around a group - and
 * anything else stays an expression, as decision 3 says. A group is one `expr` on the server;
 * nothing there changes.
 */
export type GroupMode = 'all' | 'any' | 'none';

export interface Group {
  mode: GroupMode;
  items: Node[];
}

export type Node = Sentence | Group;

export const isGroup = (node: Node): node is Group => 'items' in node;

/** A fresh group of the mode, holding one default sentence. */
export const newGroup = (mode: GroupMode = 'any'): Group => ({ mode, items: [defaultSentence()] });

/** Whether an expression has `&&` or `||` outside every pair of parentheses and quotes. */
function hasTopLevelJoin(expr: string): boolean {
  return splitTopLevel(expr, '&&').length > 1 || splitTopLevel(expr, '||').length > 1;
}

export function compileNode(node: Node): string {
  if (!isGroup(node)) return compileSentence(node);
  const parts = node.items.map((item) => {
    const compiled = compileNode(item);
    // A nested group and a sentence that is itself a join (the hour, a due date compared) are
    // parenthesised, so what the writer grouped is what the engine groups.
    return isGroup(item) || hasTopLevelJoin(compiled) ? `(${compiled})` : compiled;
  }).filter((part) => part !== '' && part !== '()');
  if (parts.length === 0) return '';
  if (node.mode === 'none') return `!(${parts.join(' || ')})`;
  return parts.join(node.mode === 'all' ? ' && ' : ' || ');
}

/** The expression split at a join outside every pair of parentheses and quotes. */
function splitTopLevel(expr: string, join: '&&' | '||'): string[] {
  const parts: string[] = [];
  let depth = 0;
  let quoted = false;
  let start = 0;
  for (let at = 0; at < expr.length; at += 1) {
    const char = expr[at];
    if (quoted) {
      if (char === '\\') at += 1;
      else if (char === "'") quoted = false;
      continue;
    }
    if (char === "'") quoted = true;
    else if (char === '(') depth += 1;
    else if (char === ')') depth -= 1;
    else if (depth === 0 && expr.startsWith(join, at)) {
      parts.push(expr.slice(start, at));
      start = at + 2;
      at += 1;
    }
  }
  parts.push(expr.slice(start));
  return parts.map((part) => part.trim());
}

/** Whether the first `(` closes at the very end, so the pair wraps the whole expression. */
function wrapped(expr: string): boolean {
  if (!expr.startsWith('(') || !expr.endsWith(')')) return false;
  let depth = 0;
  let quoted = false;
  for (let at = 0; at < expr.length; at += 1) {
    const char = expr[at];
    if (quoted) {
      if (char === '\\') at += 1;
      else if (char === "'") quoted = false;
      continue;
    }
    if (char === "'") quoted = true;
    else if (char === '(') depth += 1;
    else if (char === ')') {
      depth -= 1;
      if (depth === 0 && at < expr.length - 1) return false;
    }
  }
  return depth === 0;
}

/** The tree an expression is, where the composer could have written it; undefined otherwise. */
export function readNode(expr: string): Node | undefined {
  const trimmed = expr.trim();
  if (trimmed === '') return undefined;
  const sentence = readSentence(trimmed);
  if (sentence) return sentence;
  if (trimmed.startsWith('!(') && wrapped(trimmed.slice(1))) {
    const inner = readNode(trimmed.slice(2, -1));
    if (!inner) return undefined;
    // `!(a || b)` is none-of; `!(a && b)` has no mode of its own and stays an expression.
    if (isGroup(inner)) return inner.mode === 'any' ? { mode: 'none', items: inner.items } : undefined;
    return { mode: 'none', items: [inner] };
  }
  if (wrapped(trimmed)) {
    const inner = readNode(trimmed.slice(1, -1));
    // A pair of parentheses around one sentence is a group of one: the composer writes it for a
    // group the writer has just made and not yet filled, and reading it back as the bare
    // sentence would make the group vanish under their hands.
    // Except a sentence that is itself a join - the hour, a due date compared - whose parentheses
    // the compiler wrote for the join; those read back as the sentence.
    return inner && !isGroup(inner) && !hasTopLevelJoin(compileSentence(inner)) ? { mode: 'all', items: [inner] } : inner;
  }
  for (const [join, mode] of [['||', 'any'], ['&&', 'all']] as const) {
    const parts = splitTopLevel(trimmed, join);
    if (parts.length > 1) {
      const items = parts.map((part) => readNode(part));
      if (items.some((item) => item === undefined)) return undefined;
      return { mode, items: items as Node[] };
    }
  }
  return undefined;
}
