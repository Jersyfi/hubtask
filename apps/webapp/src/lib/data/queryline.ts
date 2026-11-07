// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The query as one line (api-guidelines.md §3, "The query as a line"), the filter bar's text mode
 * (UC-WRK-17 check 12): the browser's twin of `core/domain/model/view/Line.go`, so the line reads
 * and writes offline too. Like the server's, it is only a spelling - what a line may compare is
 * the filter's business, checked by `query.ts` against the manifest and by the server again.
 */

import type { FilterNode } from '@hubtask/sync-engine';

export interface LineSort {
  readonly field: string;
  readonly dir?: 'ASC' | 'DESC';
  readonly nulls?: 'FIRST' | 'LAST';
}

export type LineRead =
  | { readonly ok: true; readonly filter: FilterNode | undefined; readonly sort: readonly LineSort[] }
  /** `at` is the character the line stops being readable at, the first being 1. */
  | { readonly ok: false; readonly at: number };

const MAX_LENGTH = 4000;
const MAX_NESTING = 10;
const MAX_VALUES = 100;
const MAX_SORT = 4;
const STOP = "()'\",~=!<>";

const KEYWORDS = new Set([
  'AND', 'OR', 'NOT', 'IN', 'IS', 'EMPTY', 'NULL', 'BETWEEN', 'CONTAINS', 'ANY', 'ALL', 'STARTS',
  'WITH', 'MATCHES', 'ORDER', 'BY', 'ASC', 'DESC', 'NULLS', 'FIRST', 'LAST',
]);

const SYMBOLS: Record<string, FilterNode['op']> = {
  '=': 'EQ', '!=': 'NEQ', '<': 'LT', '<=': 'LTE', '>': 'GT', '>=': 'GTE', '~': 'CONTAINS',
};

interface Token { readonly kind: 'word' | 'text' | 'symbol'; readonly text: string; readonly at: number }

class Stop extends Error {
  readonly at: number;
  constructor(at: number) { super('line'); this.at = at; }
}

const isKeyword = (word: string) => KEYWORDS.has(word.toUpperCase());
const isSpace = (c: string) => /\s/u.test(c);

function lex(line: string): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  while (i < line.length) {
    const c = line[i]!;
    if (isSpace(c)) { i++; continue; }
    if (c === "'" || c === '"') {
      let text = '';
      let j = i + 1;
      for (; j < line.length && line[j] !== c; j++) {
        if (line[j] === '\\' && j + 1 < line.length) j++;
        text += line[j];
      }
      if (j >= line.length) throw new Stop(i);
      tokens.push({ kind: 'text', text, at: i });
      i = j + 1;
      continue;
    }
    if ('(),~'.includes(c)) { tokens.push({ kind: 'symbol', text: c, at: i }); i++; continue; }
    if ('=!<>'.includes(c)) {
      const symbol = line[i + 1] === '=' ? `${c}=` : c;
      if (symbol === '!') throw new Stop(i);
      tokens.push({ kind: 'symbol', text: symbol, at: i });
      i += symbol.length;
      continue;
    }
    const start = i;
    while (i < line.length && !isSpace(line[i]!) && !STOP.includes(line[i]!)) i++;
    tokens.push({ kind: 'word', text: line.slice(start, i), at: start });
  }
  return tokens;
}

/** A bare word as the document holds it: a number or a flag where it reads as one. */
function bare(word: string, kind: string | undefined): unknown {
  if (kind === 'integer' && /^-?\d+$/u.test(word)) return Number(word);
  if (kind === 'boolean' && (word === 'true' || word === 'false')) return word === 'true';
  if (kind === 'custom' || kind === 'number') {
    if (word === 'true' || word === 'false') return word === 'true';
    if (word !== '' && Number.isFinite(Number(word))) return Number(word);
  }
  return word;
}

/**
 * Reads a line into the filter and the order. `kindOf` names a field's kind as the manifest
 * reports it (`integer`, `boolean`, `number` for a custom field), which decides how a bare word is
 * read; a field it does not know reads as text, and is refused where it is sent.
 */
