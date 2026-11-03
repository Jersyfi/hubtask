// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What one search answers beyond entries (user-experience.md §5.3): the kinds it fans out to,
 * each only where its module is on, and the plain-word match the kinds without a server search
 * use - a board's title, a hub's or a collection's name.
 */

export const SEARCH_KINDS = ['all', 'entries', 'pages', 'boards', 'places'] as const;
export type SearchKind = (typeof SEARCH_KINDS)[number];

/** The kinds offered: entries and places always, the others where their module is on. */
export function kindsOffered(modulesOn: readonly string[] | undefined): SearchKind[] {
  const on = (module: string) => modulesOn?.includes(module) ?? false;
  return SEARCH_KINDS.filter((kind) => (kind === 'pages' ? on('KNOWLEDGE') : kind === 'boards' ? on('WHITEBOARD') : true));
}

/** Whether a kind's results are drawn under the kind chosen. */
export const shows = (chosen: SearchKind, kind: Exclude<SearchKind, 'all'>): boolean => chosen === 'all' || chosen === kind;

/**
 * Whether a name holds every word typed, ignoring case and accents: the match of the kinds the
 * server has no search for. A name is short, so "holds every word" is the whole question.
 */
export function nameMatches(name: string, words: string): boolean {
  const fold = (text: string) => text.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLocaleLowerCase();
  const wanted = fold(words).split(/\s+/u).filter((word) => word !== '');
  if (wanted.length === 0) return false;
  const held = fold(name);
  return wanted.every((word) => held.includes(word));
}
