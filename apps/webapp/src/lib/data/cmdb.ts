// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's configuration items as the *Assets* screen needs them (M6, UC-AST-01 … UC-AST-06): the
 * paths it reads, the inventory grouped by class, a typed attribute read from what a person typed,
 * the relations both ways in their own words, the impact by depth, the protection need beside the
 * one inherited, duplicates side by side, and a file's format and columns. Pure, so the shapes
 * have tests without a browser.
 */

import type {
  CiAttribute, CiClass, CiDecision, CiFileImport, CiOwnProtection, CiProtection, CiReached, CiRelation, CiRelationType,
  ConfigurationItem,
} from '@hubtask/sync-engine';

export const ROOT_CLASS = 'ci';

export const classesPath = '/cmdb/classes';
export const relationTypesPath = '/cmdb/relation-types';
export const sourcesPath = '/cmdb/sources';

export function cisPath(hubId: string, options: { class?: string; after?: string; includeMerged?: boolean } = {}): string {
  const query = new URLSearchParams();
  if (options.class) query.set('class', options.class);
  if (options.after) query.set('after', options.after);
  if (options.includeMerged) query.set('include_merged', 'true');
  const tail = query.toString();
  return `/hubs/${hubId}/cis${tail ? `?${tail}` : ''}`;
}
export const ciPath = (ciId: string) => `/cis/${ciId}`;
export const impactPath = (ciId: string, direction: 'DOWNSTREAM' | 'UPSTREAM') => `/cis/${ciId}/impact?direction=${direction}`;
export const runsPath = (hubId: string) => `/hubs/${hubId}/ci-runs`;
export const runPath = (runId: string) => `/ci-runs/${runId}`;
export const holdsPath = (hubId: string) => `/hubs/${hubId}/ci-holds`;
export const duplicatesPath = (hubId: string) => `/hubs/${hubId}/ci-duplicates`;
export const healthPath = (hubId: string) => `/hubs/${hubId}/cmdb-health`;

/** The sections of a hub's *Assets* screen, in their order; the first is the address's default. */
export const ASSET_SECTIONS = ['inventory', 'runs', 'duplicates', 'import', 'health'] as const;
export type AssetSection = (typeof ASSET_SECTIONS)[number];
export const sectionOf = (name: string | undefined): AssetSection =>
  (ASSET_SECTIONS as readonly string[]).includes(name ?? '') ? (name as AssetSection) : 'inventory';

/** Looks a word up and says whether the catalogue has it; the screen passes its `t`. */
export type Words = (code: string) => string | undefined;

/**
 * A class's word: an own class's name, a shipped class's word from the catalogue, else its key -
 * a shipped class carries no name (the contract's `CiClass`).
 */
export function classWord(key: string, classes: readonly CiClass[], words: Words): string {
  const own = classes.find((each) => each.key === key)?.name;
  return own || words(`app.assets.class.${key}`) || key;
}

/** A relation type's word read from one end: from its source the name, from its target the reverse. */
export function relationWord(type: CiRelationType | undefined, key: string, outgoing: boolean, words: Words): string {
  const own = outgoing ? type?.name : type?.reverse_name;
  return own || words(`app.assets.relation.${key}.${outgoing ? 'out' : 'in'}`) || key;
}

/** The classes a CI may be made of: not abstract, not archived, not the root. */
export const instantiable = (classes: readonly CiClass[]): CiClass[] =>
  classes.filter((each) => !each.abstract && !each.archived && each.key !== ROOT_CLASS);

/** The classes beneath a CI's own, which it may be refined to (`cmdb.class_not_descendant`). */
export const refinements = (classes: readonly CiClass[], key: string): CiClass[] =>
  instantiable(classes).filter((each) => each.key !== key && each.lineage.includes(key));

export interface ClassGroup {
  readonly class: string;
  readonly items: readonly ConfigurationItem[];
}

/** The inventory by class, in the order the classes are listed; a class unknown to the list last. */
export function inventoryByClass(cis: readonly ConfigurationItem[], classes: readonly CiClass[]): ClassGroup[] {
  const order = new Map(classes.map((each, index) => [each.key, index]));
  const groups = new Map<string, ConfigurationItem[]>();
  for (const ci of cis) groups.set(ci.class, [...(groups.get(ci.class) ?? []), ci]);
  return [...groups.entries()]
    .sort(([a], [b]) => (order.get(a) ?? Number.MAX_SAFE_INTEGER) - (order.get(b) ?? Number.MAX_SAFE_INTEGER) || a.localeCompare(b))
    .map(([key, items]) => ({ class: key, items }));
}

