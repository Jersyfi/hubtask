// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The filter language: one string that carries both the words and the narrowing (ADR-0066).
 *
 * Today the search screen asks two questions through two controls: a field for the words, and six
 * chips for the narrowing. The chips can say six things, and the server can already answer far
 * more than six — `POST /search` takes the whole filter grammar of ADR-0026 with its seventeen
 * operators, the server-resolved placeholders (`@me`, `@today+P3D`, `@end_of_week`), a sort for a
 * search with no words, and the two flags that widen it to the archive and the trash (ADR-0064).
 * None of that is reachable from the interface. This module is the missing half: a small language
 * the line accepts, compiled into the request the contract already documents.
 *
 * **Three rules it keeps, and they are the reason it is shaped like this.**
 *
 * 1. *Nothing is invented.* Every token below compiles to a `FilterNode` over a field the
 *    installation reports in `/meta/capabilities`, or to a flag `ItemSearchQuery` declares. A
 *    token whose field is not reported is **refused by name** rather than sent — the rule
 *    `data/query.ts` already lives by, for the same reason: a filter the server answers
 *    `query.field_unknown` to is a request the reader gets nothing back from.
 * 2. *Content and structure are separated, because the address separates them.* `/search` is a
 *    `POST` with no `GET` so that what somebody is looking for never reaches an access log
 *    (`security.md` §9), and `searchhandle.ts` carries that into the client: the address holds the
 *    narrowing, the words live under a minted handle. A line mixes the two, so it is split before
 *    it is stored — `structural()` is what may travel in a link, `content()` is what may not.
 * 3. *No date is computed here.* `due:week` becomes `@end_of_week`, which the server resolves in
 *    the actor's zone and the actor's week start. A client that computed it would be wrong for
 *    everybody travelling, and wrong every Sunday for half of Europe.
 *
 * Pure, and tested beside itself: nothing here reads the manifest, the clock or the DOM. The
 * caller hands in what it knows.
 */

/** One `key:value` a reader wrote, with the `-` that may precede it. */
export interface Token {
  readonly key: string;
  readonly value: string;
  readonly negated: boolean;
  /** Where it sits in the line, so an editor can replace exactly it. */
  readonly from: number;
  readonly to: number;
}

/** A line, read: the words that are left over, and the tokens taken out of it. */
export interface Parsed {
  readonly words: string;
  readonly tokens: readonly Token[];
}

/** Something the line asks for and cannot have, as a code and its parameters (ADR-0011). */
export interface Problem {
  readonly code: string;
  readonly params?: Readonly<Record<string, string>>;
}

/** A filter node, in the shape the contract names (`FilterNode`, ADR-0026). */
export interface Node {
  readonly op: string;
  readonly field?: string;
  readonly value?: unknown;
  readonly nodes?: readonly Node[];
}

export interface SortTerm {
  readonly field: string;
  readonly direction: 'ASC' | 'DESC';
}

/** What a line compiles to: the body of `POST /search`, minus the page and the language. */
export interface Compiled {
  readonly q?: string;
  readonly filter?: Node;
  readonly sort?: readonly SortTerm[];
  readonly includeArchived: boolean;
  readonly includeTrashed: boolean;
  readonly problems: readonly Problem[];
}

/**
 * What the caller knows and this module does not: which fields the installation offers, and what
 * the names in `in:` and `label:` stand for.
 *
 * Handed in rather than reached for, exactly as `search.svelte.ts` takes the reader's languages:
 * the manifest is read once, in one place, and a data module with a second opinion about the
 * installation is a second answer to the same question.
 */
export interface Context {
  readonly fields: ReadonlySet<string>;
  /** A collection's identifier, by the name a reader would type. Case-insensitive, caller's rule. */
  readonly collection: (name: string) => string | undefined;
  /** A label's identifier within the collections the line named. Absent until one is named. */
  readonly label: (name: string) => string | undefined;
  /** Whether a collection has been named, which is what makes `label:` answerable at all. */
  readonly hasCollection: boolean;
  /**
   * Somebody's identifier, by the name a reader would type — optional, and absent means "as
   * written".
   *
   * The same shape as `collection`, and for the same reason: a line with a UUID in it is a line
   * nobody can read back, so the chip that names a person writes their name and this turns it into
   * what the grammar compares. `me` and `none` never reach here — they are the grammar's own words.
   */
  readonly person?: (name: string) => string | undefined;
}

