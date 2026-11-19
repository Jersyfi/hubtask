// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The inventory's line narrowed on the device (work-surface §6.10, §6.11): the hub's objects are in
 * the device's copy (UC-AST-01 check 10), so *Bestand* narrows what it holds rather than asking the
 * server. The reading is the server's for the same line - `view.KindFilter` and `SearchCiNames` -,
 * so a line saved as a group or asked of the search answers the same objects: a key once, its values
 * or'ed with a comma, `-` turns the key round, `none` on owner, location and tag, `me` the reader,
 * the object type compared lowered with the types beneath it, and every free word in the name.
 */

import { parseQuery, type FilterNode, type QueryError } from '@hubtask/query';
import type { CiClass, CiGroupFilter, ConfigurationItem } from '@hubtask/sync-engine';

export const ASSET_KEYS = ['asset.class', 'asset.stage', 'asset.owner', 'asset.location', 'asset.tag'] as const;
export type AssetKey = (typeof ASSET_KEYS)[number];

/** The line *Bestand* opens with: everything not yet disposed of. */
export const DEFAULT_LINE = '-asset.stage: disposed';

/** One key as the search binds it: one of `values`, or none where `none`; `negated` turns it round. */
export interface AssetCondition {
  readonly key: AssetKey;
  readonly values: readonly string[];
  readonly none: boolean;
  readonly negated: boolean;
}

export interface InventoryFilter {
  readonly conditions: readonly AssetCondition[];
  /** The words a name must hold, lowered; a word with a leading minus is left out, as the server does. */
  readonly terms: readonly string[];
}

export type ReadLine = { readonly ok: true; readonly filter: InventoryFilter } | { readonly ok: false; readonly error: QueryError };

const isAssetKey = (field: unknown): field is AssetKey => (ASSET_KEYS as readonly string[]).includes(field as string);
const NULLABLE: readonly AssetKey[] = ['asset.owner', 'asset.location', 'asset.tag'];

/** The words a name must hold (`search.Terms`): lowered, without quotes, minus words and OR left out, each once. */
export function termsOf(words: string | undefined): string[] {
  const out: string[] = [];
  for (const field of (words ?? '').replaceAll('"', ' ').toLowerCase().split(/\s+/u)) {
    if (!field || field.startsWith('-') || field === 'or' || out.includes(field)) continue;
    out.push(field);
  }
  return out;
}

const refusal = (code: string, params: Record<string, string> = {}): ReadLine => ({ ok: false, error: { code, params } });

/** One leaf of a key: its values, or none. Undefined where the leaf is not one the inventory reads. */
function leafOf(node: FilterNode): { key: AssetKey; values: string[]; none: boolean; negated: boolean } | undefined {
  const field = node['field'];
  if (!isAssetKey(field)) return undefined;
  const value = node['value'];
  switch (node['op']) {
    case 'EQ': return { key: field, values: [String(value)], none: false, negated: false };
    case 'NEQ': return { key: field, values: [String(value)], none: false, negated: true };
    case 'IN': return { key: field, values: (value as unknown[]).map(String), none: false, negated: false };
    case 'NOT_IN': return { key: field, values: (value as unknown[]).map(String), none: false, negated: true };
    case 'IS_NULL': return NULLABLE.includes(field) ? { key: field, values: [], none: true, negated: false } : undefined;
    default: return undefined;
  }
}

/** One operand of the top AND: a leaf, an OR of one key's leaves (`a, none`), or either negated. */
function conditionOf(node: FilterNode): AssetCondition | QueryError {
  const op = node['op'];
  const nodes = (node['nodes'] as FilterNode[] | undefined) ?? [];
  if (op === 'NOT') {
    const inner = nodes.length === 1 ? conditionOf(nodes[0]!) : { code: 'query.kind_filter_grouped', params: {} };
    return 'code' in inner ? inner : { ...inner, negated: !inner.negated };
  }
  if (op === 'OR') {
    const leaves = nodes.map(leafOf);
    const first = leaves[0];
    if (!first || leaves.some((leaf) => !leaf || leaf.key !== first.key || leaf.negated)) return { code: 'query.kind_filter_grouped', params: {} };
    return {
      key: first.key,
      values: leaves.flatMap((leaf) => leaf!.values),
      none: leaves.some((leaf) => leaf!.none),
      negated: false,
    };
  }
  const leaf = leafOf(node);
  if (leaf) return leaf;
  const field = node['field'];
  return typeof field === 'string' && !field.startsWith('asset.')
    ? { code: 'query.kinds_mixed', params: { kinds: 'asset, entry' } }
    : { code: 'query.kind_filter_grouped', params: {} };
}