/** What a typed field's text becomes, or why it does not: an empty field clears the value. */
export type Parsed = { readonly ok: true; readonly value: unknown } | { readonly ok: false };

export function parseAttribute(attribute: CiAttribute, text: string): Parsed {
  const value = text.trim();
  if (value === '') return { ok: true, value: null };
  switch (attribute.kind) {
    case 'NUMBER': {
      const number = Number(value.replace(',', '.'));
      return Number.isFinite(number) ? { ok: true, value: number } : { ok: false };
    }
    case 'BOOLEAN':
      return value === 'true' || value === 'false' ? { ok: true, value: value === 'true' } : { ok: false };
    case 'DATE':
      return /^\d{4}-\d{2}-\d{2}$/u.test(value) ? { ok: true, value } : { ok: false };
    case 'CHOICE':
      return (attribute.options ?? []).includes(value) ? { ok: true, value } : { ok: false };
    default:
      return { ok: true, value };
  }
}

/** A value as the field shows it. */
export function attributeText(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/**
 * The attributes a form changed, with `null` for one cleared, or the keys that do not parse. Only
 * what changed is sent: the reconciliation keeps a value a higher-ranked source wrote, and a
 * field left alone must not claim it for `manual`.
 */
export function changedAttributes(
  attributes: readonly CiAttribute[],
  before: Record<string, unknown>,
  drafts: Record<string, string>,
): { readonly changes: Record<string, unknown>; readonly invalid: readonly string[] } {
  const changes: Record<string, unknown> = {};
  const invalid: string[] = [];
  for (const attribute of attributes) {
    const text = drafts[attribute.key];
    if (text === undefined || text === attributeText(before[attribute.key])) continue;
    const parsed = parseAttribute(attribute, text);
    if (!parsed.ok) invalid.push(attribute.key);
    else if (!(parsed.value === null && before[attribute.key] === undefined)) changes[attribute.key] = parsed.value;
  }
  return { changes, invalid };
}

/** The required attributes the form leaves empty, so the screen says so before asking. */
export const missingRequired = (attributes: readonly CiAttribute[], values: Record<string, string>): string[] =>
  attributes.filter((each) => each.need === 'REQUIRED' && !(values[each.key] ?? '').trim()).map((each) => each.key);

export interface RelationRow {
  readonly id: string;
  readonly type: string;
  readonly outgoing: boolean;
  /** The CI at the other end. */
  readonly other: string;
  readonly source: string;
  readonly lastSeen: string;
}

/** A CI's relations both ways, outgoing first, each named from this CI's end (UC-AST-01 check 5). */
export function relationRows(ciId: string, relations: readonly CiRelation[]): RelationRow[] {
  const row = (relation: CiRelation, outgoing: boolean): RelationRow => ({
    id: relation.id,
    type: relation.type,
    outgoing,
    other: outgoing ? relation.target_ci_id : relation.source_ci_id,
    source: relation.data_source,
    lastSeen: relation.last_seen,
  });
  return [
    ...relations.filter((each) => each.source_ci_id === ciId).map((each) => row(each, true)),
    ...relations.filter((each) => each.source_ci_id !== ciId && each.target_ci_id === ciId).map((each) => row(each, false)),
  ];
}

/** Whether a type may join a CI of one class to one of another; no pair joins any (`Allows`). */
export function typeAllows(type: CiRelationType, from: CiClass | undefined, to: CiClass | undefined): boolean {
  if (type.archived) return false;
  if (type.pairs.length === 0) return true;
  if (!from || !to) return false;
  return type.pairs.some((pair) => from.lineage.includes(pair.from) && to.lineage.includes(pair.to));
}

/** The impact as a list grouped by depth, the nearest first (UC-AST-05 check 5). */
export function impactByDepth(reached: readonly CiReached[]): { readonly depth: number; readonly items: readonly CiReached[] }[] {
  const depths = [...new Set(reached.map((each) => each.depth))].sort((a, b) => a - b);
  return depths.map((depth) => ({ depth, items: reached.filter((each) => each.depth === depth) }));
}

export const PROTECTION_LEVELS = ['NORMAL', 'HIGH', 'VERY_HIGH'] as const;
export type Level = (typeof PROTECTION_LEVELS)[number];
export const DIMENSIONS = ['c', 'i', 'a'] as const;
export type Dimension = (typeof DIMENSIONS)[number];

export interface ProtectionRow {
  readonly dimension: Dimension;
  readonly own: Level | null;
  readonly effective: Level | null;
  readonly fixed: boolean;
  /** The need was raised by what suffers when this CI fails. */
  readonly raised: boolean;
}

const rank = (level: Level | null | undefined) => (level ? PROTECTION_LEVELS.indexOf(level) : -1);

/** The own need beside the effective one, per dimension (UC-AST-05 check 5). */
export function protectionRows(own: CiOwnProtection | undefined, effective: CiProtection | undefined): ProtectionRow[] {
  return DIMENSIONS.map((dimension) => {
    const mine = (own?.[dimension] ?? null) as Level | null;
    const inherited = (effective?.[dimension] ?? mine) as Level | null;
    return {
      dimension,
      own: mine,
      effective: inherited,
      fixed: own?.fixed?.includes(dimension) ?? false,
      raised: rank(inherited) > rank(mine),
    };
  });
}

/** Two or more CIs side by side: every attribute any of them holds, and whether they differ. */
export function sideBySide(cis: readonly ConfigurationItem[]): { readonly key: string; readonly values: readonly string[]; readonly differ: boolean }[] {
  const keys = [...new Set(cis.flatMap((ci) => Object.keys(ci.attributes)))].sort();
  return [
    { key: 'name', values: cis.map((ci) => ci.name) },
    { key: 'class', values: cis.map((ci) => ci.class) },
    ...keys.map((key) => ({ key, values: cis.map((ci) => attributeText(ci.attributes[key])) })),
  ].map((row) => ({ ...row, differ: new Set(row.values).size > 1 }));
}

/** The tone a decision is drawn in; a refusal and a hold ask for a person. */
export function outcomeTone(outcome: CiDecision['outcome']): 'success' | 'info' | 'warning' | 'danger' | 'neutral' {
  switch (outcome) {
    case 'CREATE':
      return 'success';
    case 'UPDATE':
      return 'info';
    case 'HOLD':
    case 'DUPLICATE':
      return 'warning';
    case 'REFUSED':
      return 'danger';
    default:
      return 'neutral';
  }
}

/** A share from 0 to 1 as whole percent. */
export const percent = (share: number) => Math.round(Math.min(1, Math.max(0, share)) * 100);

/** The format a file is read in, by its name and what it holds. */
export function fileFormat(name: string, content: string): CiFileImport['format'] {
  if (/\.csv$/iu.test(name)) return 'CSV';
  const head = content.slice(0, 4096);
  if (/"bomFormat"\s*:\s*"CycloneDX"/u.test(head)) return 'CYCLONEDX';
  if (/"spdxVersion"\s*:/u.test(head) || /^SPDXVersion:/mu.test(head)) return 'SPDX';
  return /^\s*[[{]/u.test(content) ? 'JSON' : 'CSV';
}

/** A CSV table's or a JSON array's column names, for the mapping's pickers. */
export function fileColumns(format: CiFileImport['format'], content: string): string[] {
  if (format === 'CSV') {
    const first = content.split(/\r?\n/u, 1)[0] ?? '';
    const separator = (first.match(/;/gu)?.length ?? 0) > (first.match(/,/gu)?.length ?? 0) ? ';' : ',';
    return splitCsvLine(first, separator).map((each) => each.trim()).filter(Boolean);
  }
  if (format === 'JSON') {
    try {
      const parsed: unknown = JSON.parse(content);
      const rows = Array.isArray(parsed) ? parsed : [];
      return [...new Set(rows.slice(0, 50).flatMap((row) => (row && typeof row === 'object' ? Object.keys(row) : [])))];
    } catch {
      return [];
    }
  }
  return [];
}

function splitCsvLine(line: string, separator: string): string[] {
  const out: string[] = [];
  let field = '';
  let quoted = false;
  for (let i = 0; i < line.length; i++) {
    const char = line[i];
    if (quoted) {
      if (char === '"' && line[i + 1] === '"') { field += '"'; i++; }
      else if (char === '"') quoted = false;
      else field += char;
    } else if (char === '"') quoted = true;
    else if (char === separator) { out.push(field); field = ''; }
    else field += char;
  }
  out.push(field);
  return out;
}

/**
 * A mapping guessed from the columns: a column named as an attribute of the class maps to it, and
 * `name` / `hostname` and `id` / `key` are the name and the source key. A person changes it.
 */
export function guessMapping(columns: readonly string[], attributes: readonly CiAttribute[]): { name?: string; nativeKey?: string; columns: Record<string, string> } {
  const lower = (each: string) => each.toLowerCase().replace(/[^a-z0-9]+/gu, '_');
  const name = columns.find((each) => ['name', 'hostname', 'title'].includes(lower(each)));
  const nativeKey = columns.find((each) => ['id', 'key', 'asset_id', 'native_key'].includes(lower(each)));
  const mapped: Record<string, string> = {};
  for (const column of columns) {
    if (column === name || column === nativeKey) continue;
    const attribute = attributes.find((each) => each.key === lower(column));
    if (attribute) mapped[column] = attribute.key;
  }
  return { ...(name ? { name } : {}), ...(nativeKey ? { nativeKey } : {}), columns: mapped };
}

// --- The administration's definitions (UC-AST-02 check 6, UC-AST-06 check 5) -------------------

export interface AttributeDraft {
  key: string;
  kind: CiAttribute['kind'];
  need: NonNullable<CiAttribute['need']>;
  identifier: boolean;
  /** A choice's options, one per line. */
  options: string;
}

export interface ClassDraft {
  key: string;
  parent: string;
  name: string;
  abstract: boolean;
  attributes: AttributeDraft[];
  /** One identifier rule per line, its attributes joined by `+`. */
  rules: string;
  required: { types: string[]; outgoing: boolean }[];
  staleness: string;
  archived: boolean;
}

const lines = (text: string) => text.split(/\r?\n/u).map((each) => each.trim()).filter(Boolean);

/** A class as its editor holds it: only what the class itself declares, not what it inherits. */
export function classDraft(of?: CiClass): ClassDraft {
  return {
    key: of?.key ?? '',
    parent: of?.parent ?? ROOT_CLASS,
    name: of?.name ?? '',
    abstract: of?.abstract ?? false,
    attributes: (of?.attributes ?? []).map((each) => ({
      key: each.key, kind: each.kind, need: each.need ?? 'OPTIONAL', identifier: each.identifier ?? false, options: (each.options ?? []).join('\n'),
    })),
    rules: (of?.identifier_rules ?? []).filter((rule) => !rule.class || rule.class === of?.key).map((rule) => rule.attributes.join('+')).join('\n'),
    required: (of?.required_relations ?? []).map((each) => ({ types: [...each.types], outgoing: each.outgoing })),
    staleness: of && of.staleness_days ? String(of.staleness_days) : '',
    archived: of?.archived ?? false,
  };
}

/** The body `defineCiClass` takes; a staleness left empty inherits (0). */
export function classDefine(draft: ClassDraft) {
  return {
    parent: draft.parent,
    name: draft.name.trim(),
    abstract: draft.abstract,
    attributes: draft.attributes.filter((each) => each.key.trim()).map((each) => ({
      key: each.key.trim(),
      kind: each.kind,
      need: each.need,
      identifier: each.identifier,
      ...(each.kind === 'CHOICE' ? { options: lines(each.options) } : {}),
    })),
    identifier_rules: lines(draft.rules).map((line) => ({ attributes: line.split('+').map((each) => each.trim()).filter(Boolean) })),
    required_relations: draft.required.filter((each) => each.types.length > 0),
    staleness_days: Math.max(0, Number.parseInt(draft.staleness, 10) || 0),
    archived: draft.archived,
  };
}

/** A key as a person may type one: small letters, digits and underscores, starting with a letter. */
export const keyIsWellFormed = (key: string) => /^[a-z][a-z0-9_]{0,62}$/u.test(key);

/** The text a mention or a card of a CI is written as in notes, a page or a board's words. */
export const ciReferenceText = (ciId: string, strength: 'MENTION' | 'CARD' = 'MENTION') =>
  `${strength === 'CARD' ? '!' : ''}[](hubtask:ci/${ciId})`;

export const backlinksPath = (ciId: string) => `/cis/${ciId}/backlinks`;
