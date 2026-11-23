// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What an edit form writes: the fields that moved, and nothing else.
 *
 * `offline-sync.md` §4.2: a field the person did not touch is not in the log at all. A payload
 * that repeated the title beside the notes would, offline, queue the title with a fresh clock -
 * and that clock would beat a title another device changed after the form opened and before
 * the push. Online the server's merge-patch semantics hide it (an unchanged value written over
 * itself); the queue does not, which is why the comparison happens here rather than being left
 * to the server. The comparison is against what the form opened with, not against the entry as
 * it is now: the person's own edits are what count as touched.
 */

/** The three texts of the entry's edit form, as the form holds them. Empty is "none". */
export interface EntryDraft {
  readonly title: string;
  readonly notes: string;
  readonly language: string;
}

/** What the form writes: absent is untouched, `null` clears. */
export interface EntryEdit {
  title?: string;
  notes?: string | null;
  content_language?: string | null;
}

/**
 * The fields of `draft` that differ from `opened`, in the contract's shape.
 *
 * The title is trimmed before the comparison, so a space typed and deleted is not a change. Empty
 * notes clear them rather than setting them to the empty string: the contract's `null` is "there
 * are none", and a note of zero characters is not a note somebody wrote. The same for the
 * language: `null` clears a stated one.
 */
export function entryEditOf(opened: EntryDraft, draft: EntryDraft): EntryEdit {
  const title = draft.title.trim();
  const notes = draft.notes.trim() === '' ? null : draft.notes;
  const openedNotes = opened.notes.trim() === '' ? null : opened.notes;
  return {
    ...(title !== opened.title.trim() ? { title } : {}),
    ...(notes !== openedNotes ? { notes } : {}),
    ...(draft.language !== opened.language ? { content_language: draft.language || null } : {}),
  };
}

/** The two texts of an entry somebody else can change while a person writes in place. */
export type TextField = 'title' | 'notes';

/**
 * The write of an edit in place, decided three ways (UC-SYN-02 check 10): what each field was when
 * the person began (`base`), what the person holds (`mine`) and what the entry holds now
 * (`theirs`). A field the person did not change is not sent, whatever arrived meanwhile - sending
 * it back would undo the other person's change with a version `If-Match` accepts. A field changed
 * by both to different texts is a conflict: nothing is written until the person chooses.
 */
export interface InlineWrite {
  readonly edit: EntryEdit;
  readonly conflicts: readonly TextField[];
}

export function inlineWriteOf(base: EntryDraft, mine: EntryDraft, theirs: EntryDraft): InlineWrite {
  const own = entryEditOf(base, mine);
  const arrived = entryEditOf(base, theirs);
  const edit: EntryEdit = {};
  const conflicts: TextField[] = [];
  for (const field of ['title', 'notes'] as const) {
    if (!(field in own)) continue;
    if (!(field in arrived)) edit[field] = own[field] as never;
    else if (own[field] !== arrived[field]) conflicts.push(field);
  }
  if ('content_language' in own) edit.content_language = own.content_language;
  return { edit, conflicts };
}