/** Reads a line as the inventory's search does; a refusal carries the server's code. */
export function readLine(line: string): ReadLine {
  if (!line.trim()) return { ok: true, filter: { conditions: [], terms: [] } };
  const parsed = parseQuery(line);
  if (!parsed.ok) return parsed;
  const { filter, words, sort, kinds } = parsed.query;
  if (sort || kinds) return refusal('query.kinds_mixed', { kinds: 'asset' });
  const units: FilterNode[] = !filter ? [] : filter['op'] === 'AND' ? ((filter['nodes'] as FilterNode[]) ?? []) : [filter];
  const conditions: AssetCondition[] = [];
  for (const unit of units) {
    const condition = conditionOf(unit);
    if ('code' in condition) return { ok: false, error: condition };
    if (conditions.some((held) => held.key === condition.key)) return refusal('query.key_twice', { field: condition.key });
    conditions.push(condition);
  }
  return { ok: true, filter: { conditions, terms: termsOf(words) } };
}

/** What an object is read against besides itself: the types' lineages, the reader, the register's numbers. */
export interface NarrowContext {
  readonly classes: readonly CiClass[];
  /** The reader's account, which `me` stands for. */
  readonly me?: string | undefined;
  /** An object's inventory number from the register, where it has one. */
  readonly tags?: ReadonlyMap<string, string>;
}

/** The object types an object is of, lowered: its own and every one above it. */
function lineageOf(ci: Pick<ConfigurationItem, 'class'>, classes: readonly CiClass[]): string[] {
  const found = classes.find((each) => each.key === ci.class);
  return (found?.lineage.length ? found.lineage : [ci.class]).map((each) => each.toLowerCase());
}

function holds(ci: ConfigurationItem, condition: AssetCondition, context: NarrowContext): boolean {
  const values = condition.values.map((value) => (value === '@me' ? (context.me ?? '') : value));
  switch (condition.key) {
    case 'asset.class': {
      const lineage = lineageOf(ci, context.classes);
      return values.some((value) => lineage.includes(value.toLowerCase()));
    }
    case 'asset.stage': return values.some((value) => value.toUpperCase() === ci.stage);
    case 'asset.owner': {
      const owners = [ci.owner_account_id, ci.owner_group_id].filter((each): each is string => Boolean(each));
      return values.some((value) => owners.includes(value)) || (condition.none && owners.length === 0);
    }
    case 'asset.location':
      return values.some((value) => value === ci.location_id) || (condition.none && !ci.location_id);
    case 'asset.tag': {
      const tag = context.tags?.get(ci.id)?.toLowerCase() || undefined;
      return values.some((value) => value.toLowerCase() === tag) || (condition.none && !tag);
    }
  }
}

/** Whether an object passes the filter: every condition, and every term in its name. */
export function passes(ci: ConfigurationItem, filter: InventoryFilter, context: NarrowContext): boolean {
  if (ci.merged_into) return false;
  const name = ci.name.toLowerCase();
  if (filter.terms.some((term) => !name.includes(term))) return false;
  return filter.conditions.every((condition) => holds(ci, condition, context) !== condition.negated);
}

/** The objects a filter keeps, in their order. */
export const narrowed = (cis: readonly ConfigurationItem[], filter: InventoryFilter, context: NarrowContext): ConfigurationItem[] =>
  cis.filter((ci) => passes(ci, filter, context));

/** The filter without one key: what the values of that key are counted within (Q-3). */
export const without = (filter: InventoryFilter, key: AssetKey): InventoryFilter => ({
  ...filter,
  conditions: filter.conditions.filter((each) => each.key !== key),
});

