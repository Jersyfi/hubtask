// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the capture field offers while a token is typed (design-system.md §11.8): `@` a collection,
 * `#` a hub, `+` a person, `!` a label. The person need not remember a name: the field lists the ones
 * that match as they type, and picking one writes the whole name, which `parse.ts` then reads as the
 * chip it always did. Pure, so the arithmetic is tested without a browser.
 */

/** The sigils and what each names, in the order the hint line lists them. */
export const SIGILS = { '@': 'collection', '#': 'hub', '+': 'person', '!': 'label' } as const;
export type Sigil = keyof typeof SIGILS;
export type TokenKind = (typeof SIGILS)[Sigil];

/** The token the caret stands in: its sigil, what is typed after it, and where it is. */
export interface Token {
  readonly sigil: Sigil;
  readonly query: string;
  readonly start: number;
  readonly end: number;
}

/** One name offered, with what tells it apart: a collection's hub. */
export interface Offer {
  readonly id: string;
  readonly name: string;
  readonly hint?: string;
}

/**
 * The token that ends at the caret: a sigil at the start of the text or after white space, and the
 * characters after it up to the caret, with no white space among them. A name with a space in it
 * is still found - it is what the list is for - once the first word has narrowed it.
 */
export function tokenAt(text: string, caret: number): Token | undefined {
  const before = text.slice(0, caret);
  const match = /(?:^|\s)([@#+!])([^\s@#+!]*)$/u.exec(before);
  if (!match) return undefined;
  const sigil = match[1] as Sigil;
  const query = match[2] ?? '';
  return { sigil, query, start: caret - query.length - 1, end: caret };
}

const fold = (value: string) => value.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLocaleLowerCase();

/**
 * The names that match what is typed: those that start with it first, then those with a word that
 * starts with it, then those that hold it anywhere - each group in the order given, at most `limit`.
 */
export function offersFor(query: string, candidates: readonly Offer[], limit = 6): readonly Offer[] {
  const wanted = fold(query.trim());
  if (wanted === '') return candidates.slice(0, limit);
  const starts: Offer[] = [];
  const words: Offer[] = [];
  const holds: Offer[] = [];
  for (const each of candidates) {
    const name = fold(each.name);
    if (name.startsWith(wanted)) starts.push(each);
    else if (name.split(/[\s\-_/]+/u).some((word) => word.startsWith(wanted))) words.push(each);
    else if (name.includes(wanted)) holds.push(each);
  }
  return [...starts, ...words, ...holds].slice(0, limit);
}

/** The text with the token replaced by the whole name and a space after it, and where the caret goes. */
export function pick(text: string, token: Token, name: string): { readonly text: string; readonly caret: number } {
  const written = `${token.sigil}${name} `;
  const after = text.slice(token.end).replace(/^\s/u, '');
  return { text: `${text.slice(0, token.start)}${written}${after}`, caret: token.start + written.length };
}
