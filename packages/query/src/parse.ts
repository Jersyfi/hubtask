// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The browser's twin of core/domain/model/view/Query.go and Line.go (ADR-0102): the same reading of
// both spellings onto the filter tree, the same canonical text. It answers while somebody types,
// before the server is asked; the server reads the text again and its answer is the one that
// counts. test/querylanguage/lines.json holds the two to the same trees, so they cannot drift.

/** A node of the filter tree, in the contract's shape (`FilterNode`, ADR-0026). */
export type FilterNode = { readonly [key: string]: unknown };

/** A query read from text: the tree, the order, and what the tree has no node for. */
export interface Query {
  filter?: FilterNode;
  sort?: { field: string; dir?: 'DESC' }[];
  words?: string;
  kinds?: string[];
  include_archived?: boolean;
  include_trashed?: boolean;
}

/** A refusal as the server words it: a code, the character it stops at (1 first), a suggestion. */
export interface QueryError {
  readonly code: string;
  readonly at?: number;
  readonly params: Readonly<Record<string, string>>;
}

export type Parsed = { ok: true; query: Query } | { ok: false; error: QueryError };

export const MAX_LINE_LENGTH = 4000;
const MAX_DEPTH = 5;
const MAX_NESTING = 2 * MAX_DEPTH;

export const OBJECT_KINDS = [
  'task', 'workpackage', 'activity', 'ticket', 'page', 'whiteboard', 'post', 'goal', 'dashboard',
  'asset', 'file', 'person', 'comment', 'collection', 'hub',
] as const;

const ENTRY_LEVELS: Record<string, string> = { task: 'TASK', workpackage: 'WORK_PACKAGE', activity: 'ACTIVITY' };

/** The fields the twin knows, by the grammar's name: their kind and the operators that matter here. */
type Kind = 'string' | 'text' | 'id' | 'boolean' | 'integer' | 'timestamp' | 'enum' | 'id_set' | 'custom';
interface Field { readonly name: string; readonly kind: Kind; readonly values?: readonly string[]; readonly sortable?: boolean }

const SLA_STANDINGS = ['MET', 'PAUSED', 'RUNNING', 'AT_RISK', 'BREACHED'];
const ASSET_STAGES = ['PLANNED', 'ORDERED', 'IN_STOCK', 'IN_USE', 'MAINTENANCE', 'RETIRED', 'DISPOSED'];
const GOAL_STATES = ['NONE', 'ON_TRACK', 'AT_RISK', 'OFF_TRACK', 'DONE', 'DROPPED'];
const FIELDS: readonly Field[] = [
  { name: 'type', kind: 'enum', values: ['TASK', 'WORK_PACKAGE', 'ACTIVITY'], sortable: true },
  { name: 'entry_type', kind: 'string' }, { name: 'parent_id', kind: 'id' }, { name: 'collection_id', kind: 'id' },
  { name: 'bucket_id', kind: 'id' }, { name: 'status', kind: 'string' }, { name: 'is_completed', kind: 'boolean', sortable: true },
  { name: 'title', kind: 'string', sortable: true }, { name: 'notes', kind: 'text' }, { name: 'depth', kind: 'integer', sortable: true },
  { name: 'order_key', kind: 'string', sortable: true }, { name: 'created_by', kind: 'id' },
  { name: 'created_at', kind: 'timestamp', sortable: true }, { name: 'updated_at', kind: 'timestamp', sortable: true },
  { name: 'completed_at', kind: 'timestamp', sortable: true }, { name: 'start_at', kind: 'timestamp', sortable: true },
  { name: 'due_at', kind: 'timestamp', sortable: true }, { name: 'archived_at', kind: 'timestamp', sortable: true },
  { name: 'labels', kind: 'id_set' }, { name: 'assignee_id', kind: 'id' }, { name: 'members', kind: 'id_set' },
  { name: 'text', kind: 'text' }, { name: 'blocked', kind: 'boolean' }, { name: 'blocking', kind: 'boolean' },
  { name: 'linked_to', kind: 'id' }, { name: 'sla_due', kind: 'timestamp', sortable: true },
  { name: 'sla_standing', kind: 'enum', values: SLA_STANDINGS }, { name: 'group_queue_of', kind: 'id' },
  { name: 'satisfaction', kind: 'integer' }, { name: 'hub_id', kind: 'id' }, { name: 'under', kind: 'id' },
  { name: 'blocks', kind: 'id' }, { name: 'affects', kind: 'id' }, { name: 'sprint_id', kind: 'id' },
  { name: 'release_id', kind: 'id' }, { name: 'slip_days', kind: 'integer' },
  { name: 'critical', kind: 'boolean' }, { name: 'time_logged', kind: 'integer' },
  // The keys of one kind, answered by that kind's search alone (POST /search:all).
  { name: 'asset.class', kind: 'string' }, { name: 'asset.stage', kind: 'enum', values: ASSET_STAGES },
  { name: 'asset.owner', kind: 'id' }, { name: 'asset.location', kind: 'id' }, { name: 'asset.tag', kind: 'string' },
  { name: 'page.verified', kind: 'boolean' }, { name: 'page.owner', kind: 'id' },
  { name: 'goal.state', kind: 'enum', values: GOAL_STATES },
];
const KIND_KEYS = ['asset.class', 'asset.stage', 'asset.owner', 'asset.location', 'asset.tag', 'page.verified', 'page.owner', 'goal.state'];
/** The enums written lowered, as they are typed. */
const LOWERED_ENUMS = new Set(['sla_standing', 'asset.stage', 'goal.state']);

export const READER_KEYS: Readonly<Record<string, string>> = {
  status: 'status', type: 'type', who: 'assignee_id', with: 'members', by: 'created_by', in: 'collection_id',
  label: 'labels', created: 'created_at', updated: 'updated_at', due: 'due_at', start: 'start_at',
  done: 'completed_at', title: 'title', text: 'text', note: 'notes', linked: 'linked_to', parent: 'parent_id',
  'desk.sla': 'sla_due', 'desk.standing': 'sla_standing', 'desk.satisfaction': 'satisfaction',
  hub: 'hub_id', under: 'under', blocks: 'blocks', affects: 'affects', 'plan.sprint': 'sprint_id',
  'plan.release': 'release_id', 'plan.slip': 'slip_days', 'plan.critical': 'critical', 'time.logged': 'time_logged',
};
const FIELD_KEYS: Record<string, string> = Object.fromEntries(Object.entries(READER_KEYS).map(([k, v]) => [v, k]));

