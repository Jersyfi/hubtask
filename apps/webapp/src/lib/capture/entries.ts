// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Turning a piece of text into entries (UC-AI-09), as far as it is decided without a screen: when
 * the capture field offers it, what a proposal holds, and which proposal is the one asked for.
 *
 * **Offered, never pushed.** *Make entries* is offered for pasted text of more than one line, where
 * AI is on and the text is going into a collection; nothing is shown otherwise (check 1). What a
 * proposal holds is the server's: only the fields the collection carries arrive (check 2), and
 * nothing is created before an entry is taken (check 3).
 */

import type { Suggestion } from '@hubtask/sync-engine';

/** One entry of a proposal, as the payload carries it. */
export interface ProposedEntry {
  /** What taking this one names (`SuggestionAcceptance.take`). */
  readonly key: string;
  readonly title: string;
  readonly notes?: string;
  readonly amount?: string;
  /** A calendar date, `YYYY-MM-DD`. */
  readonly dueDate?: string;
}

/** How many lines of text make it worth turning into entries rather than capturing as one. */
export const LINES_TO_MAKE_ENTRIES = 2;

/** What the offer depends on. */
export interface Offering {
  /** The manifest announces AI suggestions: the workspace has a provider and consented. */
  readonly aiOn: boolean;
  /** The text goes into a collection the person may create in - not the jumble, not nowhere. */
  readonly intoCollection: boolean;
}

/** The lines of a text that say something. */
const linesOf = (text: string): readonly string[] => text.split(/\r?\n/u).filter((line) => line.trim() !== '');

/** Whether the capture field offers *Make entries* for this text (UC-AI-09 check 1). */
export const offersMakeEntries = (text: string, offering: Offering): boolean =>
  offering.aiOn && offering.intoCollection && linesOf(text).length >= LINES_TO_MAKE_ENTRIES;

const textOf = (value: unknown): string | undefined => (typeof value === 'string' && value.trim() !== '' ? value : undefined);

/** The entries a proposal holds, in its order; nothing for any other kind. */
export function entriesOf(suggestion: Pick<Suggestion, 'kind' | 'payload'>): readonly ProposedEntry[] {
  if (suggestion.kind !== 'ENTRIES') return [];
  const listed = (suggestion.payload as { entries?: unknown }).entries;
  if (!Array.isArray(listed)) return [];
  return listed.flatMap((value): ProposedEntry[] => {
    const entry = value as Record<string, unknown>;
    const key = textOf(entry.key);
    const title = textOf(entry.title);
    if (!key || !title) return [];
    const notes = textOf(entry.notes);
    const amount = textOf(entry.amount);
    const dueDate = textOf(entry.due_date);
    return [{ key, title, ...(notes ? { notes } : {}), ...(amount ? { amount } : {}), ...(dueDate ? { dueDate } : {}) }];
  });
}

/**
 * A proposed calendar date as the reader's language writes it. The date is a day, not an instant:
 * drawn on UTC so that no zone moves it to the day before.
 */
export function dayOf(date: string, locale: string): string {
  const at = Date.parse(`${date}T00:00:00Z`);
  if (Number.isNaN(at)) return date;
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(at);
}

/** The collection a proposal's entries go into. */
export const collectionOf = (suggestion: Pick<Suggestion, 'payload'>): string | undefined =>
  textOf((suggestion.payload as { collection_id?: unknown }).collection_id);

/**
 * The proposal one ask produced: of the person's own standing proposals, the newest of entries for
 * this collection made since the ask. A rule draft or an answer in the same listing is not one.
 */
export function entriesProposalSince<T extends Pick<Suggestion, 'kind' | 'status' | 'created_at' | 'payload'>>(
  suggestions: readonly T[],
  askedAt: string,
  collectionId: string,
): T | undefined {
  const asked = Date.parse(askedAt);
  return suggestions
    .filter(
      (each) =>
        each.kind === 'ENTRIES' && each.status === 'PROPOSED' && Date.parse(each.created_at) >= asked && collectionOf(each) === collectionId,
    )
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))[0];
}
