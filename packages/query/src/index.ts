// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Hubtask query in the browser (ADR-0102): the twin of the server's parser, and the two things
// the design system's QueryLine takes - a language that cuts a line into chips and writes a chip
// back, and a catalogue of keys with their values. Structurally the shapes of
// `@hubtask/design-system`'s query.ts; this package imports nothing, so the web app joins the two.
// Fetching is the sync engine's: the request bodies below are what a caller sends.

import { formatQuery, lexQuery, parseQuery, type Query, type QueryError } from './parse.ts';

export { formatQuery, parseQuery, OBJECT_KINDS, READER_KEYS, resolveKey } from './parse.ts';
export type { FilterNode, Parsed, Query, QueryError } from './parse.ts';

/** A key of `/meta/capabilities`' `query_keys`, as the contract names it. */
export interface QueryKey {
  readonly key: string;
  readonly field?: string;
  readonly value: string;
  readonly product?: string;
  readonly objects: readonly string[];
  readonly examples: readonly string[];
  readonly values: readonly string[];
}

/** What the web app hands in: names in the reader's language (rule 8) and a refusal worded. */
export interface Wording {
  key(key: string): string;
  kind?(key: QueryKey): string;
  value?(key: string, value: string): string;
  problem(error: QueryError): string;
}

type Chip = {
  readonly kind: 'chip';
  readonly id: string;
  readonly field: string;
  readonly fieldLabel: string;
  readonly operatorLabel?: string;
  readonly values: readonly { readonly value: string; readonly label: string }[];
  readonly isNegated?: boolean;
  readonly problem?: { readonly message: string; readonly fix?: string; readonly fixLabel?: string };
};
type TextChip = { readonly kind: 'text'; readonly id: string; readonly text: string; readonly problem?: { readonly message: string } };
export type Token = Chip | TextChip;

