// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One search over every kind (UC-WRK-18 check 9): what the header search and the
 * search page draw from `POST /search:all`. Pure, so the scopes, the best hits, the way a hit
 * opens and the hint for a `word:` that names no field are tested without mounting anything.
 *
 * The line travels whole as `q` in the body, never in an address (Q-5, UC-WRK-18/4); the server
 * keeps the kinds a key belongs to and narrows every hit and count to what the reader may read.
 */

import { resolveKey } from '@hubtask/query';

/** The kinds `/search:all` answers, as the contract names them. */
export type Kind = 'ENTRY' | 'TICKET' | 'PAGE' | 'WHITEBOARD' | 'GOAL' | 'DASHBOARD' | 'ASSET' | 'PERSON' | 'FILE' | 'COMMENT';

export interface Hit {
  readonly kind: Kind;
  readonly id: string;
  readonly title: string;
  readonly hub_id: string | null;
  readonly collection_id: string | null;
  readonly tile: string | null;
  readonly snippet: string | null;
  readonly state: string | null;
  readonly key: string | null;
  readonly updated_at: string | null;
  readonly on: { readonly kind: 'ENTRY' | 'PAGE' | 'WHITEBOARD'; readonly id: string } | null;
}

export interface Facet {
  readonly kind: Kind;
  readonly count: number;
  readonly more: boolean;
}

export interface Answer {
  readonly data: readonly Hit[];
  readonly facets: readonly Facet[];
}

/** The scopes of the header search, in the order the concept draws them (§6.11). */
export const BAR_KINDS: readonly Kind[] = ['ENTRY', 'TICKET', 'PAGE', 'WHITEBOARD', 'PERSON', 'FILE', 'ASSET'];

/** Every kind of the search page, entries first, comments last. */
export const PAGE_KINDS: readonly Kind[] = ['ENTRY', 'TICKET', 'PAGE', 'WHITEBOARD', 'GOAL', 'DASHBOARD', 'ASSET', 'PERSON', 'FILE', 'COMMENT'];

/**
 * The module a kind needs, by the manifest's module keys; a kind whose module is off is never
 * asked and never drawn (P-17). Without TICKET asked, the server counts tickets as entries.
 */
const MODULE: Partial<Record<Kind, string>> = { TICKET: 'SERVICE_DESK', PAGE: 'KNOWLEDGE', WHITEBOARD: 'WHITEBOARD', GOAL: 'GOALS', DASHBOARD: 'GOALS', ASSET: 'ASSETS' };

/** The kinds of `wanted` this installation answers, as its modules are on. */
export function kindsOn(wanted: readonly Kind[], modulesOn: readonly string[] | undefined): Kind[] {
  return wanted.filter((kind) => {
    const module = MODULE[kind];
    return module === undefined || (modulesOn ?? []).includes(module);
  });
}

/** The body of `POST /search:all`. Undefined when nothing is asked: the server needs a line. */
export function requestOf(line: string, kinds: readonly Kind[], options: { hubId?: string; size?: number } = {}) {
  const q = line.trim().slice(0, 200);
  if (q === '' || kinds.length === 0) return undefined;
  return { q, kinds: [...kinds], ...(options.hubId ? { hub_id: options.hubId } : {}), ...(options.size ? { size: options.size } : {}) };
}

/** A scope chip: a kind with its count, *all*, or the hub one stands in. */
export interface Scope {
  readonly id: 'ALL' | 'HERE' | Kind;
  readonly count?: number;
  readonly more?: boolean;
}

/**
 * The chips over the hits: *all* with the sum, every kind that found something with its count,
 * and *here: hub* where one stands in a hub. A kind with no hit is not a chip - a scope that
 * would show nothing is noise.
 */
export function scopesOf(facets: readonly Facet[], kinds: readonly Kind[], hasHere: boolean): Scope[] {
  const counted = kinds.flatMap((kind) => {
    const facet = facets.find((each) => each.kind === kind);
    return facet && facet.count > 0 ? [{ id: kind, count: facet.count, more: facet.more }] : [];
  });
  const total = counted.reduce((sum, each) => sum + each.count, 0);
  return [
    { id: 'ALL', count: total, more: counted.some((each) => each.more) },
    ...counted,
    ...(hasHere ? [{ id: 'HERE' as const }] : []),
  ];
}

/**
 * The best hits and the other kinds, as the header search draws them: the best hit of each of the
 * first three kinds that found something, then one of every other kind. Narrowed to one kind, the
 * hits of that kind are all best hits.
 */
export function arrange(hits: readonly Hit[], kinds: readonly Kind[], narrowed?: Kind, best = 3): { best: Hit[]; more: Hit[] } {
  if (narrowed) return { best: hits.filter((hit) => hit.kind === narrowed).slice(0, 8), more: [] };
  const firsts = kinds.flatMap((kind) => {
    const first = hits.find((hit) => hit.kind === kind);
    return first ? [first] : [];
  });
  return { best: firsts.slice(0, best), more: firsts.slice(best) };
}

function objectPath(kind: string, id: string): string | undefined {
  switch (kind) {
    case 'ENTRY':
    case 'TICKET':
      return `/items/${id}`;
    case 'PAGE':
      return `/pages/${id}`;
    case 'WHITEBOARD':
      return `/whiteboards/${id}`;
    case 'GOAL':
      return `/goals/${id}`;
    case 'DASHBOARD':
      return `/dashboards/${id}`;
    case 'ASSET':
      return `/cis/${id}`;
    default:
      return undefined;
  }
}

/**
 * Where a hit opens: the object itself, or for a file and a comment what it is on. A person has
 * no page of their own; their hit searches their entries instead (`personLine`).
 */
