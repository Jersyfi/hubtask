// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the palette lists, as one list in the order the arrow keys walk it (UC-WRK-27).
 *
 * The rows are a list and not a tree, because the arrow keys are: a reader pressing ↓ walks the
 * matches, then the quick narrowings, then the ways on, and the position in *this* array is the
 * whole keyboard model. Building it in one pure function keeps the field's key handling and the
 * panel's markup from counting differently - the bug every combobox written twice has, where the
 * highlight and the thing Enter presses drift apart by one.
 *
 * **The highlight follows a row, not a position.** The entries arrive a moment after the local
 * rows, and a highlight held as an index would slide onto another row under the reader's finger;
 * so it is held as the row's key and found again in each new list (`indexOfKey`).
 *
 * Pure, and tested beside itself.
 */

import type { Quick } from '../data/searchquery.ts';
import { score, type Score } from './rank.ts';

/**
 * What a candidate row is: a hub or collection, an action, a place or settings page, one row of a
 * settings screen, an entry, or a place this device opened last.
 */
export type CandidateKind = 'container' | 'action' | 'place' | 'setting' | 'hit' | 'recent';

/** One thing the palette could list, before ranking. */
export interface Candidate {
  readonly kind: CandidateKind;
  /** Unique within its kind: a container's or entry's id, an action's or place's own name. */
  readonly id: string;
  /** What is matched against the words, and what the row says. Resolved text. */
  readonly label: string;
  /**
   * Other words in the reader's language that find it, never drawn - "anlegen" for *Eintrag
   * hinzufügen*. Resolved text.
   */
  readonly keywords?: string;
  /** Where it is, said beside the label: the hub of a collection whose name is not unique. */
  readonly context?: string;
  /** The shortcut that does the same thing, by its id in `lib/shortcuts.ts`. */
  readonly shortcut?: string;
}

export type RowKind = CandidateKind | 'source' | 'quick' | 'all' | 'refine';

export interface Row {
  readonly kind: RowKind;
  /** The candidate's id, a source row's id, the narrowing's line, or the row's own name. */
  readonly id: string;
}

/** How many of each kind one answer lists: enough to choose from, few enough to read. */
export const CAPS: Readonly<Record<CandidateKind, number>> = { container: 5, action: 7, place: 5, setting: 5, hit: 5, recent: 5 };

/**
 * How many actions an empty palette lists: few enough to read at a glance - capturing, the
 * screen's own, then the rest of the frame's. Every other action is a word away.
 */
export const EMPTY_ACTIONS = 5;

/** The groups the list is drawn in, each under its own heading. */
export type Group = 'places' | 'actions' | 'settings' | 'entries' | 'recent';

/** Among groups that match equally well, the order they come in. */
const GROUP_ORDER: readonly Group[] = ['places', 'actions', 'settings', 'entries', 'recent'];

/** The group a kind is drawn in: a hub, a collection and a page are all places to go. */
export function groupOf(kind: CandidateKind): Group {
  if (kind === 'container' || kind === 'place') return 'places';
  if (kind === 'action') return 'actions';
  if (kind === 'setting') return 'settings';
  if (kind === 'hit') return 'entries';
  return 'recent';
}

/**
 * How well a candidate answers the words. A keyword counts as a word's start at most, so a row
 * that says the words ranks above one that only means them.
 */
function gradeOf(candidate: Candidate, words: string, locale?: string): Score {
  const byLabel = score(candidate.label, words, locale);
  const byKeywords = candidate.keywords ? Math.min(score(candidate.keywords, words, locale), 2) : 0;
  const found = Math.max(byLabel, byKeywords) as Score;
  return candidate.kind === 'hit' && found === 0 ? 1 : found;
}

/**
 * The candidates that answer the words, group by group, best first, each kind capped.
 *
 * **In the drawn order.** The list is drawn under a heading per group and the arrows walk what is
 * drawn, so the group whose best row matches best comes first, and within a group the best row
 * first: "ranked by match" (UC-WRK-27 check 2) inside the heading that says what a row is.
 *
 * With no words the palette is "where was I, and what can I do here": the places this device
 * opened last, then the actions - the screen's own before the frame's - and nothing is ranked
 * against an empty question. A place opened last is offered only then: with words it would repeat a container or
 * an entry found under its own heading. An entry the server returned matches even where no grade
 * here says so - it was found by a word form or by meaning - so it is kept at the lowest grade.
 */