export function readLine(line: string, kindOf: (field: string) => string | undefined = () => undefined): LineRead {
  if (line.length > MAX_LENGTH) return { ok: false, at: MAX_LENGTH + 1 };
  let tokens: Token[];
  try {
    tokens = lex(line);
  } catch (stop) {
    if (stop instanceof Stop) return { ok: false, at: [...line.slice(0, stop.at)].length + 1 };
    throw stop;
  }
  let pos = 0;
  const peek = (): Token => tokens[pos] ?? { kind: 'symbol', text: '', at: line.length };
  const done = () => pos >= tokens.length;
  const next = () => tokens[pos++] ?? peek();
  const fail = (): never => { throw new Stop(peek().at); };
  const atKeyword = (word: string) => peek().kind === 'word' && peek().text.toUpperCase() === word;
  const atSymbol = (symbol: string) => !done() && peek().kind === 'symbol' && peek().text === symbol;
  const expectKeyword = (word: string) => { if (!atKeyword(word)) fail(); next(); };

  const value = (field: string): unknown => {
    const token = peek();
    if (token.kind === 'text') { next(); return token.text; }
    if (token.kind === 'word' && !isKeyword(token.text)) { next(); return bare(token.text, kindOf(field)); }
    return fail();
  };
  const list = (field: string, op: FilterNode['op']): FilterNode => {
    if (!atSymbol('(')) fail();
    next();
    const values: unknown[] = [];
    for (;;) {
      if (values.length > MAX_VALUES) fail();
      values.push(value(field));
      if (atSymbol(',')) { next(); continue; }
      if (!atSymbol(')')) fail();
      next();
      return { field, op, value: values } as FilterNode;
    }
  };
  const comparison = (): FilterNode => {
    const name = peek();
    if (name.kind !== 'word' || isKeyword(name.text)) fail();
    next();
    const field = name.text;
    const op = peek();
    if (op.kind === 'symbol' && SYMBOLS[op.text]) {
      next();
      return { field, op: SYMBOLS[op.text]!, value: value(field) } as FilterNode;
    }
    if (atKeyword('IN')) { next(); return list(field, 'IN'); }
    if (atKeyword('NOT')) { next(); expectKeyword('IN'); return list(field, 'NOT_IN'); }
    if (atKeyword('IS')) {
      next();
      const negated = atKeyword('NOT');
      if (negated) next();
      if (!atKeyword('EMPTY') && !atKeyword('NULL')) fail();
      next();
      const leaf = { field, op: 'IS_NULL' } as FilterNode;
      return negated ? ({ op: 'NOT', nodes: [leaf] } as FilterNode) : leaf;
    }
    if (atKeyword('BETWEEN')) {
      next();
      const low = value(field);
      expectKeyword('AND');
      return { field, op: 'BETWEEN', value: [low, value(field)] } as FilterNode;
    }
    if (atKeyword('CONTAINS')) {
      next();
      if (atKeyword('ANY')) { next(); return list(field, 'CONTAINS_ANY'); }
      if (atKeyword('ALL')) { next(); return list(field, 'CONTAINS_ALL'); }
      return { field, op: 'CONTAINS', value: value(field) } as FilterNode;
    }
    if (atKeyword('STARTS')) { next(); expectKeyword('WITH'); return { field, op: 'STARTS_WITH', value: value(field) } as FilterNode; }
    if (atKeyword('MATCHES')) { next(); return { field, op: 'MATCHES', value: value(field) } as FilterNode; }
    return fail();
  };
  const not = (nesting: number): FilterNode => {
    if (atKeyword('NOT')) {
      next();
      if (nesting >= MAX_NESTING) fail();
      return { op: 'NOT', nodes: [not(nesting + 1)] } as FilterNode;
    }
    if (atSymbol('(')) {
      next();
      if (nesting >= MAX_NESTING) fail();
      const node = or(nesting + 1);
      if (!atSymbol(')')) fail();
      next();
      return node;
    }
    return comparison();
  };
  const combination = (op: 'AND' | 'OR', nesting: number, operand: (n: number) => FilterNode): FilterNode => {
    const nodes = [operand(nesting)];
    while (atKeyword(op)) { next(); nodes.push(operand(nesting)); }
    return nodes.length === 1 ? nodes[0]! : ({ op, nodes } as FilterNode);
  };
  const and = (nesting: number) => combination('AND', nesting, not);
  const or = (nesting: number): FilterNode => combination('OR', nesting, and);

  try {
    let filter: FilterNode | undefined;
    if (!atKeyword('ORDER') && !done()) filter = or(0);
    const sort: LineSort[] = [];
    if (atKeyword('ORDER')) {
      next();
      expectKeyword('BY');
      for (;;) {
        const name = peek();
        if (name.kind !== 'word' || isKeyword(name.text) || sort.length > MAX_SORT) fail();
        next();
        let term: LineSort = { field: name.text };
        if (atKeyword('ASC') || atKeyword('DESC')) term = { ...term, dir: next().text.toUpperCase() as 'ASC' | 'DESC' };
        if (atKeyword('NULLS')) {
          next();
          if (!atKeyword('FIRST') && !atKeyword('LAST')) fail();
          term = { ...term, nulls: next().text.toUpperCase() as 'FIRST' | 'LAST' };
        }
        sort.push(term);
        if (!atSymbol(',')) break;
        next();
      }
    }
    if (!done()) fail();
    return { ok: true, filter, sort };
  } catch (stop) {
    if (stop instanceof Stop) return { ok: false, at: [...line.slice(0, stop.at)].length + 1 };
    throw stop;
  }
}