/** One value a key takes among the objects, with how many hold it. */
export interface ValueCount {
  readonly value: string;
  readonly count: number;
}

/**
 * The values a key takes among the objects the rest of the line keeps, the most held first: the
 * object types by their own key, the stages lowered as they are typed, owners and locations by
 * their identifier, `none` where an object has none.
 */
export function valueCounts(cis: readonly ConfigurationItem[], filter: InventoryFilter, key: AssetKey, context: NarrowContext): ValueCount[] {
  const counts = new Map<string, number>();
  const add = (value: string) => counts.set(value, (counts.get(value) ?? 0) + 1);
  for (const ci of narrowed(cis, without(filter, key), context)) {
    switch (key) {
      case 'asset.class': add(ci.class); break;
      case 'asset.stage': add(ci.stage.toLowerCase()); break;
      case 'asset.owner': add(ci.owner_account_id ?? ci.owner_group_id ?? 'none'); break;
      case 'asset.location': add(ci.location_id ?? 'none'); break;
      case 'asset.tag': add(context.tags?.get(ci.id)?.toLowerCase() || 'none'); break;
    }
  }
  return [...counts.entries()].map(([value, count]) => ({ value, count })).sort((a, b) => b.count - a.count || a.value.localeCompare(b.value));
}

/** One row of the object-type tree: a type, how deep it stands, and the objects of it and beneath it. */
export interface TreeRow {
  readonly key: string;
  readonly depth: number;
  readonly count: number;
  readonly hasChildren: boolean;
}

/**
 * The object-type tree with counts (scene invlist 3): every type with the objects of it and of the
 * types beneath it, parents before their children in the types' order. A type with no object is
 * left out unless one beneath it has some; the root is the tree's *All*, not a row.
 */