const TERM = /^(-?)([A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*):\s*(.*)$/su;

/**
 * Cuts a line into its top-level pieces - a `key: values` term, a bracketed group with what
 * negates it, a word or phrase - each with its text as typed. The unfinished end is what follows
 * the last space.
 */
export function segments(text: string): { pieces: string[]; rest: string } {
  const finished = /\s$/u.test(text) || text === '' ? text : text.slice(0, Math.max(text.search(/\S+$/u), 0));
  let tokens;
  try {
    tokens = lexQuery(finished);
  } catch {
    return { pieces: [], rest: text };
  }
  const pieces: string[] = [];
  let i = 0;
  while (i < tokens.length) {
    const start = tokens[i];
    if (!start) break;
    let j = i;
    let depth = 0;
    const startsGroup = start.text === '(' || start.text === 'NOT' || (start.text === '-' && tokens[i + 1]?.text === '(');
    for (; j < tokens.length; j += 1) {
      const t = tokens[j];
      if (!t) break;
      if (t.kind === 'symbol' && t.text === '(') depth += 1;
      if (t.kind === 'symbol' && t.text === ')') depth -= 1;
      const next = tokens[j + 1];
      const continues = depth > 0 || (startsGroup && j === i) || next?.text === ',' || t.text === ',' ||
        (j === i && /^-?[A-Za-z][\w.]*:$/u.test(t.text)) || next?.text === '+' || t.text === '+';
      if (!continues) break;
    }
    const last = tokens[Math.min(j, tokens.length - 1)];
    const end = last ? last.at + (last.kind === 'text' ? last.text.length + 2 : last.text.length) : finished.length;
    pieces.push(finished.slice(start.at, end).trim());
    i = j + 1;
  }
  return { pieces, rest: text.slice(finished.length) };
}

/** The language QueryLine takes: chips for terms, text chips for groups and words (Q-2). */
export function createLanguage(wording: Wording) {
  let counter = 0;
  return {
    parse(text: string): { tokens: Token[]; rest: string } {
      const { pieces, rest } = segments(text);
      const tokens = pieces.map((piece): Token => {
        counter += 1;
        const id = `q${counter}`;
        const parsed = parseQuery(piece);
        const problem = parsed.ok ? undefined : problemOf(parsed.error, wording);
        const term = TERM.exec(piece);
        if (!term || piece.includes('(')) return { kind: 'text', id, text: piece, problem };
        const negated = term[1] === '-';
        const key = term[2] ?? '';
        const raw = (term[3] ?? '').trim();
        const sign = /^(<=|>=|<|>|=|~)/u.exec(raw)?.[1];
        const values = (sign ? raw.slice(sign.length) : raw).split(/\s*[,+]\s*/u).filter(Boolean)
          .map((value) => ({ value, label: wording.value?.(key, value.replace(/^"|"$/gu, '')) ?? value.replace(/^"|"$/gu, '') }));
        return { kind: 'chip', id, field: key, fieldLabel: wording.key(key), operatorLabel: sign, values, isNegated: negated, problem };
      });
      return { tokens, rest };
    },
    format(token: Token): string {
      if (token.kind === 'text') return token.text;
      const values = token.values.map((v) => (/[\s(),"]/u.test(v.value) && !v.value.startsWith('"') ? `"${v.value.replaceAll('"', '\\"')}"` : v.value));
      return `${token.isNegated ? '-' : ''}${token.field}: ${token.operatorLabel ?? ''}${values.join(', ')}`;
    },
  };
}

/**
 * The chips with the words a fresh cut of the same line gives them - ids kept, so a chip in use
 * stays the same chip; undefined where the line differs or nothing reads differently. The words of
 * a language can arrive after the line was cut (a catalogue loaded later).
 */
export function relabelled(current: readonly Token[], fresh: readonly Token[]): Token[] | undefined {
  if (current.length !== fresh.length) return undefined;
  let changed = false;
  const next: Token[] = [];
  for (const [i, token] of current.entries()) {
    const other = fresh[i]!;
    if (token.kind === 'text' && other.kind === 'text') {
      if (token.text !== other.text) return undefined;
      if (JSON.stringify(token.problem) !== JSON.stringify(other.problem)) changed = true;
      next.push({ ...other, id: token.id });
      continue;
    }
    if (token.kind !== 'chip' || other.kind !== 'chip') return undefined;
    const same = token.field === other.field && token.isNegated === other.isNegated && token.operatorLabel === other.operatorLabel &&
      token.values.map((v) => v.value).join('\u0000') === other.values.map((v) => v.value).join('\u0000');
    if (!same) return undefined;
    if (token.fieldLabel !== other.fieldLabel || JSON.stringify([token.values, token.problem]) !== JSON.stringify([other.values, other.problem])) changed = true;
    next.push({ ...other, id: token.id });
  }
  return changed ? next : undefined;
}

function problemOf(error: QueryError, wording: Wording) {
  const fix = error.params.suggestion;
  return { message: wording.problem(error), fix, fixLabel: fix };
}

/** One value the data holds for a field, with its count, from a grouped summary. */
export interface CountedValue {
  readonly value: string;
  readonly label: string;
  readonly countLabel?: string;
}

/**
 * The catalogue QueryLine takes: the keys the server serves, named by the reader's language; the
 * values a key's closed list holds, then those the data holds (Q-3).
 */
export function createCatalogue(keys: readonly QueryKey[], wording: Wording, counted?: (key: string, prefix: string) => readonly CountedValue[]) {
  return {
    fields: keys.map((key) => ({ key: key.key, label: wording.key(key.key), detail: wording.kind?.(key), example: key.examples[0] })),
    values(key: string, prefix: string): readonly CountedValue[] {
      const own = keys.find((candidate) => candidate.key === key)?.values ?? [];
      const listed = own.map((value) => ({ value, label: wording.value?.(key, value) ?? value }));
      return [...listed, ...(counted?.(key, prefix) ?? [])];
    },
  };
}

/** The words of the language offered after fields and values (Q-3): they stay English. */
export const KEYWORDS = ['OR', 'AND', 'NOT', 'sort:', 'me', 'none', 'today', 'overdue', 'this-week', '-7d', '+2w'] as const;

/** Where a count or a value list is asked: one hub, collection or entry, or the whole workspace. */
export type Scope = { readonly container_id: string } | { readonly item_id: string } | { readonly workspace: true };

/**
 * The body of `POST /items:aggregate` that counts a line while it is typed (Q-4), computed by the
 * server for the reader alone (Q-5). The line travels in the body, never in an address
 * (UC-WRK-18/4). Undefined where the line cannot be read yet, or orders - a count has no order.
 */
export function countRequest(line: string, scope: Scope) {
  const parsed = parseQuery(line);
  if (!parsed.ok || parsed.query.sort) return undefined;
  return { scope, q: line, aggregate: { function: 'COUNT' } };
}

/**
 * The body that lists a key's values with their counts, within the rest of the line: the same
 * summary grouped by the key's field. Undefined for a key without one.
 */
export function valuesRequest(key: QueryKey, line: string, scope: Scope) {
  if (!key.field || key.value === 'custom' || key.value === 'time' || key.value === 'words' || key.value === 'text') return undefined;
  const parsed = parseQuery(line);
  if (!parsed.ok) return undefined;
  const rest: Query = { ...parsed.query };
  delete rest.sort;
  const q = formatQuery(rest);
  return { scope, ...(q ? { q } : {}), aggregate: { function: 'COUNT', group_by: key.field } };
}