export function rank(candidates: readonly Candidate[], words: string, locale?: string): readonly Candidate[] {
  if (words.trim() === '') {
    const actions = candidates.filter((each) => each.kind === 'action');
    const isScreens = (each: Candidate) => each.id.startsWith('page:');
    return [
      ...candidates.filter((each) => each.kind === 'recent').slice(0, CAPS.recent),
      // The frame's first - capturing - leads wherever one stands; then the screen's own acts, then
      // the rest of the frame's.
      ...((frame) => [...frame.slice(0, 1), ...actions.filter(isScreens), ...frame.slice(1)])(actions.filter((each) => !isScreens(each))).slice(0, EMPTY_ACTIONS),
    ];
  }
  const graded = candidates
    .filter((candidate) => candidate.kind !== 'recent')
    .map((candidate, order) => ({ candidate, grade: gradeOf(candidate, words, locale), order }))
    .filter((each) => each.grade > 0)
    .sort((a, b) => b.grade - a.grade || a.order - b.order);
  const taken: Record<CandidateKind, number> = { container: 0, action: 0, place: 0, setting: 0, hit: 0, recent: 0 };
  const kept: { readonly candidate: Candidate; readonly grade: Score }[] = [];
  for (const each of graded) {
    if (taken[each.candidate.kind] >= CAPS[each.candidate.kind]) continue;
    taken[each.candidate.kind] += 1;
    kept.push(each);
  }
  const best = new Map<Group, number>();
  for (const { candidate, grade } of kept) best.set(groupOf(candidate.kind), Math.max(best.get(groupOf(candidate.kind)) ?? 0, grade));
  const groups = [...best.keys()].sort((a, b) => (best.get(b) ?? 0) - (best.get(a) ?? 0) || GROUP_ORDER.indexOf(a) - GROUP_ORDER.indexOf(b));
  return groups.flatMap((group) => kept.filter((each) => groupOf(each.candidate.kind) === group).map((each) => each.candidate));
}

/**
 * The rows, in the order the arrows walk them: the matches, the rows another feature contributes
 * for these words (`source`), the quick narrowings, then the ways on.
 *
 * `refine` - the same question with the filter open - is offered only where there is something to
 * refine. With an empty field the one way on is the search screen itself, and a second row saying
 * almost the same thing would be a choice nobody has.
 */
export function rowsOf(
  words: string,
  matches: readonly Candidate[],
  quick: readonly Quick[],
  sourced: readonly string[] = [],
): readonly Row[] {
  const rows: Row[] = matches.map((each) => ({ kind: each.kind, id: each.id }));
  for (const id of sourced) rows.push({ kind: 'source', id });
  for (const each of quick) rows.push({ kind: 'quick', id: each.line });
  rows.push({ kind: 'all', id: 'all' });
  if (words.trim() !== '') rows.push({ kind: 'refine', id: 'refine' });
  return rows;
}

/** A row's identity, which is what the highlight holds. */
export function keyOf(row: Row): string {
  return `${row.kind}:${row.id}`;
}

/** Where the row with this key is now, or `-1` when it is gone (or nothing was highlighted). */
export function indexOfKey(rows: readonly Row[], key: string | undefined): number {
  if (key === undefined) return -1;
  return rows.findIndex((row) => keyOf(row) === key);
}

/**
 * The element id of a row, for `aria-activedescendant`: derived from the row's identity, so the
 * row a screen reader was told about keeps its id when the list changes around it. An id may not
 * hold a space, and a narrowing's line holds several, so every character outside a safe set is
 * written as its code.
 */
export function rowId(row: Row): string {
  const safe = keyOf(row).replace(/[^A-Za-z0-9_-]/g, (character) => `_${character.codePointAt(0)?.toString(16) ?? ''}`);
  return `palette-row-${safe}`;
}

/**
 * Where ↓ and ↑ land, counted from "nothing highlighted".
 *
 * `-1` is that state and it is a real one rather than a placeholder: a field somebody is typing in
 * has no row highlighted, and Enter then means "search for what I typed" rather than "open whatever
 * happened to be first". Walking past either end returns to it, so the list cannot be left without
 * a way back to one's own words.
 */
export function step(active: number, count: number, by: 1 | -1): number {
  if (count === 0) return -1;
  const next = active + by;
  if (next >= count) return -1;
  if (next < -1) return count - 1;
  return next;
}
