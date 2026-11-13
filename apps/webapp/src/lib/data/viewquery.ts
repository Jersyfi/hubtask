// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the one panel *Ansicht* asks (work-surface concept §6.2, E-21; §6.11, Q-1…Q-5): the Hubtask
 * query as typed, turned into the tree a list, a board and a saved view send. Pure, so each step
 * is tested.
 *
 * The line is read by `@hubtask/query`, the server's twin (ADR-0102); the server reads the tree
 * again and its answer counts. Names a person types - a person, a label, a collection - are
 * resolved to identifiers here, before anything is sent, because the server compares identifiers
 * (ADR-0102, "names are the client's"). A `word:` that names no field (`Re: Besprechung`) is
 * searched as words rather than refused, and the chip says so (Q-2, never "too complex").
 */

import { formatQuery, parseQuery, segments, type Query, type QueryError } from '@hubtask/query';
import type { FilterNode } from '@hubtask/sync-engine';

import type { ItemsQuery } from './items.svelte.ts';

/** Looks a typed name up, and an identifier back to its name for the line. */
export interface Names {
  person(name: string): string | undefined;
  label(name: string): string | undefined;
  collection(name: string): string | undefined;
  /** A hub's release and a collection's sprint, where the screen knows them (the plan). */
  release?(name: string): string | undefined;
  sprint?(name: string): string | undefined;
  nameOf?(field: string, id: string): string | undefined;
}

const NAMED: Readonly<Record<string, 'person' | 'label' | 'collection' | 'release' | 'sprint'>> = {
  assignee_id: 'person',
  members: 'person',
  created_by: 'person',
  labels: 'label',
  collection_id: 'collection',
  release_id: 'release',
  sprint_id: 'sprint',
  // A hub is a place named as a collection is: the names' collection lookup answers both.
  hub_id: 'collection',
};
const IDENTIFIER = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/iu;
const isWord = (value: unknown): value is string => typeof value === 'string' && !value.startsWith('@') && !IDENTIFIER.test(value);

/** The line with every `word:` that names no field quoted, so it is searched as words, and those pieces. */
export function asWords(text: string): { line: string; pieces: readonly string[] } {
  const parsed = parseQuery(text);
  if (parsed.ok || parsed.error.code !== 'query.field_unknown') return { line: text, pieces: [] };
  const { pieces, rest } = segments(`${text} `);
  const quoted: string[] = [];
  const line = pieces
    .map((piece) => {
      const own = parseQuery(piece);
      if (own.ok || own.error.code !== 'query.field_unknown') return piece;
      quoted.push(piece);
      return `"${piece.replaceAll('"', '')}"`;
    })
    .concat(rest.trim() === '' ? [] : [rest.trim()])
    .join(' ');
  return { line, pieces: quoted };
}

/** Resolves the names in a tree to identifiers; what names nobody stays as typed (the server refuses it). */
export function resolveNames(node: FilterNode, names: Names): { node: FilterNode; unknown: readonly string[] } {
  const unknown: string[] = [];
  const visit = (each: FilterNode): FilterNode => {
    if (Array.isArray(each['nodes'])) return { ...each, nodes: (each['nodes'] as FilterNode[]).map(visit) };
    const kind = NAMED[String(each['field'])];
    if (!kind) return each;
    const one = (value: unknown) => {
      if (!isWord(value)) return value;
      const id = names[kind]?.(value);
      if (!id) unknown.push(value);
      return id ?? value;
    };
    const value = each['value'];
    return { ...each, value: Array.isArray(value) ? value.map(one) : one(value) };
  };
  return { node: visit(node), unknown };
}

/** Writes identifiers back as names, for the line a saved view opens with. */
function namedBack(node: FilterNode, names: Names): FilterNode {
  if (Array.isArray(node['nodes'])) return { ...node, nodes: (node['nodes'] as FilterNode[]).map((each) => namedBack(each, names)) };
  const field = String(node['field']);
  if (!NAMED[field] || !names.nameOf) return node;
  const one = (value: unknown) => (typeof value === 'string' && IDENTIFIER.test(value) ? (names.nameOf?.(field, value) ?? value) : value);
  const value = node['value'];
  return { ...node, value: Array.isArray(value) ? value.map(one) : one(value) };
}

export type Order = { readonly field: string; readonly dir: 'ASC' | 'DESC' } | undefined;

export type Read =
  | { readonly ok: true; readonly query: ItemsQuery; readonly words: readonly string[]; readonly unknown: readonly string[]; readonly line: string }
  | { readonly ok: false; readonly error: QueryError };

/**
 * What the list sends for a line: its tree with the free words as one `text MATCHES` node beside it
 * - the server's `EntryFilter` -, names resolved, and the panel's order before the line's own.
 */
