// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Links and references, as the screens read them (UC-PLN-01): what a link is called from each of
 * its ends, which links a timeline draws as dependencies, and the references a text holds.
 *
 * **A reference is written, never resolved, here.** Text holds `[](hubtask:entry/<id>)` - with no
 * title, because a title written into the text would be read by everybody who reads the text,
 * including a reader who may not see the entry. What the reader sees of it comes from
 * `POST /references:resolve`, which answers only the address and `available: false` for what is
 * out of their sight (check 4).
 */

import type { EntryLink, EntryLinks, LinkType, Reference, ReferenceKind, ReferenceStrength, ReferenceTarget } from '@hubtask/sync-engine';

/** The six types every workspace has. A shipped type answers null names; the client words them. */
export const SYSTEM_LINK_KEYS = ['blocks', 'depends_on', 'duplicates', 'runs_on', 'affects', 'implements'] as const;

const isSystemKey = (key: string): boolean => (SYSTEM_LINK_KEYS as readonly string[]).includes(key);

/**
 * What a link is called from one of its ends: the type's name from its source, its inverse name
 * from its target. A shipped type is worded by the catalogue (`app.links.out.*`, `app.links.in.*`);
 * a symmetric type reads the same from both ends.
 */
export function sideName(
  type: Pick<LinkType, 'key' | 'name' | 'inverse_name' | 'direction'> | undefined,
  key: string,
  isOutward: boolean,
  t: (code: string) => string,
): string {
  if (type === undefined || (type.name === null && isSystemKey(key))) {
    if (isSystemKey(key)) return t(`app.links.${isOutward ? 'out' : 'in'}.${key}`);
    return key;
  }
  if (isOutward || type.direction === 'SYMMETRIC') return type.name ?? key;
  return type.inverse_name ?? type.name ?? key;
}

/** The types a new link may be drawn with: not archived, in the registry's order. */
export function drawableTypes(types: readonly LinkType[]): LinkType[] {
  return [...types].filter((type) => !type.archived).sort((a, b) => a.position - b.position);
}

/** One side of a link as the entry's panel lists it: the relation's name and who is at the far end. */
export interface LinkRow {
  readonly id: string;
  readonly name: string;
  readonly other: ReferenceTarget;
}

/** One relation and every entry at its far end, in the order the links were drawn. */
export interface LinkGroup {
  readonly name: string;
  readonly rows: readonly LinkRow[];
}

/** The entry's links grouped by what they are called from this entry ("blocks", "is blocked by"). */
export function groupLinks(
  links: EntryLinks['links'],
  types: readonly LinkType[],
  t: (code: string) => string,
): LinkGroup[] {
  const groups = new Map<string, LinkRow[]>();
  for (const link of links) {
    const type = types.find((each) => each.key === link.type);
    const name = sideName(type, link.type, link.outward, t);
    const rows = groups.get(name) ?? [];
    rows.push({ id: link.id, name, other: link.other });
    groups.set(name, rows);
  }
  return [...groups].map(([name, rows]) => ({ name, rows }));
}

/** An entry as the timeline places it: its start and due day, `YYYY-MM-DD`. */
export interface Dated {
  readonly id: string;
  readonly start?: string;
  readonly due?: string;
}

/** A dependency the timeline draws: the entry waited for, the waiting one, and whether it starts too early. */
export interface Dependency {
  readonly id: string;
  readonly waitedFor: string;
  readonly waiting: string;
  readonly isEarly: boolean;
}

/**
 * The links among the shown entries that are dependencies, each pointed from the entry waited for
 * to the waiting one (check 6). `waits` names the waiting end: SOURCE for "depends on", TARGET for
 * "blocks". A waiting entry is early when it starts - or, with no start, is due - before the entry
 * it waits for is due.
 */