// ---------------------------------------------------------------------------------------------
// The vocabulary
// ---------------------------------------------------------------------------------------------

/**
 * The keys the line knows, and the field each one needs.
 *
 * The names are the reader's rather than the column's — `due`, not `due_at`; `who`, not
 * `assignee_id` — because a line is written by a person and read by one. The mapping to the
 * grammar is here and nowhere else, so a field the installation renames or withdraws costs one
 * row.
 */
export const KEYS = {
  is: 'is_completed',
  type: 'type',
  who: 'assignee_id',
  with: 'members',
  by: 'created_by',
  due: 'due_at',
  start: 'start_at',
  created: 'created_at',
  updated: 'updated_at',
  done: 'completed_at',
  in: 'collection_id',
  label: 'labels',
  title: 'title',
  note: 'notes',
  sort: '',
} as const;

export type Key = keyof typeof KEYS;

const KEY_NAMES = Object.keys(KEYS) as Key[];

/** The keys whose value is somebody's content, and therefore may never reach a link. */
const CONTENT_KEYS: ReadonlySet<string> = new Set(['title', 'note']);

/** The date keys, which all read the same way over different fields. */
const DATE_KEYS: ReadonlySet<string> = new Set(['due', 'start', 'created', 'updated', 'done']);

/** The words a date key accepts instead of a comparison. */
const WHENS: Readonly<Record<string, Node | undefined>> = {
  overdue: { op: 'LT', value: '@today' },
  today: { op: 'LTE', value: '@today' },
  week: { op: 'LTE', value: '@end_of_week' },
  month: { op: 'LTE', value: '@end_of_month' },
  none: { op: 'IS_NULL' },
};

/** What `is:` answers. The first two are a filter; the last two widen what the search sees. */
const STATES = ['open', 'done', 'archived', 'trashed'] as const;

/** What `sort:` may order by, as the reader's word and the field it means. */
export const SORTS: Readonly<Record<string, string>> = {
  due: 'due_at',
  start: 'start_at',
  created: 'created_at',
  updated: 'updated_at',
  done: 'completed_at',
  title: 'title',
};

/** The entry kinds `type:` accepts, as the reader writes them. */
const TYPES: Readonly<Record<string, string>> = {
  task: 'TASK',
  work_package: 'WORK_PACKAGE',
  activity: 'ACTIVITY',
};

// ---------------------------------------------------------------------------------------------
// Reading a line
// ---------------------------------------------------------------------------------------------

/**
 * Splits a line into its parts, keeping a quoted run together.
 *
 * Quotes are the one piece of punctuation a search box has always had, and the words half of this
 * line hands them straight to `websearch_to_tsquery`, which has them too. So the splitting keeps
 * them where they were typed rather than stripping them: a quoted phrase stays a quoted phrase in
 * the words, and a quoted value loses its quotes because a value is not a phrase.
 */
function chunks(line: string): { text: string; from: number; to: number }[] {
  const out: { text: string; from: number; to: number }[] = [];
  let start = -1;
  let quoted = false;
  for (let index = 0; index <= line.length; index += 1) {
    const char = line[index];
    const ends = char === undefined || (!quoted && /\s/.test(char));
    if (char === '"') quoted = !quoted;
    if (ends) {
      if (start >= 0) out.push({ text: line.slice(start, index), from: start, to: index });
      start = -1;
      continue;
    }
    if (start < 0) start = index;
  }
  return out;
}

const TOKEN = /^(-?)([a-z][a-z_]*):(.*)$/;

/**
 * Reads a line into words and tokens.
 *
 * A chunk that looks like a token but names a key this client does not know is **left in the
 * words**, deliberately. `http://example.invalid` and `note: remember this` are things people type
 * into search boxes, and a reader who wrote one meant to search for it; refusing it would make the
 * line a language you can get wrong rather than a field you can type in.
 */
