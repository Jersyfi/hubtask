// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a write to an entry makes stale, named precisely.
 *
 * Every write used to name `/items` - the prefix of everything - and so a retitled entry re-read
 * its comments, its reminders, its attachments and its series along with itself, none of which
 * had moved; on a real server a title saved once was twenty-one requests, and the third write of
 * a session met the credential's burst (issue 877). The engine's names take two marks since then:
 * a trailing `$` ends the name at the path itself, and `*` stands for one segment - so these are
 * the three things an entry's own write changes, and the one shape a write that may reach other
 * entries takes.
 *
 * Pure and tested, and imported by every store that writes an entry and by `live.ts`, which maps
 * a record to the same names: the write's answer and the record for it must agree on what is
 * stale, or one of them reloads something the other did not.
 */

/** Every list and board: `POST /items:query`, whatever its document. */
export const ENTRY_LISTS = '/items:query';

/** The entry's own document, with or without `?expand=labels`, and nothing under it. */
export const entryDocument = (id: string): string => `/items/${id}$`;

/** The entry's history: every change to it is a step somebody may be reading. */
export const entryHistory = (id: string): string => `/items/${id}/activity`;

/** Every entry document a screen holds open - the open entry, and the one beside the list. */
export const ANY_ENTRY_DOCUMENT = '/items/*$';

/** Every entry history a screen holds open. */
export const ANY_ENTRY_HISTORY = '/items/*/activity';

/** What a write to one entry's own fields makes stale: the lists, its document, its history. */
export function touchesOf(id: string): readonly string[] {
  return [ENTRY_LISTS, entryDocument(id), entryHistory(id)];
}

/**
 * What a write that may reach entries beyond the one named makes stale - a move, a completion
 * (a parent's rule may complete the parent), an archive (the subtree follows), a bulk, a trash.
 * Every open document and history, and the lists; still not the threads and the reminders, which
 * no such write changes.
 */
export const TOUCHES_ANY_ENTRY: readonly string[] = [ENTRY_LISTS, ANY_ENTRY_DOCUMENT, ANY_ENTRY_HISTORY];
