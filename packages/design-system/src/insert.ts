// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The keys of the insert list (`InsertList`): the menus' roving and type-ahead, with Enter taking the
// active choice and Escape closing. A function over data, so the contract is tested without a browser.

import { rovingIndex, typeAheadIndex } from './focus.ts';

/** The new active index, `take`, `close`, or null when the key is not the list's. */
export function listKey(key: string, active: number, labels: readonly string[]): number | 'take' | 'close' | null {
  if (key === 'Enter') return labels.length > 0 ? 'take' : null;
  if (key === 'Escape') return 'close';
  return rovingIndex(key, active, labels.length) ?? typeAheadIndex(key, active, labels);
}

/** The words typed after "/" narrow the choices: a choice stays when its label or detail holds them. */
export function matchingGroups<C extends { label: string; detail?: string }, G extends { choices: C[] }>(
  groups: readonly G[],
  query: string,
): G[] {
  const needle = query.trim().toLocaleLowerCase();
  if (needle === '') return [...groups];
  const holds = (text: string | undefined) => text?.toLocaleLowerCase().includes(needle) ?? false;
  return groups
    .map((group) => ({ ...group, choices: group.choices.filter((choice) => holds(choice.label) || holds(choice.detail)) }))
    .filter((group) => group.choices.length > 0);
}

/** A text cut where it holds the typed words, so the list can mark them. */
export function markedParts(text: string, query: string): { text: string; isMatch: boolean }[] {
  const needle = query.trim().toLocaleLowerCase();
  if (needle === '') return [{ text, isMatch: false }];
  const parts: { text: string; isMatch: boolean }[] = [];
  const lower = text.toLocaleLowerCase();
  let from = 0;
  for (let at = lower.indexOf(needle); at !== -1; at = lower.indexOf(needle, from)) {
    if (at > from) parts.push({ text: text.slice(from, at), isMatch: false });
    parts.push({ text: text.slice(at, at + needle.length), isMatch: true });
    from = at + needle.length;
  }
  if (from < text.length) parts.push({ text: text.slice(from), isMatch: false });
  return parts;
}