export function parse(line: string): Parsed {
  const tokens: Token[] = [];
  const words: string[] = [];

  for (const chunk of chunks(line)) {
    const match = TOKEN.exec(chunk.text);
    const key = match?.[2];
    if (!match || !key || !(KEY_NAMES as readonly string[]).includes(key)) {
      words.push(chunk.text);
      continue;
    }
    tokens.push({
      key,
      value: unquote(match[3] ?? ''),
      negated: match[1] === '-',
      from: chunk.from,
      to: chunk.to,
    });
  }

  return { words: words.join(' ').trim(), tokens };
}

function unquote(value: string): string {
  return value.startsWith('"') && value.endsWith('"') && value.length > 1
    ? value.slice(1, -1)
    : value;
}

function quote(value: string): string {
  return /[\s"]/.test(value) ? `"${value.replace(/"/g, '')}"` : value;
}

/** One token, written back out the way a reader would have typed it. */
export function write(token: Pick<Token, 'key' | 'value' | 'negated'>): string {
  return `${token.negated ? '-' : ''}${token.key}:${quote(token.value)}`;
}

// ---------------------------------------------------------------------------------------------
// Splitting content from structure
// ---------------------------------------------------------------------------------------------

/**
 * The half of a line that may travel in a link: the tokens whose values are structural.
 *
 * A collection, a label, a kind, a state, a date, `me` — none of them is anybody's text. What is
 * left out is the words and `title:`/`note:`, which are, and which stay under the handle in
 * `sessionStorage` where the words already live (`searchhandle.ts`, issue 997).
 */
export function structural(parsed: Parsed): string {
  return parsed.tokens.filter((token) => !CONTENT_KEYS.has(token.key)).map(write).join(' ');
}

/** And the half that may not: the words, and the tokens that quote somebody's text. */
export function content(parsed: Parsed): string {
  const quoted = parsed.tokens.filter((token) => CONTENT_KEYS.has(token.key)).map(write);
  return [parsed.words, ...quoted].filter(Boolean).join(' ');
}

/**
 * Adds a token to a line, or takes it away again — what a chip press does.
 *
 * The same value twice is a removal, which is what makes a chip a toggle; a key that may only be
 * answered once (`is:`, `sort:`) replaces its answer instead of gathering a second one.
 */
const SINGULAR: ReadonlySet<string> = new Set(['sort']);

export function toggle(line: string, key: Key, value: string, negated = false): string {
  const parsed = parse(line);
  const same = (token: Token) =>
    token.key === key && token.value === value && token.negated === negated;
  const present = parsed.tokens.some(same);
  const kept = parsed.tokens.filter((token) =>
    present ? !same(token) : !(SINGULAR.has(key) && token.key === key),
  );
  const next = present ? kept : [...kept, { key, value, negated }];
  return [...next.map(write), parsed.words].filter(Boolean).join(' ').trim();
}

/** Whether a line already asks this, which is what a chip is drawn from. */
export function has(parsed: Parsed, key: Key, value: string, negated = false): boolean {
  return parsed.tokens.some(
    (token) => token.key === key && token.value === value && token.negated === negated,
  );
}

/** What one question is currently answered with, which is the number a chip carries. */
export function values(parsed: Parsed, key: Key): readonly string[] {
  return parsed.tokens.filter((token) => token.key === key).map(write);
}

/** Empties one question, keeping the rest of the line — the chip's "clear". */
export function clear(line: string, key: Key): string {
  const parsed = parse(line);
  const kept = parsed.tokens.filter((token) => token.key !== key);
  return [...kept.map(write), parsed.words].filter(Boolean).join(' ').trim();
}

/** The words alone, with every token taken out: what a field for words would hold. */
export function wordsOf(line: string): string {
  return parse(line).words;
}

/**
 * The narrowing alone, as a line — everything but the words.
 *
 * The pair `wordsOf`/`narrowingOf` is what lets one string be edited through two surfaces: a field
 * that holds only words beside chips that hold only tokens, and a single field holding both. The
 * string is the state either way, so neither surface can disagree with the other.
 */
export function narrowingOf(line: string): string {
  return parse(line).tokens.map(write).join(' ');
}

/** Words and a narrowing, joined back into the one line that is the state. */
export function joinLine(narrowing: string, words: string): string {
  return [narrowing.trim(), words.trim()].filter(Boolean).join(' ');
}

// ---------------------------------------------------------------------------------------------
// The few that answer most of the questions
// ---------------------------------------------------------------------------------------------

/**
 * The narrowings offered where there is no room to build one: in the bar's menu, and as the first
 * row of the filter bar.
 *
 * Six, and the number is the point. Every product that has a filter surface also has this list —
 * Jira's starred filters, Linear's saved views, Todoist's filters — because the long tail of
 * filters is built once and the head of it is pressed every day. What makes these safe to hard-code
 * is that each one is a *line*, in the language the filter bar and the field already speak: pressing
 * one is indistinguishable from having typed it, and it can be edited afterwards like anything
 * somebody typed themselves. A quick filter that opened a mode nobody could edit would be the
 * fourth filter vocabulary this client does not need.
 */
export interface Quick {
  /** The message code for its word (ADR-0011). */
  readonly code: string;
  /** The line it stands for. */
  readonly line: string;
  /** The field it needs, so one the installation does not report is not offered. */
  readonly field: string;
}

export const QUICK: readonly Quick[] = [
  { code: 'app.searchline.quick.mine_open', line: 'who:me is:open', field: 'assignee_id' },
  { code: 'app.searchline.quick.due_week', line: 'is:open due:week', field: 'due_at' },
  { code: 'app.searchline.quick.overdue', line: 'is:open due:overdue', field: 'due_at' },
  { code: 'app.searchline.quick.unassigned', line: 'who:none is:open', field: 'assignee_id' },
  { code: 'app.searchline.quick.recent', line: 'updated:week sort:-updated', field: 'updated_at' },
  { code: 'app.searchline.quick.done', line: 'is:done sort:-done', field: 'is_completed' },
];

/** Whether a line is exactly one quick filter's, which is what draws it as the chosen one. */
export function isQuick(line: string, quick: Quick): boolean {
  const asked = parse(line);
  const wanted = parse(quick.line);
  if (asked.words !== '' || asked.tokens.length !== wanted.tokens.length) return false;
  return wanted.tokens.every((token) => has(asked, token.key as Key, token.value, token.negated));
}

// ---------------------------------------------------------------------------------------------
// Compiling
// ---------------------------------------------------------------------------------------------

/**
 * Turns a read line into the body of `POST /search`.
 *
 * An AND of one node per question, and within one question an OR of its answers — several values
 * of one question widen it, several questions narrow it, which is the arithmetic the chips already
 * use and the one people expect from every filter they have ever used.
 *
 * Anything it cannot honour is a `Problem` rather than a silent omission. That is the half of
 * `query.ts`'s rule this inherits: a condition the editor allowed and the document dropped would
 * be a narrowing the reader can see and the server never received.
 */
export function compile(parsed: Parsed, context: Context): Compiled {
  const problems: Problem[] = [];
  const grouped = new Map<string, Node[]>();
  let includeArchived = false;
  let includeTrashed = false;
  const sort: SortTerm[] = [];

  for (const token of parsed.tokens) {
    if (token.key === 'sort') {
      const term = sortTerm(token, problems);
      if (term) sort.push(term);
      continue;
    }
    if (token.key === 'is' && (token.value === 'archived' || token.value === 'trashed')) {
      if (token.value === 'archived') includeArchived = true;
      else includeTrashed = true;
      // `archived` is also a question about the entry, and the catalogue can answer it: an entry
      // that is archived is one whose `archived_at` is set. The trash has no such field - being
      // deleted is not something the grammar may ask about - so `is:trashed` widens the search and
      // narrows nothing, and the line says so rather than pretending otherwise.
      if (token.value === 'archived' && permits('archived_at', context, problems, token)) {
        push(grouped, 'archived_at', negate({ op: 'IS_NULL', field: 'archived_at' }, !token.negated));
      }
      continue;
    }

    const field = KEYS[token.key as Key];
    if (!permits(field, context, problems, token)) continue;

    const node = nodeFor(token, field, context, problems);
    if (node) push(grouped, `${token.key}:${token.negated}`, node);
  }

  const parts: Node[] = [];
  for (const nodes of grouped.values()) {
    parts.push(nodes.length === 1 ? (nodes[0] as Node) : { op: 'OR', nodes });
  }

  const words = parsed.words.trim();
  // The server refuses a sort beside words by name, because with words there *is* a ranking and it
  // is the ranking. Pre-empted here so the reader is told by the line rather than by a failure.
  if (words !== '' && sort.length > 0) problems.push({ code: 'app.searchline.sort_with_words' });

  return {
    q: words === '' ? undefined : words,
    filter: parts.length === 0 ? undefined : parts.length === 1 ? parts[0] : { op: 'AND', nodes: parts },
    sort: words === '' && sort.length > 0 ? sort : undefined,
    includeArchived,
    includeTrashed,
    problems,
  };
}

function push(grouped: Map<string, Node[]>, key: string, node: Node): void {
  const held = grouped.get(key);
  if (held) held.push(node);
  else grouped.set(key, [node]);
}

/** Whether the installation offers the field this token needs. */
function permits(
  field: string,
  context: Context,
  problems: Problem[],
  token: Token,
): boolean {
  if (field === '' || context.fields.has(field)) return true;
  problems.push({ code: 'app.searchline.field_absent', params: { key: token.key } });
  return false;
}

/** `NOT` around a node, or the node itself. One wrapper, so `-` reads the same everywhere. */
function negate(node: Node, negated: boolean): Node {
  return negated ? { op: 'NOT', nodes: [node] } : node;
}

function nodeFor(token: Token, field: string, context: Context, problems: Problem[]): Node | undefined {
  const value = token.value.trim();
  if (value === '') {
    problems.push({ code: 'app.searchline.value_missing', params: { key: token.key } });
    return undefined;
  }

  if (DATE_KEYS.has(token.key)) return negate2(dateNode(token.key, field, value, problems), token);

  switch (token.key) {
    case 'is': {
      if (value === 'open') return negate({ op: 'EQ', field, value: false }, token.negated);
      if (value === 'done') return negate({ op: 'EQ', field, value: true }, token.negated);
      problems.push({
        code: 'app.searchline.value_unknown',
        params: { key: token.key, value, expected: STATES.join(', ') },
      });
      return undefined;
    }
    case 'type': {
      const kinds = list(value).map((each) => TYPES[each.toLowerCase()]);
      if (kinds.some((kind) => kind === undefined)) {
        problems.push({
          code: 'app.searchline.value_unknown',
          params: { key: token.key, value, expected: Object.keys(TYPES).join(', ') },
        });
        return undefined;
      }
      return negate({ op: 'IN', field, value: kinds as string[] }, token.negated);
    }
    case 'who':
    case 'by': {
      if (value === 'none') {
        return negate({ op: 'IS_NULL', field }, token.negated);
      }
      const who = person(value, context, problems);
      return who === undefined ? undefined : negate({ op: 'EQ', field, value: who }, token.negated);
    }
    case 'with': {
      const who = person(value, context, problems);
      return who === undefined ? undefined : negate({ op: 'CONTAINS', field, value: who }, token.negated);
    }
    case 'in': {
      const wanted = list(value);
      const ids = wanted.map((name) => context.collection(name));
      const missing = wanted.filter((_, index) => ids[index] === undefined);
      if (missing.length > 0) {
        problems.push({ code: 'app.searchline.place_unknown', params: { value: missing.join(', ') } });
        return undefined;
      }
      return negate({ op: 'IN', field, value: ids as string[] }, token.negated);
    }
    case 'label': {
      if (!context.hasCollection) {
        problems.push({ code: 'app.searchline.label_needs_place' });
        return undefined;
      }
      const wanted = list(value);
      const ids = wanted.map((name) => context.label(name));
      const missing = wanted.filter((_, index) => ids[index] === undefined);
      if (missing.length > 0) {
        problems.push({ code: 'app.searchline.label_unknown', params: { value: missing.join(', ') } });
        return undefined;
      }
      return negate({ op: 'CONTAINS_ANY', field, value: ids as string[] }, token.negated);
    }
    case 'title':
    case 'note':
      return negate({ op: 'CONTAINS', field, value }, token.negated);
    default:
      return undefined;
  }
}

/** A date node is negated by the caller, because `IS_NULL` and a comparison negate alike. */
function negate2(node: Node | undefined, token: Token): Node | undefined {
  return node === undefined ? undefined : negate(node, token.negated);
}

/**
 * `due:today`, `due:<2026-10-01`, `due:>@today+P7D`, `due:2026-10-01..2026-10-31`.
 *
 * The anchors travel unresolved. `@end_of_week` is the server's answer in the actor's zone and the
 * actor's week start, and a client that wrote the date out would have a query that quietly went
 * stale — which is the sentence `Placeholder.go` opens with.
 */
function dateNode(key: string, field: string, value: string, problems: Problem[]): Node | undefined {
  const when = WHENS[value];
  if (when) return { ...when, field };

  const range = value.split('..');
  if (range.length === 2 && range[0] && range[1]) {
    if (!isMoment(range[0]) || !isMoment(range[1])) {
      problems.push({
        code: 'app.searchline.value_unknown',
        params: { key, value, expected: Object.keys(WHENS).join(', ') },
      });
      return undefined;
    }
    return { op: 'BETWEEN', field, value: [moment(range[0]), moment(range[1])] };
  }

  const match = /^(<=|>=|<|>)?(.+)$/.exec(value);
  const bound = match?.[2];
  // A word that is neither one of the five nor a date is **refused**, not sent. `due:wee` on its
  // way to `due:week` would otherwise compile to a comparison against the text "wee" — a filter
  // the server accepts, answers with nothing, and gives the reader no reason for.
  if (!bound || !isMoment(bound)) {
    problems.push({
      code: 'app.searchline.value_unknown',
      params: { key, value, expected: Object.keys(WHENS).join(', ') },
    });
    return undefined;
  }
  const op = { '<': 'LT', '<=': 'LTE', '>': 'GT', '>=': 'GTE' }[match?.[1] ?? ''] ?? 'LTE';
  return { op, field, value: moment(bound) };
}

/** An anchor, a date, or a full timestamp. Nothing else is a moment. */
function isMoment(raw: string): boolean {
  const value = raw.trim();
  if (value.startsWith('@')) return /^@[a-z_]+([+-]P[0-9A-Z]+)?$/.test(value);
  return /^\d{4}-\d{2}-\d{2}(T[\d:.]+(Z|[+-]\d{2}:\d{2})?)?$/.test(value);
}

/**
 * A moment, as the request carries it: an anchor, or a date the reader wrote.
 *
 * A bare date is widened to the whole day's start, which is what somebody writing `2026-10-01`
 * means — the server compares an instant, and `LTE 2026-10-01T00:00:00Z` on a date-only value is
 * what every other client of this grammar sends.
 */
function moment(raw: string): string {
  const value = raw.trim();
  if (value.startsWith('@')) return value;
  return /^\d{4}-\d{2}-\d{2}$/.test(value) ? `${value}T00:00:00Z` : value;
}

/** `me` is the server's own placeholder; anything else is an identifier the caller resolved. */
/**
 * Whose work: `me` is the server's placeholder, a name is looked up, and anything the caller can
 * make nothing of is **refused by name** rather than sent as itself.
 *
 * Sent as itself is what the prototype did first, and it is the quiet failure this module exists
 * to avoid: `who:anna` became `EQ assignee_id 'anna'`, which the server answers with an empty page
 * rather than a complaint, and the reader reads "nothing matches" about a question nobody asked.
 */
function person(value: string, context: Context, problems: Problem[]): string | undefined {
  if (value === 'me') return '@me';
  const found = context.person?.(value);
  if (found !== undefined) return found;
  // No resolver at all is the pure case the tests use: the value stands for itself.
  if (context.person === undefined) return value;
  problems.push({ code: 'app.searchline.person_unknown', params: { value } });
  return undefined;
}

function list(value: string): string[] {
  return value.split(',').map((each) => each.trim()).filter(Boolean);
}

function sortTerm(token: Token, problems: Problem[]): SortTerm | undefined {
  const descending = token.value.startsWith('-') || token.negated;
  const name = token.value.replace(/^-/, '').trim();
  const field = SORTS[name];
  if (!field) {
    problems.push({
      code: 'app.searchline.value_unknown',
      params: { key: 'sort', value: token.value, expected: Object.keys(SORTS).join(', ') },
    });
    return undefined;
  }
  return { field, direction: descending ? 'DESC' : 'ASC' };
}

/** Whether a line asks anything at all. Neither words nor a filter is the empty screen, not a search. */
export function asksSomething(compiled: Compiled): boolean {
  return compiled.q !== undefined || compiled.filter !== undefined;
}