export function pathOf(hit: Hit): string | undefined {
  if (hit.kind === 'FILE' || hit.kind === 'COMMENT') return hit.on ? objectPath(hit.on.kind, hit.on.id) : undefined;
  return objectPath(hit.kind, hit.id);
}

/** The line that finds a person's entries: their identifier, which the chip names (Q-5). */
export const personLine = (hit: Hit) => `who: ${hit.id}`;

/** A snippet's pieces, the found words marked: the server writes them between « and ». */
export function marked(snippet: string): { readonly text: string; readonly isFound: boolean }[] {
  const pieces: { text: string; isFound: boolean }[] = [];
  const pattern = /«([^»]*)»/gu;
  let at = 0;
  for (let match = pattern.exec(snippet); match; match = pattern.exec(snippet)) {
    if (match.index > at) pieces.push({ text: snippet.slice(at, match.index), isFound: false });
    pieces.push({ text: match[1] ?? '', isFound: true });
    at = match.index + match[0].length;
  }
  if (at < snippet.length) pieces.push({ text: snippet.slice(at), isFound: false });
  return pieces;
}

/** A title's pieces, marked where they hold a typed word - titles carry no «». */
export function markWords(title: string, words: string): { readonly text: string; readonly isFound: boolean }[] {
  const terms = words
    .split(/\s+/u)
    .filter((word) => word !== '' && !word.includes(':'))
    .map((word) => word.replace(/^"|"$/gu, '').replace(/[.*+?^${}()|[\]\\]/gu, '\\$&'))
    .filter((word) => word.length > 1);
  if (terms.length === 0) return [{ text: title, isFound: false }];
  return marked(title.replace(new RegExp(`(${terms.join('|')})`, 'giu'), '«$1»'));
}

/** A typed key such as `SA-12` or `TAMA-18`: the words are one, and Enter jumps to it. */
export function keyIn(words: string): string | undefined {
  const typed = words.trim();
  return /^[A-Za-z][A-Za-z0-9]{0,9}-\d{1,9}$/u.test(typed) ? typed.toUpperCase() : undefined;
}

/**
 * A `word:` that names no key of the language is words, not a dead end (Q-2, the build plan's
 * decision): "Re: Besprechung" is searched as `"Re:" Besprechung`. Answers the line to send and
 * the words taken as words, which the line names at its chip.
 */
export function wordsForUnknownKeys(line: string, known: ReadonlySet<string>): { line: string; asWords: string[] } {
  const asWords: string[] = [];
  const rewritten = line.replace(/(^|\s)(-?)([A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*):/gu, (whole, lead: string, minus: string, key: string) => {
    if (known.has(key) || resolveKey(key) !== undefined || key.startsWith('cf.')) return whole;
    asWords.push(`${key}:`);
    return `${lead}${minus}"${key}:"`;
  });
  return { line: rewritten, asWords };
}

/** How long ago a hit was changed, as the search page's facet counts it. */
export type Since = 'TODAY' | 'WEEK' | 'OLDER';

export function sinceOf(updatedAt: string | null, now: Date): Since {
  if (!updatedAt) return 'OLDER';
  const at = new Date(updatedAt).getTime();
  const startOfDay = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  if (at >= startOfDay) return 'TODAY';
  return at >= startOfDay - 6 * 86_400_000 ? 'WEEK' : 'OLDER';
}

/** Counts of the hits read, by a property: the hub facets and the *changed* facets. */
export function countBy<T extends string>(hits: readonly Hit[], of: (hit: Hit) => T | null): Map<T, number> {
  const counts = new Map<T, number>();
  for (const hit of hits) {
    const value = of(hit);
    if (value !== null) counts.set(value, (counts.get(value) ?? 0) + 1);
  }
  return counts;
}

/** The orders of the search page: relevance is the server's; the others sort what was found. */
export type Order = 'RELEVANCE' | 'UPDATED' | 'TITLE';

export function ordered(hits: readonly Hit[], order: Order, locale: string): Hit[] {
  if (order === 'RELEVANCE') return [...hits];
  if (order === 'UPDATED') return [...hits].sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? ''));
  const collator = new Intl.Collator(locale);
  return [...hits].sort((a, b) => collator.compare(a.title, b.title));
}

/** A hit's tile: the product's colour and mark, which identify and mean nothing (WB-2). */
export function tileOf(hit: Hit): { readonly icon: string; readonly product?: 'pages' | 'whiteboards' | 'goals' | 'assets' | 'desk'; readonly color?: string } {
  const on = hit.kind === 'FILE' || hit.kind === 'COMMENT' ? (hit.on?.kind ?? 'ENTRY') : hit.kind;
  const mark = hit.kind === 'FILE' ? 'paperclip' : hit.kind === 'COMMENT' ? 'message-square' : undefined;
  switch (on) {
    case 'PAGE':
      return { icon: mark ?? 'book-open', product: 'pages' };
    case 'WHITEBOARD':
      return { icon: mark ?? 'layout-template', product: 'whiteboards' };
    case 'GOAL':
      return { icon: 'flag', product: 'goals' };
    case 'DASHBOARD':
      return { icon: 'layout-grid', product: 'goals' };
    case 'ASSET':
      return { icon: 'shield', product: 'assets' };
    case 'PERSON':
      return { icon: 'user' };
    case 'TICKET':
      return { icon: hit.state === 'DONE' ? 'square-check' : 'inbox', product: 'desk' };
    default:
      return { icon: mark ?? (hit.state === 'DONE' ? 'square-check' : 'square'), color: 'brand' };
  }
}