export function readQuery(text: string, names: Names, order?: Order): Read {
  const { line, pieces } = asWords(text);
  const parsed = parseQuery(line);
  if (!parsed.ok) return parsed;
  const query: Query = parsed.query;
  const nodes: FilterNode[] = [];
  if (query.filter) nodes.push(query.filter as FilterNode);
  if (query.words) nodes.push({ field: 'text', op: 'MATCHES', value: query.words });
  const joined: FilterNode | undefined = nodes.length > 1 ? { op: 'AND', nodes } : nodes[0];
  const resolved = joined ? resolveNames(joined, names) : undefined;
  const sort = order ? [order] : query.sort?.map((each) => ({ field: each.field, dir: each.dir ?? ('ASC' as const) }));
  return {
    ok: true,
    query: { ...(resolved ? { filter: resolved.node } : {}), ...(sort?.length ? { sort } : {}) },
    words: pieces,
    unknown: resolved?.unknown ?? [],
    line,
  };
}

/** A chip's words: its piece with every identifier written as its name (Q-2), else the piece as typed. */
export function spokenPiece(piece: string, names: Names): string {
  const parsed = parseQuery(piece);
  if (!parsed.ok || !parsed.query.filter || !IDENTIFIER_IN.test(piece)) return piece;
  return formatQuery({ filter: namedBack(parsed.query.filter as FilterNode, names) }) ?? piece;
}
const IDENTIFIER_IN = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/iu;

/** The line a stored tree is spelled as, names for identifiers; undefined where no text spells it. */
export function lineOf(query: ItemsQuery | undefined, names: Names): string | undefined {
  if (!query?.filter) return '';
  return formatQuery({ filter: namedBack(query.filter, names) });
}

/** One panel's state: what a view saves and what the dot on the tab compares. */
export interface ViewSetup {
  readonly layout: string;
  readonly line: string;
  readonly order: Order;
  readonly grouping: string;
  readonly done: string;
  readonly fields: readonly string[];
  readonly columnsBy: string;
  readonly lanesBy: string;
  readonly dateField: string;
  readonly limits: boolean;
}

/** How many parts differ from the view the tab opened with: *Ansicht · 3 geändert*. */
export function changedParts(now: ViewSetup, base: ViewSetup): number {
  const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null);
  const parts: [unknown, unknown][] = [
    [now.layout.startsWith('LIST_') ? 'LIST' : now.layout, base.layout.startsWith('LIST_') ? 'LIST' : base.layout],
    [now.line.trim(), base.line.trim()],
    [now.order, base.order],
    [now.grouping, base.grouping],
    [now.done, base.done],
    [now.fields, base.fields],
    [now.columnsBy, base.columnsBy],
    [now.lanesBy, base.lanesBy],
    [now.dateField, base.dateField],
    [now.limits, base.limits],
  ];
  return parts.filter(([a, b]) => !same(a, b)).length;
}

/** The grouping a view stores (`SavedView.grouping`): the columns, the lanes, the calendar's date, the list's own. */
export function groupingOf(setup: ViewSetup): Record<string, unknown> {
  return {
    ...(setup.columnsBy ? { field: setup.columnsBy, limit_per_group: 50 } : {}),
    ...(setup.lanesBy ? { lanes_by: setup.lanesBy } : {}),
    ...(setup.dateField ? { date_field: setup.dateField } : {}),
    list: setup.grouping,
    done: setup.done,
    ...(setup.limits ? {} : { limits: false }),
  };
}

/** The value a board's lane is drawn for: the entry's own value of the lanes' field. */
export function laneOf(entry: object, field: string): string {
  const record = entry as Record<string, unknown>;
  const value = field.startsWith('custom_fields.')
    ? (record['custom_fields'] as Record<string, unknown> | undefined)?.[field.slice('custom_fields.'.length)]
    : record[field];
  if (Array.isArray(value)) return value.length > 0 ? String(value[0]) : '';
  return value === undefined || value === null ? '' : String(value);
}

/** The lanes in the order they first appear, the empty lane last. */
export function lanesOf(entries: readonly object[], field: string): readonly string[] {
  const seen: string[] = [];
  for (const entry of entries) {
    const lane = laneOf(entry, field);
    if (!seen.includes(lane)) seen.push(lane);
  }
  return [...seen.filter((lane) => lane !== ''), ...(seen.includes('') ? [''] : [])];
}

/** The days of a calendar month as weeks, Monday or Sunday first as the account says. */
export function monthGrid(year: number, month: number, weekStartsSunday = false): readonly (readonly string[])[] {
  const first = new Date(Date.UTC(year, month, 1));
  const shift = (first.getUTCDay() + (weekStartsSunday ? 0 : 6)) % 7;
  const start = Date.UTC(year, month, 1 - shift);
  const weeks: string[][] = [];
  for (let week = 0; week < 6; week += 1) {
    const days: string[] = [];
    for (let day = 0; day < 7; day += 1) days.push(new Date(start + (week * 7 + day) * 86_400_000).toISOString().slice(0, 10));
    weeks.push(days);
    if (new Date(start + (week * 7 + 7) * 86_400_000).getUTCMonth() !== month) break;
  }
  return weeks;
}
