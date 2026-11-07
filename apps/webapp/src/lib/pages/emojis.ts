// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The emoji picker's data: emojibase's set in the reader's language, loaded the first time the
 * picker opens and never with the page (the first-screen budget). A language emojibase does not
 * hold falls back to English. Pure apart from the loading, so the search has tests.
 */

import { emojiSets } from '../frame/parts.ts';

/** One emoji the picker offers. */
export interface PickerEmoji {
  readonly unicode: string;
  readonly label: string;
  readonly group: number;
  readonly order: number;
  readonly tags: readonly string[];
}

/** The groups the picker shows, in emojibase's order; the skin-tone components are not offered. */
export const EMOJI_GROUPS = [
  { order: 0, key: 'smileys_emotion' },
  { order: 1, key: 'people_body' },
  { order: 3, key: 'animals_nature' },
  { order: 4, key: 'food_drink' },
  { order: 5, key: 'travel_places' },
  { order: 6, key: 'activities' },
  { order: 7, key: 'objects' },
  { order: 8, key: 'symbols' },
  { order: 9, key: 'flags' },
] as const;

/** How many matches a search shows; a longer list is narrowed by typing on. */
export const MAX_MATCHES = 120;

type Raw = { unicode?: unknown; label?: unknown; group?: unknown; order?: unknown; tags?: unknown };

/** The entries the picker may offer: with a group it shows, its characters and a label. */
export function pickable(raw: readonly Raw[]): PickerEmoji[] {
  const shown = new Set<number>(EMOJI_GROUPS.map((group) => group.order));
  const out: PickerEmoji[] = [];
  for (const each of raw) {
    if (typeof each.unicode !== 'string' || typeof each.label !== 'string' || typeof each.group !== 'number' || !shown.has(each.group)) continue;
    out.push({
      unicode: each.unicode,
      label: each.label,
      group: each.group,
      order: typeof each.order === 'number' ? each.order : Number.MAX_SAFE_INTEGER,
      tags: Array.isArray(each.tags) ? each.tags.filter((tag): tag is string => typeof tag === 'string') : [],
    });
  }
  return out.sort((a, b) => a.order - b.order);
}

/**
 * The emoji whose label or a tag holds every word typed, the label's matches first; nothing typed
 * answers nothing (the groups are shown then).
 */
export function searchEmojis(all: readonly PickerEmoji[], query: string, locale: string): PickerEmoji[] {
  const words = query.toLocaleLowerCase(locale).split(/\s+/u).filter((word) => word !== '');
  if (words.length === 0) return [];
  const inLabel: PickerEmoji[] = [];
  const inTags: PickerEmoji[] = [];
  for (const each of all) {
    const label = each.label.toLocaleLowerCase(locale);
    const tags = each.tags.map((tag) => tag.toLocaleLowerCase(locale));
    if (!words.every((word) => label.includes(word) || tags.some((tag) => tag.includes(word)))) continue;
    (words.every((word) => label.includes(word)) ? inLabel : inTags).push(each);
    if (inLabel.length >= MAX_MATCHES) break;
  }
  return [...inLabel, ...inTags].slice(0, MAX_MATCHES);
}

const LOADERS = emojiSets as unknown as Readonly<Record<string, () => Promise<{ default: readonly Raw[] }>>>;

const loaded = new Map<string, Promise<PickerEmoji[]>>();

/** The emoji set in a language, loaded once; English where emojibase holds no set for it. */
export function loadEmojis(locale: string): Promise<PickerEmoji[]> {
  const language = locale.toLowerCase().split('-')[0] ?? 'en';
  const key = Object.hasOwn(LOADERS, language) ? language : 'en';
  let set = loaded.get(key);
  if (!set) {
    set = (LOADERS[key] as () => Promise<{ default: readonly Raw[] }>)().then((module) => pickable(module.default));
    set.catch(() => loaded.delete(key));
    loaded.set(key, set);
  }
  return set;
}