export function dependenciesAmong(links: readonly EntryLink[], dated: readonly Dated[]): Dependency[] {
  const byId = new Map(dated.map((entry) => [entry.id, entry]));
  const found: Dependency[] = [];
  for (const link of links) {
    if (link.waits !== 'SOURCE' && link.waits !== 'TARGET') continue;
    const waiting = link.waits === 'SOURCE' ? link.source_id : link.target_id;
    const waitedFor = link.waits === 'SOURCE' ? link.target_id : link.source_id;
    const first = byId.get(waitedFor);
    const second = byId.get(waiting);
    if (!first || !second) continue;
    const begins = second.start ?? second.due;
    const isEarly = first.due !== undefined && begins !== undefined && begins < first.due;
    found.push({ id: link.id, waitedFor, waiting, isEarly });
  }
  return found;
}

/** One reference a text holds, with how it is drawn there. */
export interface FoundReference {
  readonly reference: Reference;
  readonly strength: ReferenceStrength;
}

const KINDS: readonly ReferenceKind[] = ['ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT', 'CI'];
const VIEWABLE: ReadonlySet<ReferenceKind> = new Set(['VIEW', 'DASHBOARD', 'WHITEBOARD']);

/** The server's grammar (`reference.Extract`): a Markdown link to an address, an image of one for a card. */
const ADDRESS =
  /(!?)\[[^\]]*\]\(hubtask:([a-z]+)\/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:#([A-Za-z0-9_-]{1,64}))?\)/g;

const RANK: Record<ReferenceStrength, number> = { MENTION: 0, CARD: 1, EMBED: 2 };

/**
 * The references a text holds, once each per object and anchor, the strongest drawing winning -
 * what the server keeps as the text's references, read the same way so the panel and the
 * backlinks agree. A malformed address is text.
 */
export function referencesIn(markdown: string): FoundReference[] {
  if (!markdown.includes('hubtask:')) return [];
  const found: FoundReference[] = [];
  const index = new Map<string, number>();
  for (const match of markdown.matchAll(ADDRESS)) {
    const kind = (match[2] ?? '').toUpperCase() as ReferenceKind;
    if (!KINDS.includes(kind)) continue;
    const id = (match[3] ?? '').toLowerCase();
    const anchor = match[4];
    const strength: ReferenceStrength = match[1] !== '!' ? 'MENTION' : VIEWABLE.has(kind) ? 'EMBED' : 'CARD';
    const key = `${kind}/${id}#${anchor ?? ''}`;
    const at = index.get(key);
    const seen = at === undefined ? undefined : found[at];
    if (at !== undefined && seen !== undefined) {
      if (RANK[strength] > RANK[seen.strength]) found[at] = { ...seen, strength };
      continue;
    }
    index.set(key, found.length);
    found.push({ reference: { kind, id, ...(anchor ? { anchor } : {}) }, strength });
  }
  return found;
}

/** The text a mention or a card of an entry is written as - an address and no title. */
export function referenceText(id: string, strength: 'MENTION' | 'CARD'): string {
  return `${strength === 'CARD' ? '!' : ''}[](hubtask:entry/${id})`;
}

/**
 * The text with a reference put where the caret was, replacing a selection, with a space on
 * either side where it would otherwise run into a word; and where the caret lands after it.
 */
export function inserted(text: string, start: number, end: number, insertion: string): { text: string; caret: number } {
  const before = text.slice(0, start);
  const after = text.slice(end);
  const lead = before === '' || /\s$/.test(before) ? '' : ' ';
  const tail = after === '' || /^\s/.test(after) ? '' : ' ';
  const written = `${lead}${insertion}${tail}`;
  return { text: `${before}${written}${after}`, caret: before.length + written.length };
}

/** What a resolved reference reads as: its key and title, or nothing where it is out of sight. */
export function targetLabel(target: ReferenceTarget | undefined): string | undefined {
  if (!target?.available) return undefined;
  const title = target.title ?? '';
  return target.key ? `${target.key} ${title}`.trim() : title;
}
