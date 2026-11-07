// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** What a key is read from: the collection's handle and its key prefix, where it has one. */
export interface KeyedCollection {
  readonly handle?: string | null;
  readonly key_prefix?: string | null;
}

/**
 * An entry's ticket key (ADR-0094, user-experience.md §5.6): its collection's key prefix and its
 * number - `INC-42` - or the handle in capitals where the collection has no prefix. Drawn this one
 * way in a list, a panel, a board card and a mail; the reference people quote in a subject or a
 * reply, and what the search takes to jump straight to the entry.
 */
export function ticketKeyOf(collection: KeyedCollection | null | undefined, number: number | null | undefined): string | undefined {
  const word = collection?.key_prefix || collection?.handle?.toUpperCase();
  if (!word || !number) return undefined;
  return `${word}-${number}`;
}

/** The handle and number a ticket key names, or undefined for text that is not one. */
export function parseTicketKey(text: string): { handle: string; number: number } | undefined {
  const match = /^\s*\[?([A-Za-z][A-Za-z0-9]{1,19})-(\d{1,12})\]?\s*$/.exec(text);
  const [, handle, number] = match ?? [];
  if (!handle || !number) return undefined;
  return { handle: handle.toLowerCase(), number: Number(number) };
}