export function classTree(cis: readonly ConfigurationItem[], classes: readonly CiClass[], root = 'ci'): TreeRow[] {
  const counts = new Map<string, number>();
  for (const ci of cis) {
    const lineage = classes.find((each) => each.key === ci.class)?.lineage ?? [ci.class];
    for (const key of new Set(lineage)) counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  const known = new Set(classes.map((each) => each.key));
  const childrenOf = (parent: string) => classes.filter((each) => each.key !== root && (each.parent === parent || (parent === root && !known.has(each.parent))));
  const rows: TreeRow[] = [];
  const walk = (parent: string, depth: number) => {
    for (const each of childrenOf(parent)) {
      const count = counts.get(each.key) ?? 0;
      if (count === 0) continue;
      const hasChildren = childrenOf(each.key).some((child) => (counts.get(child.key) ?? 0) > 0);
      rows.push({ key: each.key, depth, count, hasChildren });
      walk(each.key, depth + 1);
    }
  };
  walk(root, 0);
  return rows;
}

/** The value of a key the line holds alone, where it holds exactly one, not negated. */
export function soleValue(line: string, key: AssetKey): string | undefined {
  const read = readLine(line);
  if (!read.ok) return undefined;
  const condition = read.filter.conditions.find((each) => each.key === key);
  return condition && !condition.negated && !condition.none && condition.values.length === 1 ? condition.values[0] : undefined;
}

/** The line with one key set to a single value, or taken out (`undefined`); the rest kept. */
export function withKey(line: string, key: AssetKey, value: string | undefined): string {
  const read = readLine(line);
  const rest = read.ok ? read.filter.conditions.filter((each) => each.key !== key).map(conditionText) : [];
  return [value ? `${key}: ${quoted(value)}` : '', ...rest, read.ok ? (wordsOf(line) ?? '') : ''].filter(Boolean).join(' ');
}

const wordsOf = (line: string): string | undefined => {
  const parsed = parseQuery(line);
  return parsed.ok ? parsed.query.words : undefined;
};

/** One condition written as it is typed: stages lowered, the reader as `me`. */
function conditionText(condition: AssetCondition): string {
  const values = condition.values.map((value) => (value === '@me' ? 'me' : condition.key === 'asset.stage' ? value.toLowerCase() : quoted(value)));
  return `${condition.negated ? '-' : ''}${condition.key}: ${[...values, ...(condition.none ? ['none'] : [])].join(', ')}`;
}

const quoted = (value: string) => (/[\s(),"]/u.test(value) ? `"${value.replaceAll('"', '\\"')}"` : value);

/**
 * A saved group's filter as the line (UC-AST-11): its object types, stages, owners, locations and
 * numbers as `asset.` keys, the text its name holds as words. Attribute values have no key; they
 * are returned beside the line for the screen to keep narrowing by.
 */
export function lineOfGroup(filter: CiGroupFilter): { line: string; attributes: Readonly<Record<string, string>> } {
  const parts: string[] = [];
  const key = (name: AssetKey, values: readonly string[] | undefined, none?: boolean) => {
    const all = [...(values ?? []).map(quoted), ...(none ? ['none'] : [])];
    if (all.length > 0) parts.push(`${name}: ${all.join(', ')}`);
  };
  key('asset.class', filter.classes);
  key('asset.stage', filter.stages?.map((each) => each.toLowerCase()));
  key('asset.owner', filter.owners, filter.no_owner);
  key('asset.location', filter.locations, filter.no_location);
  key('asset.tag', filter.tags, filter.no_tag);
  if (filter.name_has) parts.push(quoted(filter.name_has));
  return { line: parts.join(' '), attributes: filter.attributes ?? {} };
}

/** Whether an object holds a group's attribute values, compared as the server does: lowered text. */
export function holdsAttributes(ci: ConfigurationItem, attributes: Readonly<Record<string, string>>): boolean {
  return Object.entries(attributes).every(([key, value]) => {
    const own = ci.attributes[key];
    return own !== undefined && own !== null && String(own).toLowerCase() === value.toLowerCase();
  });
}

/**
 * A line as a group's filter (*Als Gruppe speichern*), or why it cannot become one - a message
 * code: a group holds what is, never what is not (a negated key), and one text its name holds.
 */
export function groupOfLine(line: string, me: string | undefined): { ok: true; filter: CiGroupFilter } | { ok: false; code: string } {
  const read = readLine(line);
  if (!read.ok) return { ok: false, code: read.error.code };
  if (read.filter.conditions.some((each) => each.negated)) return { ok: false, code: 'app.inventory.group_negated' };
  const words = (wordsOf(line) ?? '').trim();
  const inQuotes = /^"([^"]*)"$/u.exec(words)?.[1];
  if (inQuotes === undefined && /\s/u.test(words)) return { ok: false, code: 'app.inventory.group_words' };
  if (inQuotes === undefined && words.startsWith('-')) return { ok: false, code: 'app.inventory.group_negated' };
  const phrase = (inQuotes ?? words).trim();
  const filter: CiGroupFilter = {};
  for (const condition of read.filter.conditions) {
    const values = condition.values.map((value) => (value === '@me' ? (me ?? value) : value));
    switch (condition.key) {
      case 'asset.class': filter.classes = values; break;
      case 'asset.stage': filter.stages = values.map((each) => each.toUpperCase()) as CiGroupFilter['stages']; break;
      case 'asset.owner':
        if (values.length) filter.owners = values;
        if (condition.none) filter.no_owner = true;
        break;
      case 'asset.location':
        if (values.length) filter.locations = values;
        if (condition.none) filter.no_location = true;
        break;
      case 'asset.tag':
        if (values.length) filter.tags = values.map((each) => each.toLowerCase());
        if (condition.none) filter.no_tag = true;
        break;
    }
  }
  if (phrase) filter.name_has = phrase;
  if (Object.keys(filter).length === 0) return { ok: false, code: 'cmdb.group_filter_empty' };
  return { ok: true, filter };
}

/** The highest of an object's own protection need over confidentiality, integrity and availability. */
export function highestProtection(protection: ConfigurationItem['protection'] | undefined): 'NORMAL' | 'HIGH' | 'VERY_HIGH' | undefined {
  const order = ['NORMAL', 'HIGH', 'VERY_HIGH'] as const;
  let best = -1;
  for (const level of [protection?.c, protection?.i, protection?.a]) {
    const at = level ? order.indexOf(level) : -1;
    if (at > best) best = at;
  }
  return best >= 0 ? order[best] : undefined;
}
