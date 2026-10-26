// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * An entry's ticket key (ADR-0094): its collection's handle in capitals and its number in the
 * collection - `EINKAUF-42`. The reference people quote in a mail subject or a reply, and what
 * the search takes to jump straight to the entry.
 */
export function ticketKeyOf(handle: string | null | undefined, number: number | null | undefined): string | undefined {
  if (!handle || !number) return undefined;
  return `${handle.toUpperCase()}-${number}`;
}

/** The handle and number a ticket key names, or undefined for text that is not one. */
export function parseTicketKey(text: string): { handle: string; number: number } | undefined {
  const match = /^\s*\[?([A-Za-z][A-Za-z0-9]{1,19})-(\d{1,12})\]?\s*$/.exec(text);
  const [, handle, number] = match ?? [];
  if (!handle || !number) return undefined;
  return { handle: handle.toLowerCase(), number: Number(number) };
}