const WORDS: Partial<Record<FilterNode['op'], string>> = {
  EQ: '=', NEQ: '!=', LT: '<', LTE: '<=', GT: '>', GTE: '>=', CONTAINS: 'CONTAINS', STARTS_WITH: 'STARTS WITH',
  MATCHES: 'MATCHES', IN: 'IN', NOT_IN: 'NOT IN', CONTAINS_ANY: 'CONTAINS ANY', CONTAINS_ALL: 'CONTAINS ALL',
};

const bareField = (name: string | undefined): name is string =>
  !!name && !isKeyword(name) && ![...name].some((c) => isSpace(c) || STOP.includes(c));

function valueText(value: unknown): string {
  if (typeof value === 'boolean' || typeof value === 'number') return String(value);
  return `'${String(value ?? '').replaceAll('\\', '\\\\').replaceAll("'", "\\'")}'`;
}

function nodeText(node: FilterNode, nested: boolean): string | undefined {
  if (node.op === 'AND' || node.op === 'OR') {
    const parts = (node.nodes ?? []).map((child) => nodeText(child, true));
    if (parts.length < 2 || parts.some((part) => part === undefined)) return undefined;
    const joined = parts.join(` ${node.op} `);
    return nested ? `(${joined})` : joined;
  }
  if (node.op === 'NOT') {
    const child = node.nodes?.length === 1 ? nodeText(node.nodes[0]!, true) : undefined;
    return child === undefined ? undefined : `NOT ${child}`;
  }
  if (!bareField(node.field)) return undefined;
  if (node.op === 'IS_NULL') return `${node.field} IS EMPTY`;
  if (node.op === 'BETWEEN') {
    const [low, high] = Array.isArray(node.value) ? node.value : [];
    return `${node.field} BETWEEN ${valueText(low)} AND ${valueText(high)}`;
  }
  const word = WORDS[node.op];
  if (!word) return undefined;
  if (['IN', 'NOT_IN', 'CONTAINS_ANY', 'CONTAINS_ALL'].includes(node.op)) {
    const values = Array.isArray(node.value) ? node.value : [node.value];
    return `${node.field} ${word} (${values.map(valueText).join(', ')})`;
  }
  return `${node.field} ${word} ${valueText(node.value)}`;
}

/** Writes the filter and the order as a line `readLine` reads back to them; undefined where no line spells it. */
export function writeLine(filter: FilterNode | undefined, sort: readonly LineSort[] = []): string | undefined {
  const parts: string[] = [];
  if (filter) {
    const text = nodeText(filter, false);
    if (text === undefined) return undefined;
    parts.push(text);
  }
  if (sort.length > 0) {
    if (sort.some((term) => !bareField(term.field))) return undefined;
    parts.push(`ORDER BY ${sort.map((term) => [term.field, term.dir, term.nulls ? `NULLS ${term.nulls}` : undefined].filter(Boolean).join(' ')).join(', ')}`);
  }
  return parts.join(' ');
}