const IS_WORDS: Record<string, { field: string; value: boolean }> = {
  open: { field: 'is_completed', value: false }, done: { field: 'is_completed', value: true },
  blocked: { field: 'blocked', value: true }, blocking: { field: 'blocking', value: true },
};

const PERIODS: Record<string, [string, string]> = {
  today: ['@today', '@end_of_day'],
  'this-week': ['@start_of_week', '@end_of_week'],
  'last-week': ['@start_of_week-P1W', '@start_of_week-PT1S'],
  'next-week': ['@start_of_week+P1W', '@end_of_week+P1W'],
  'this-month': ['@start_of_month', '@end_of_month'],
  'last-month': ['@start_of_month-P1M', '@start_of_month-PT1S'],
};

const ANCHORS: Record<string, string> = {
  now: '@now', today: '@today', 'end-of-day': '@end_of_day', 'start-of-week': '@start_of_week',
  'end-of-week': '@end_of_week', 'start-of-month': '@start_of_month', 'end-of-month': '@end_of_month',
};

const FIELD_MARKER = /^(-?)([A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*):/;
const RELATIVE = /^([+-])(\d{1,4})([dwmy])$/;
const DATE_ONLY = /^\d{4}-\d{2}-\d{2}$/;
const TODAY_OFFSET = /^@today([+-])P(\d{1,4})([DWMY])$/;
const CUSTOM_KEY = /^[a-z][a-z0-9_]{0,49}$/;
const SPACE = /\s/u;

class Refusal {
  readonly error: QueryError;
  constructor(error: QueryError) {
    this.error = error;
  }
}

const runes = (text: string) => Array.from(text).length;

function refuse(text: string, at: number, code: string, params: Record<string, string> = {}): never {
  throw new Refusal({ code, at: runes(text.slice(0, at)) + 1, params });
}

function fieldOf(name: string): Field | undefined {
  if (name.startsWith('custom_fields.')) {
    return CUSTOM_KEY.test(name.slice('custom_fields.'.length)) ? { name, kind: 'custom', sortable: true } : undefined;
  }
  return FIELDS.find((field) => field.name === name);
}

export function resolveKey(key: string): string | undefined {
  if (key in READER_KEYS) return READER_KEYS[key];
  if (key.startsWith('cf.')) {
    const name = `custom_fields.${key.slice(3)}`;
    return fieldOf(name) ? name : undefined;
  }
  return fieldOf(key) ? key : undefined;
}

const queryKeys = () => [...Object.keys(READER_KEYS), ...KIND_KEYS, 'is', 'type', 'sort'].sort();
const sortKeys = () => Object.keys(READER_KEYS).filter((key) => fieldOf(READER_KEYS[key] ?? '')?.sortable).sort();

function editDistance(a: string, b: string): number {
  const ra = Array.from(a);
  const rb = Array.from(b);
  const row = rb.map((_, j) => j).concat(rb.length);
  for (let i = 1; i <= ra.length; i += 1) {
    let previous = row[0] ?? 0;
    row[0] = i;
    for (let j = 1; j <= rb.length; j += 1) {
      const cost = ra[i - 1] === rb[j - 1] ? 0 : 1;
      const current = Math.min((row[j] ?? 0) + 1, (row[j - 1] ?? 0) + 1, previous + cost);
      previous = row[j] ?? 0;
      row[j] = current;
    }
  }
  return row[rb.length] ?? 0;
}

function nearest(typed: string, candidates: readonly string[]): string {
  let best = '';
  let distance = 3;
  for (const candidate of candidates) {
    if (typed !== '' && candidate.startsWith(typed)) return candidate;
    const d = editDistance(typed, candidate);
    if (d < distance) {
      best = candidate;
      distance = d;
    }
  }
  return best;
}

function suggestion(field: string, typed: string, candidates: readonly string[]): Record<string, string> {
  const params: Record<string, string> = { field };
  const best = nearest(typed.toLowerCase(), candidates);
  if (best) params.suggestion = best;
  return params;
}

// ---------------------------------------------------------------------------------------------
// The field: value spelling
// ---------------------------------------------------------------------------------------------

interface Token { kind: 'word' | 'text' | 'symbol'; text: string; at: number; joined: boolean }

export function lexQuery(text: string): Token[] {
  const tokens: Token[] = [];
  let end = -1;
  let i = 0;
  while (i < text.length) {
    const char = text[i] ?? '';
    const joined = i === end;
    if (SPACE.test(char)) {
      i += 1;
      continue;
    }
    if (char === '"') {
      let value = '';
      let j = i + 1;
      let closed = false;
      for (; j < text.length; j += 1) {
        if (text[j] === '\\' && j + 1 < text.length) {
          j += 1;
          value += text[j];
          continue;
        }
        if (text[j] === '"') {
          closed = true;
          break;
        }
        value += text[j];
      }
      if (!closed) refuse(text, i, 'query.line_malformed');
      tokens.push({ kind: 'text', text: value, at: i, joined });
      i = j + 1;
    } else if (char === '(' || char === ')' || char === ',') {
      tokens.push({ kind: 'symbol', text: char, at: i, joined });
      i += 1;
    } else {
      const start = i;
      while (i < text.length && !SPACE.test(text[i] ?? '') && !'(),"'.includes(text[i] ?? '')) i += 1;
      tokens.push({ kind: 'word', text: text.slice(start, i), at: start, joined });
    }
    end = i;
  }
  return tokens;
}

type Item = { node?: FilterNode; piece?: string };
// group is set for `members("…")`, whose argument text is.
interface Raw { text: string; quoted: boolean; at: number; group?: boolean }

/** The one function value: the people of a group, `members("Support")`. */
const MEMBERS_FUNCTION = 'members';

const matches = (piece: string): FilterNode => ({ field: 'text', op: 'MATCHES', value: piece });
const not = (node: FilterNode): FilterNode => ({ op: 'NOT', nodes: [node] });
const phrase = (text: string) => `"${text.replaceAll('"', '')}"`;

function joinAnd(nodes: FilterNode[]): FilterNode | undefined {
  if (nodes.length === 0) return undefined;
  if (nodes.length === 1) return nodes[0];
  return { op: 'AND', nodes };
}

function nodesOf(items: Item[]): FilterNode[] {
  return items.map((item) => {
    if (item.node) return item.node;
    const piece = item.piece ?? '';
    return piece.startsWith('-') ? not(matches(piece.slice(1))) : matches(piece);
  });
}

class QueryParser {
  pos = 0;
  readonly q: Query = {};
  readonly tokens: Token[];
  readonly text: string;
  constructor(tokens: Token[], text: string) {
    this.tokens = tokens;
    this.text = text;
  }

  done = () => this.pos >= this.tokens.length;
  peek = (): Token => this.tokens[this.pos] ?? { kind: 'symbol', text: '', at: this.text.length, joined: false };
  atWord = (word: string) => !this.done() && this.peek().kind === 'word' && this.peek().text === word;
  atSymbol = (symbol: string) => !this.done() && this.peek().kind === 'symbol' && this.peek().text === symbol;
  atJoinedSymbol = (symbol: string) => this.atSymbol(symbol) && this.peek().joined;

  /** Reads `("name")` or `(name)` after `members`. */
  groupArgument(at: number): Raw {
    this.pos += 1;
    if (this.done() || this.peek().kind === 'symbol') refuse(this.text, at, 'query.group_required');
    const arg = this.peek();
    this.pos += 1;
    if (!this.atSymbol(')')) refuse(this.text, at, 'query.group_required');
    this.pos += 1;
    return { text: arg.text, quoted: false, at, group: true };
  }

  query(): Query {
    const items = this.and(0, true);
    if (!this.done()) refuse(this.text, this.peek().at, 'query.line_malformed');
    const words = items.filter((item) => !item.node).map((item) => item.piece ?? '');
    if (words.length) this.q.words = words.join(' ');
    const filter = joinAnd(items.filter((item) => item.node).map((item) => item.node as FilterNode));
    if (filter) this.q.filter = filter;
    return this.q;
  }

  or(nesting: number): FilterNode {
    const nodes = this.branches(nesting);
    return nodes.length === 1 ? (nodes[0] as FilterNode) : { op: 'OR', nodes };
  }

  branches(nesting: number): FilterNode[] {
    const nodes: FilterNode[] = [];
    for (;;) {
      nodes.push(joinAnd(nodesOf(this.and(nesting, false))) as FilterNode);
      if (!this.atWord('OR')) return nodes;
      this.pos += 1;
    }
  }

  and(nesting: number, top: boolean): Item[] {
    const items: Item[] = [];
    while (!this.done() && !this.atSymbol(')')) {
      if (this.atWord('OR')) {
        if (!top || items.length === 0) break;
        this.pos += 1;
        const rest = this.branches(nesting);
        return [{ node: { op: 'OR', nodes: [joinAnd(nodesOf(items)) as FilterNode, ...rest] } }];
      }
      if (this.atWord('AND')) {
        this.pos += 1;
        continue;
      }
      const item = this.unary(nesting, top);
      if (item.node || item.piece) items.push(item);
    }
    if (items.length === 0 && !top) refuse(this.text, this.peek().at, 'query.line_malformed');
    return items;
  }

  unary(nesting: number, top: boolean): Item {
    const t = this.peek();
    const next = this.tokens[this.pos + 1];
    if (this.atWord('NOT') || (t.kind === 'word' && t.text === '-' && next?.joined && next.text === '(')) {
      this.pos += 1;
      if (nesting >= MAX_NESTING) refuse(this.text, t.at, 'query.filter_too_deep', { maximum: String(MAX_DEPTH) });
      const operand = this.unary(nesting + 1, false);
      return { node: not(operand.node ?? (nodesOf([operand])[0] as FilterNode)) };
    }
    if (this.atSymbol('(')) {
      this.pos += 1;
      if (nesting >= MAX_NESTING) refuse(this.text, t.at, 'query.filter_too_deep', { maximum: String(MAX_DEPTH) });
      const node = this.or(nesting + 1);
      if (!this.atSymbol(')')) refuse(this.text, this.peek().at, 'query.bracket_unclosed', { suggestion: ')' });
      this.pos += 1;
      return { node };
    }
    if (t.kind === 'symbol') {
      if (t.text === ',') {
        this.pos += 1;
        return {};
      }
      refuse(this.text, t.at, 'query.line_malformed');
    }
    if (t.kind === 'text') {
      this.pos += 1;
      return { piece: phrase(t.text) };
    }
    const marker = FIELD_MARKER.exec(t.text);
    if (marker) return this.term(marker, top);
    this.pos += 1;
    const after = this.peek();
    if (t.text === '-' && !this.done() && after.joined && after.kind === 'text') {
      this.pos += 1;
      return { piece: `-${phrase(after.text)}` };
    }
    return { piece: t.text };
  }

  term(marker: RegExpExecArray, top: boolean): Item {
    const t = this.tokens[this.pos] as Token;
    this.pos += 1;
    const negated = marker[1] === '-';
    const key = marker[2] ?? '';
    const keyAt = t.at + (marker[1] ?? '').length;
    const values: Raw[] = [];
    let all = false;
    const rest = t.text.slice(marker[0].length);
    let expect = true;
    if (rest !== '') {
      values.push({ text: rest, quoted: false, at: t.at + marker[0].length });
      expect = false;
      if (rest === MEMBERS_FUNCTION && this.atJoinedSymbol('(')) values[0] = this.groupArgument(t.at + marker[0].length);
    }
    for (;;) {
      if (expect) {
        const v = this.peek();
        if (this.done() || v.kind === 'symbol' || (v.kind === 'word' && (['OR', 'AND', 'NOT'].includes(v.text) || FIELD_MARKER.test(v.text)))) {
          refuse(this.text, v.at, 'query.value_required', { field: key });
        }
        this.pos += 1;
        if (v.kind === 'word' && ['<', '<=', '>', '>=', '='].includes(v.text) && !this.done() && this.peek().kind === 'word') {
          const operand = this.peek();
          this.pos += 1;
          values.push({ text: v.text + operand.text, quoted: false, at: v.at });
        } else if (v.kind === 'word' && v.text === MEMBERS_FUNCTION && this.atJoinedSymbol('(')) {
          values.push(this.groupArgument(v.at));
        } else {
          values.push({ text: v.text, quoted: v.kind === 'text', at: v.at });
        }
      }
      if (this.atSymbol(',')) {
        this.pos += 1;
        expect = true;
        continue;
      }
      if (this.atWord('+') && values.length > 0) {
        this.pos += 1;
        all = true;
        expect = true;
        continue;
      }
      break;
    }
    if (all && values.length < 2) refuse(this.text, t.at, 'query.line_malformed');
    return this.leaf(key, keyAt, negated, values, all, top);
  }

  leaf(key: string, keyAt: number, negated: boolean, values: Raw[], all: boolean, top: boolean): Item {
    if (key === 'sort') {
      if (!top || negated) refuse(this.text, keyAt, 'query.sort_in_group');
      for (const v of values) {
        const desc = v.text.startsWith('-');
        const name = desc ? v.text.slice(1) : v.text;
        const field = resolveKey(name);
        if (!field || KIND_KEYS.includes(field)) refuse(this.text, v.at, 'query.field_unknown', suggestion(name, name, sortKeys()));
        (this.q.sort ??= []).push(desc ? { field, dir: 'DESC' } : { field });
      }
      return {};
    }
    if (key === 'is') return this.isTerm(negated, values, top);
    if (key === 'type') return this.typeTerm(keyAt, negated, values, top);
    const name = resolveKey(key);
    const field = name ? fieldOf(name) : undefined;
    if (!field) refuse(this.text, keyAt, 'query.field_unknown', suggestion(key, key, queryKeys()));
    return { node: this.compare(field, negated, values, all) };
  }

  isTerm(negated: boolean, values: Raw[], top: boolean): Item {
    const nodes: FilterNode[] = [];
    for (const v of values) {
      const word = v.text.toLowerCase();
      if (word === 'archived' || word === 'trashed') {
        if (!top || negated || values.length > 1) refuse(this.text, v.at, 'query.flag_in_group', { value: word });
        if (word === 'archived') this.q.include_archived = true;
        else this.q.include_trashed = true;
        return {};
      }
      const state = IS_WORDS[word];
      if (!state) refuse(this.text, v.at, 'query.value_unknown', suggestion('is', v.text, ['open', 'done', 'blocked', 'blocking', 'archived', 'trashed']));
      nodes.push({ field: state.field, op: negated ? 'NEQ' : 'EQ', value: state.value });
    }
    if (nodes.length === 1) return { node: nodes[0] as FilterNode };
    return { node: { op: negated ? 'AND' : 'OR', nodes } };
  }

  typeTerm(keyAt: number, negated: boolean, values: Raw[], top: boolean): Item {
    const levels: string[] = [];
    const kinds: string[] = [];
    for (const v of values) {
      const word = v.text.replaceAll('_', '').toLowerCase();
      if (!(OBJECT_KINDS as readonly string[]).includes(word)) {
        refuse(this.text, v.at, 'query.value_unknown', suggestion('type', v.text, OBJECT_KINDS));
      }
      const level = ENTRY_LEVELS[word];
      if (level) levels.push(level);
      if (!kinds.includes(word)) kinds.push(word);
    }
    if (levels.length < kinds.length) {
      if (!top || negated) refuse(this.text, keyAt, 'query.kind_in_group');
      (this.q.kinds ??= []).push(...kinds);
      return {};
    }
    const many = levels.length > 1;
    const op = negated ? (many ? 'NOT_IN' : 'NEQ') : many ? 'IN' : 'EQ';
    return { node: { field: 'type', op, value: many ? levels : levels[0] } };
  }

  compare(field: Field, negated: boolean, values: Raw[], all: boolean): FilterNode {
    const first = values[0] as Raw;
    if (values.length === 1 && !first.quoted) {
      const sign = comparator(first.text);
      if (sign) {
        const bound = sign.op === 'LT' || sign.op === 'GTE' ? 'low' : sign.op === 'LTE' || sign.op === 'GT' ? 'high' : '';
        return negate(this.single(field, sign.op, { text: sign.operand, quoted: false, at: first.at }, bound), negated);
      }
      const cut = first.text.indexOf('..');
      if (cut >= 0) return negate(this.rangeOf(field, first.text.slice(0, cut), first.text.slice(cut + 2), first.at), negated);
    }
    const none = values.some((v) => !v.quoted && v.text === 'none');
    const rest = values.filter((v) => v.quoted || v.text !== 'none');
    const nodes: FilterNode[] = [];
    if (rest.length) nodes.push(this.defaultCompare(field, rest, all));
    if (none) nodes.push({ field: field.name, op: 'IS_NULL' });
    return negate(nodes.length > 1 ? { op: 'OR', nodes } : (nodes[0] as FilterNode), negated);
  }

  defaultCompare(field: Field, values: Raw[], all: boolean): FilterNode {
    let op = defaultOp(field);
    if (values.length === 1) return this.single(field, op, values[0] as Raw, '');
    if (field.kind === 'id_set') op = all ? 'CONTAINS_ALL' : 'CONTAINS_ANY';
    else if (all || op !== 'EQ') refuse(this.text, (values[1] as Raw).at, 'query.values_not_allowed', { field: field.name });
    else op = 'IN';
    return { field: field.name, op, value: values.map((v) => this.value(field, v, '')) };
  }

  single(field: Field, op: string, v: Raw, bound: string): FilterNode {
    if (field.kind === 'timestamp' && !v.quoted && op === 'EQ' && bound === '') {
      const word = v.text.toLowerCase();
      const span = PERIODS[word];
      if (span) return { field: field.name, op: 'BETWEEN', value: [...span] };
      if (word === 'overdue') return { field: field.name, op: 'LT', value: '@today' };
      const m = RELATIVE.exec(word);
      if (m) return { field: field.name, op: m[1] === '-' ? 'GTE' : 'LTE', value: offsetPlaceholder(m) };
      if (DATE_ONLY.test(v.text)) return { field: field.name, op: 'BETWEEN', value: [`${v.text}T00:00:00Z`, `${v.text}T23:59:59Z`] };
    }
    return { field: field.name, op, value: this.value(field, v, bound) };
  }

  rangeOf(field: Field, low: string, high: string, at: number): FilterNode {
    if (low === '' && high === '') refuse(this.text, at, 'query.value_required', { field: field.name });
    if (low === '') return this.single(field, 'LTE', { text: high, quoted: false, at }, 'high');
    if (high === '') return this.single(field, 'GTE', { text: low, quoted: false, at }, 'low');
    return {
      field: field.name, op: 'BETWEEN',
      value: [this.value(field, { text: low, quoted: false, at }, 'low'), this.value(field, { text: high, quoted: false, at }, 'high')],
    };
  }

  value(field: Field, v: Raw, bound: string): unknown {
    if (v.group) {
      if (field.kind !== 'id' && field.kind !== 'id_set') refuse(this.text, v.at, 'query.value_unknown', { field: field.name });
      return `@group:${v.text}`;
    }
    if (v.quoted) return v.text;
    const word = v.text;
    switch (field.kind) {
      case 'id':
      case 'id_set':
        if (word === 'me') return '@me';
        if (word === 'my-groups') return '@my_groups';
        if (word === 'here') refuse(this.text, v.at, 'query.value_needs_place', { field: field.name });
        break;
      case 'timestamp': {
        const lower = word.toLowerCase();
        const anchor = ANCHORS[lower];
        if (anchor) return anchor;
        const m = RELATIVE.exec(lower);
        if (m) return offsetPlaceholder(m);
        if (DATE_ONLY.test(word)) return bound === 'high' ? `${word}T23:59:59Z` : `${word}T00:00:00Z`;
        break;
      }
      case 'boolean': {
        const lower = word.toLowerCase();
        if (lower === 'true' || lower === 'yes') return true;
        if (lower === 'false' || lower === 'no') return false;
        refuse(this.text, v.at, 'query.value_unknown', suggestion(field.name, word, ['true', 'false']));
        break;
      }
      case 'integer':
        if (/^[+-]?\d+$/.test(word)) return Number(word);
        break;
      case 'enum': {
        const allowed = (field.values ?? []).find((value) => value.toLowerCase() === word.toLowerCase());
        if (allowed) return allowed;
        refuse(this.text, v.at, 'query.value_unknown', suggestion(field.name, word, (field.values ?? []).map((value) => value.toLowerCase())));
        break;
      }
      case 'custom':
        return bareCustom(word);
      default:
    }
    return word;
  }
}

function bareCustom(word: string): unknown {
  if (word === 'true' || word === 'false') return word === 'true';
  if (/^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$/.test(word)) {
    const number = Number(word);
    if (Number.isFinite(number)) return number;
  }
  return word;
}

function negate(node: FilterNode, negated: boolean): FilterNode {
  if (!negated) return node;
  if (node.op === 'EQ') return { ...node, op: 'NEQ' };
  if (node.op === 'IN') return { ...node, op: 'NOT_IN' };
  return not(node);
}

function defaultOp(field: Field): string {
  if (field.kind === 'id_set') return 'CONTAINS';
  if (field.name === 'text') return 'MATCHES';
  if (field.name === 'title' || field.name === 'notes') return 'CONTAINS';
  return 'EQ';
}

function comparator(text: string): { op: string; operand: string } | undefined {
  for (const [prefix, op] of [['<=', 'LTE'], ['>=', 'GTE'], ['<', 'LT'], ['>', 'GT'], ['=', 'EQ'], ['~', 'CONTAINS']] as const) {
    if (text.startsWith(prefix) && text.length > prefix.length) return { op, operand: text.slice(prefix.length) };
  }
  if (text.endsWith('*') && text.length > 1) return { op: 'STARTS_WITH', operand: text.slice(0, -1) };
  return undefined;
}

const offsetPlaceholder = (m: RegExpExecArray) => `@today${m[1]}P${m[2]}${(m[3] ?? '').toUpperCase()}`;

// ---------------------------------------------------------------------------------------------
// The = / AND / ORDER BY line (Line.go)
// ---------------------------------------------------------------------------------------------

const LINE_KEYWORDS = new Set(['AND', 'OR', 'NOT', 'IN', 'IS', 'EMPTY', 'NULL', 'BETWEEN', 'CONTAINS', 'ANY', 'ALL', 'STARTS', 'WITH', 'MATCHES', 'ORDER', 'BY', 'ASC', 'DESC', 'NULLS', 'FIRST', 'LAST']);
const SYMBOLS: Record<string, string> = { '=': 'EQ', '!=': 'NEQ', '<': 'LT', '<=': 'LTE', '>': 'GT', '>=': 'GTE', '~': 'CONTAINS' };

function lexLine(line: string): Token[] | number {
  const tokens: Token[] = [];
  let i = 0;
  while (i < line.length) {
    const c = line[i] ?? '';
    if (SPACE.test(c)) {
      i += 1;
    } else if (c === "'" || c === '"') {
      let value = '';
      let j = i + 1;
      for (; j < line.length && line[j] !== c; j += 1) {
        if (line[j] === '\\' && j + 1 < line.length) j += 1;
        value += line[j];
      }
      if (j >= line.length) return i;
      tokens.push({ kind: 'text', text: value, at: i, joined: false });
      i = j + 1;
    } else if ('(),~'.includes(c)) {
      tokens.push({ kind: 'symbol', text: c, at: i, joined: false });
      i += 1;
    } else if ('=!<>'.includes(c)) {
      const symbol = line[i + 1] === '=' ? `${c}=` : c;
      if (symbol === '!') return i;
      tokens.push({ kind: 'symbol', text: symbol, at: i, joined: false });
      i += symbol.length;
    } else {
      const start = i;
      while (i < line.length && !SPACE.test(line[i] ?? '') && !`()'",~=!<>`.includes(line[i] ?? '')) i += 1;
      tokens.push({ kind: 'word', text: line.slice(start, i), at: start, joined: false });
    }
  }
  return tokens;
}

/** Whether a text is the `=` line, decided as the server decides it. */
function comparisonLine(text: string): boolean {
  const tokens = lexLine(text);
  if (typeof tokens === 'number') {
    if (text.split(/\s+/u).some((word) => FIELD_MARKER.test(word))) return false;
    return /[=<>~']/.test(text);
  }
  let compares = false;
  for (const t of tokens) {
    if (t.kind === 'word' && FIELD_MARKER.test(t.text)) return false;
    if (t.kind === 'symbol' && SYMBOLS[t.text]) compares = true;
    if (t.kind === 'word' && ['IN', 'IS', 'BETWEEN', 'CONTAINS', 'STARTS', 'MATCHES', 'ORDER'].includes(t.text.toUpperCase())) compares = true;
  }
  return compares;
}

class LineParser {
  pos = 0;
  readonly tokens: Token[];
  readonly line: string;
  constructor(tokens: Token[], line: string) {
    this.tokens = tokens;
    this.line = line;
  }
  done = () => this.pos >= this.tokens.length;
  peek = (): Token => this.tokens[this.pos] ?? { kind: 'symbol', text: '', at: this.line.length, joined: false };
  next = () => { const t = this.peek(); this.pos += 1; return t; };
  fail = (): never => refuse(this.line, this.peek().at, 'query.line_malformed');
  atKeyword = (word: string) => this.peek().kind === 'word' && this.peek().text.toUpperCase() === word;
  atSymbol = (symbol: string) => !this.done() && this.peek().kind === 'symbol' && this.peek().text === symbol;

  combination(op: string, nesting: number, operand: (n: number) => FilterNode): FilterNode {
    const nodes = [operand(nesting)];
    while (this.atKeyword(op)) {
      this.next();
      nodes.push(operand(nesting));
    }
    return nodes.length === 1 ? (nodes[0] as FilterNode) : { op, nodes };
  }
  or = (n: number): FilterNode => this.combination('OR', n, this.and);
  and = (n: number): FilterNode => this.combination('AND', n, this.not);
  not = (n: number): FilterNode => {
    if (this.atKeyword('NOT')) {
      this.next();
      if (n >= MAX_NESTING) this.fail();
      return { op: 'NOT', nodes: [this.not(n + 1)] };
    }
    if (this.atSymbol('(')) {
      this.next();
      if (n >= MAX_NESTING) this.fail();
      const node = this.or(n + 1);
      if (!this.atSymbol(')')) this.fail();
      this.next();
      return node;
    }
    return this.comparison();
  };

  comparison(): FilterNode {
    const name = this.peek();
    if (name.kind !== 'word' || LINE_KEYWORDS.has(name.text.toUpperCase())) this.fail();
    this.next();
    const field = fieldOf(name.text);
    const leaf: Record<string, unknown> = { field: name.text };
    const op = this.peek();
    if (op.kind === 'symbol' && SYMBOLS[op.text]) {
      this.next();
      return { ...leaf, op: SYMBOLS[op.text], value: this.value(field) };
    }
    const word = (w: string) => { if (!this.atKeyword(w)) this.fail(); this.next(); };
    if (this.atKeyword('IN')) { this.next(); return this.list(leaf, 'IN', field); }
    if (this.atKeyword('NOT')) { this.next(); word('IN'); return this.list(leaf, 'NOT_IN', field); }
    if (this.atKeyword('IS')) {
      this.next();
      const negated = this.atKeyword('NOT');
      if (negated) this.next();
      if (!this.atKeyword('EMPTY') && !this.atKeyword('NULL')) this.fail();
      this.next();
      const node = { ...leaf, op: 'IS_NULL' };
      return negated ? { op: 'NOT', nodes: [node] } : node;
    }
    if (this.atKeyword('BETWEEN')) {
      this.next();
      const low = this.value(field);
      word('AND');
      return { ...leaf, op: 'BETWEEN', value: [low, this.value(field)] };
    }
    if (this.atKeyword('CONTAINS')) {
      this.next();
      if (this.atKeyword('ANY')) { this.next(); return this.list(leaf, 'CONTAINS_ANY', field); }
      if (this.atKeyword('ALL')) { this.next(); return this.list(leaf, 'CONTAINS_ALL', field); }
      return { ...leaf, op: 'CONTAINS', value: this.value(field) };
    }
    if (this.atKeyword('STARTS')) { this.next(); word('WITH'); return { ...leaf, op: 'STARTS_WITH', value: this.value(field) }; }
    if (this.atKeyword('MATCHES')) { this.next(); return { ...leaf, op: 'MATCHES', value: this.value(field) }; }
    return this.fail();
  }

  list(leaf: Record<string, unknown>, op: string, field: Field | undefined): FilterNode {
    if (!this.atSymbol('(')) this.fail();
    this.next();
    const values: unknown[] = [];
    for (;;) {
      values.push(this.value(field));
      if (this.atSymbol(',')) { this.next(); continue; }
      if (!this.atSymbol(')')) this.fail();
      this.next();
      return { ...leaf, op, value: values };
    }
  }

  value(field: Field | undefined): unknown {
    const t = this.peek();
    if (t.kind === 'text') { this.next(); return t.text; }
    if (t.kind !== 'word' || LINE_KEYWORDS.has(t.text.toUpperCase())) this.fail();
    this.next();
    if (field?.kind === 'integer' && /^[+-]?\d+$/.test(t.text)) return Number(t.text);
    if (field?.kind === 'boolean' && (t.text === 'true' || t.text === 'false')) return t.text === 'true';
    if (field?.kind === 'custom') return bareCustom(t.text);
    return t.text;
  }

  sortList(): { field: string; dir?: 'DESC' }[] {
    const terms: Record<string, string>[] = [];
    for (;;) {
      const name = this.peek();
      if (name.kind !== 'word' || LINE_KEYWORDS.has(name.text.toUpperCase())) this.fail();
      this.next();
      const term: Record<string, string> = { field: name.text };
      if (this.atKeyword('ASC') || this.atKeyword('DESC')) term.dir = this.next().text.toUpperCase();
      if (this.atKeyword('NULLS')) {
        this.next();
        if (!this.atKeyword('FIRST') && !this.atKeyword('LAST')) this.fail();
        term.nulls = this.next().text.toUpperCase();
      }
      terms.push(term);
      if (!this.atSymbol(',')) return terms as { field: string; dir?: 'DESC' }[];
      this.next();
    }
  }
}

function parseLine(line: string): Query {
  const tokens = lexLine(line);
  if (typeof tokens === 'number') refuse(line, tokens, 'query.line_malformed');
  const p = new LineParser(tokens, line);
  const q: Query = {};
  if (!p.atKeyword('ORDER') && !p.done()) q.filter = p.or(0);
  if (p.atKeyword('ORDER')) {
    p.next();
    if (!p.atKeyword('BY')) p.fail();
    p.next();
    q.sort = p.sortList();
  }
  if (!p.done()) p.fail();
  return q;
}

/** Reads a query in either spelling, as the server's ParseQuery does. */
export function parseQuery(text: string): Parsed {
  try {
    if (new TextEncoder().encode(text).length > MAX_LINE_LENGTH) {
      return { ok: false, error: { code: 'query.line_too_long', params: { maximum: String(MAX_LINE_LENGTH) } } };
    }
    if (text.trim() === '') return { ok: true, query: {} };
    if (comparisonLine(text)) return { ok: true, query: parseLine(text) };
    return { ok: true, query: new QueryParser(lexQuery(text), text).query() };
  } catch (thrown) {
    if (thrown instanceof Refusal) return { ok: false, error: thrown.error };
    throw thrown;
  }
}

// ---------------------------------------------------------------------------------------------
// Writing the field: value spelling back
// ---------------------------------------------------------------------------------------------

const keyOf = (name: string) => FIELD_KEYS[name] ?? (name.startsWith('custom_fields.') ? `cf.${name.slice(14)}` : name);
const plainWord = (word: string) => word !== '' && !/[\s(),"\\]/u.test(word);

function freeWord(word: string): boolean {
  if (word.length > 1 && word.startsWith('"') && word.endsWith('"')) return !word.slice(1, -1).includes('"');
  return plainWord(word) && !word.startsWith('-') && !FIELD_MARKER.test(word) && !['OR', 'AND', 'NOT'].includes(word);
}

/** Writes a query in the field: value spelling, or undefined where no text spells its tree. */
export function formatQuery(q: Query): string | undefined {
  const parts: string[] = [];
  if (q.words) parts.push(q.words);
  if (q.kinds?.length) parts.push(`type: ${q.kinds.join(', ')}`);
  if (q.filter) {
    if (q.filter.op === 'AND') {
      const children = (q.filter.nodes ?? []) as FilterNode[];
      if (children.length < 2) return undefined;
      for (const child of children) {
        const text = formatNode(child, true, true);
        if (text === undefined) return undefined;
        parts.push(text);
      }
    } else {
      const text = formatNode(q.filter, true, false);
      if (text === undefined) return undefined;
      parts.push(text);
    }
  }
  if (q.include_archived) parts.push('is: archived');
  if (q.include_trashed) parts.push('is: trashed');
  if (q.sort?.length) {
    const terms: string[] = [];
    for (const term of q.sort as Record<string, unknown>[]) {
      if (typeof term.field !== 'string' || !term.field || term.nulls !== undefined) return undefined;
      if (term.dir !== undefined && term.dir !== 'DESC') return undefined;
      terms.push(`${term.dir === 'DESC' ? '-' : ''}${keyOf(term.field)}`);
    }
    parts.push(`sort: ${terms.join(', ')}`);
  }
  return parts.join(' ');
}

function formatNode(node: FilterNode, top: boolean, inAnd: boolean): string | undefined {
  const children = (node.nodes ?? []) as FilterNode[];
  switch (node.op) {
    case 'AND': {
      if (children.length < 2) return undefined;
      const parts = children.map((child) => formatNode(child, false, true));
      if (parts.some((part) => part === undefined)) return undefined;
      const text = parts.join(' ');
      return inAnd ? `(${text})` : text;
    }
    case 'OR': {
      const list = formatNoneList(node);
      if (list !== undefined) return list;
      if (children.length < 2) return undefined;
      const parts = children.map((child) => formatNode(child, false, false));
      if (parts.some((part) => part === undefined)) return undefined;
      return `(${parts.join(' OR ')})`;
    }
    case 'NOT': {
      if (children.length !== 1) return undefined;
      const negated = formatNegated(children[0] as FilterNode, top);
      if (negated !== undefined) return negated;
      const text = formatNode(children[0] as FilterNode, false, true);
      return text === undefined ? undefined : `NOT ${text}`;
    }
  }
  return formatLeaf(node, top, false);
}

function formatNoneList(node: FilterNode): string | undefined {
  const children = (node.nodes ?? []) as FilterNode[];
  if (children.length !== 2) return undefined;
  const [first, second] = children as [FilterNode, FilterNode];
  if (second.op !== 'IS_NULL' || first.field !== second.field || (first.op !== 'EQ' && first.op !== 'IN')) return undefined;
  if (['type', 'is_completed', 'blocked', 'blocking'].includes(String(first.field))) return undefined;
  const text = formatLeaf(first, false, false);
  return text === undefined ? undefined : `${text}, none`;
}

function formatNegated(node: FilterNode, top: boolean): string | undefined {
  const op = String(node.op);
  if (op === 'OR') {
    const list = formatNoneList(node);
    return list === undefined ? undefined : `-${list}`;
  }
  if (['AND', 'NOT', 'EQ', 'IN'].includes(op)) return undefined;
  if (node.field === 'text' && op === 'MATCHES' && !top && typeof node.value === 'string' && freeWord(node.value)) return `-${node.value}`;
  const text = formatLeaf(node, top, true);
  return text === undefined ? undefined : `-${text}`;
}

function formatLeaf(node: FilterNode, top: boolean, negated: boolean): string | undefined {
  const op = String(node.op);
  const name = String(node.field ?? '');
  const value = node.value;
  const field = fieldOf(name);
  if (!field) return undefined;
  if (['is_completed', 'blocked', 'blocking'].includes(name) && typeof value === 'boolean' && !negated && (op === 'EQ' || op === 'NEQ')) {
    const word = Object.entries(IS_WORDS).find(([, state]) => state.field === name && state.value === value)?.[0];
    if (word) return `${op === 'NEQ' ? '-' : ''}is: ${word}`;
  }
  if (name === 'type') {
    const list = Array.isArray(value);
    const levels = (list ? value : [value]).map((v) => Object.entries(ENTRY_LEVELS).find(([, level]) => level === v)?.[0]);
    if (levels.some((level) => level === undefined) || negated) return undefined;
    if ((op === 'EQ' && !list) || (op === 'IN' && list)) return `type: ${levels.join(', ')}`;
    if ((op === 'NEQ' && !list) || (op === 'NOT_IN' && list)) return `-type: ${levels.join(', ')}`;
    return undefined;
  }
  if (name === 'text' && op === 'MATCHES' && !top && !negated && typeof value === 'string' && freeWord(value)) return value;
  let prefix = `${keyOf(name)}: `;
  const standard = defaultOp(field);
  switch (op) {
    case 'IS_NULL':
      return `${prefix}none`;
    case 'BETWEEN': {
      if (!Array.isArray(value) || value.length !== 2) return undefined;
      if (field.kind === 'timestamp') {
        const period = Object.entries(PERIODS).find(([, span]) => span[0] === value[0] && span[1] === value[1])?.[0];
        if (period) return prefix + period;
        const low = String(value[0]);
        if (low.endsWith('T00:00:00Z') && DATE_ONLY.test(low.slice(0, 10)) && value[1] === `${low.slice(0, 10)}T23:59:59Z`) return prefix + low.slice(0, 10);
      }
      const low = formatValue(value[0], field, 'low', true);
      const high = formatValue(value[1], field, 'high', true);
      return low === undefined || high === undefined ? undefined : `${prefix}${low}..${high}`;
    }
    case 'LT':
    case 'LTE':
    case 'GT':
    case 'GTE': {
      const bound = op === 'LTE' || op === 'GT' ? 'high' : 'low';
      if (field.kind === 'timestamp' && !negated && typeof value === 'string') {
        const m = TODAY_OFFSET.exec(value);
        if (m && ((m[1] === '-' && op === 'GTE') || (m[1] === '+' && op === 'LTE'))) return `${prefix}${m[1]}${m[2]}${(m[3] ?? '').toLowerCase()}`;
      }
      const text = formatValue(value, field, bound, true);
      const sign = { LT: '<', LTE: '<=', GT: '>', GTE: '>=' }[op];
      return text === undefined ? undefined : `${prefix}${sign}${text}`;
    }
    case 'IN':
    case 'NOT_IN':
    case 'CONTAINS_ANY':
    case 'CONTAINS_ALL': {
      if (!Array.isArray(value) || value.length < 2) return undefined;
      if ((op === 'IN' || op === 'NOT_IN') && standard !== 'EQ') return undefined;
      if ((op === 'CONTAINS_ANY' || op === 'CONTAINS_ALL') && field.kind !== 'id_set') return undefined;
      const parts = value.map((v) => formatValue(v, field, '', false));
      if (parts.some((part) => part === undefined || part === 'none')) return undefined;
      if (op === 'NOT_IN') {
        if (negated) return undefined;
        prefix = `-${prefix}`;
      }
      return prefix + parts.join(op === 'CONTAINS_ALL' ? ' + ' : ', ');
    }
    case 'EQ':
    case 'NEQ':
    case 'CONTAINS':
    case 'STARTS_WITH':
    case 'MATCHES': {
      if (op === 'NEQ') {
        if (negated) return undefined;
        prefix = `-${prefix}`;
      }
      const text = formatValue(value, field, '', false);
      if (text === undefined || (field.kind === 'timestamp' && (op === 'EQ' || op === 'NEQ'))) return undefined;
      const effective = op === 'NEQ' ? 'EQ' : op;
      if (effective === standard) return text === 'none' || field.kind === 'timestamp' ? undefined : prefix + text;
      if (effective === 'EQ') return `${prefix}=${text}`;
      if (effective === 'CONTAINS') return `${prefix}~${text}`;
      if (effective === 'STARTS_WITH') return text.startsWith('"') ? undefined : `${prefix}${text}*`;
    }
  }
  return undefined;
}

function formatValue(raw: unknown, field: Field, bound: string, inRange: boolean): string | undefined {
  if (typeof raw === 'boolean') return String(raw);
  if (typeof raw === 'number') return String(raw);
  if (typeof raw !== 'string') return undefined;
  if (raw.startsWith('@') && ['timestamp', 'id', 'id_set'].includes(field.kind)) return formatPlaceholder(raw);
  if (field.kind === 'timestamp') {
    const day = raw.slice(0, 10);
    if (raw === `${day}T00:00:00Z` && DATE_ONLY.test(day) && bound !== 'high') return day;
    if (raw === `${day}T23:59:59Z` && DATE_ONLY.test(day) && bound === 'high') return day;
    return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/.test(raw) ? raw : undefined;
  }
  if (field.kind === 'enum') return LOWERED_ENUMS.has(field.name) ? raw.toLowerCase() : raw;
  if (field.kind === 'boolean') return undefined;
  if (bareSafe(raw, field)) return raw;
  if (inRange) return undefined;
  return `"${raw.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"`;
}

function bareSafe(value: string, field: Field): boolean {
  if (!plainWord(value) || value.includes('..') || value.endsWith('*') || '<>=~-+@'.includes(value[0] ?? '') || FIELD_MARKER.test(value)) return false;
  if (['none', 'me', 'my-groups', 'here', 'OR', 'AND', 'NOT', '+'].includes(value)) return false;
  if (field.kind === 'custom' && typeof bareCustom(value) !== 'string') return false;
  return field.kind !== 'integer';
}

function formatPlaceholder(value: string): string {
  if (value === '@me') return 'me';
  if (value === '@my_groups') return 'my-groups';
  if (value.startsWith('@group:')) return `${MEMBERS_FUNCTION}("${value.slice(7).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}")`;
  const word = Object.entries(ANCHORS).find(([, anchor]) => anchor === value)?.[0];
  if (word) return word;
  const m = TODAY_OFFSET.exec(value);
  return m ? `${m[1]}${m[2]}${(m[3] ?? '').toLowerCase()}` : value;
}
