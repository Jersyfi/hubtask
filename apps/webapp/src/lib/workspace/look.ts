// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a hub or a collection looks like: the symbol and the colour its people chose (UC-WRK-33).
//
// The server stores the symbol as a free string (`icon`) and refuses a new colour outside the ten
// label tokens (`color_token`); this is where the client decides what it offers and how it reads
// back what it finds. A value it does not offer - written by a script, an older client, another door
// or before the server checked the colour - is read as "none chosen" rather than drawn as a guess:
// an unknown symbol would be a missing glyph, an unknown colour a tile with no pair behind it.
//
// The colour is the place's, never its level's (design-system.md §6): nothing here derives one
// from the type. The type decides only the mark of a place nobody chose a symbol for, which is the
// mark the navigation drew before there was a choice.

import { labelTokens, type LabelToken } from '@hubtask/design-system';
import type { IconName } from '@hubtask/design-system/icons';

/**
 * The symbols on offer, in the order the picker shows them. Each is declared in the design
 * system's icon list under the group that asks for them; a name added here and not there does not
 * compile.
 */
export const SYMBOLS = [
  'house',
  'briefcase',
  'heart',
  'book-open',
  'graduation-cap',
  'dumbbell',
  'stethoscope',
  'plane',
  'bike',
  'shopping-cart',
  'wallet',
  'wrench',
  'hammer',
  'code',
  'lightbulb',
  'rocket',
  'flag',
  'music',
  'camera',
  'palette',
  'leaf',
  'paw-print',
  'utensils',
  'gift',
] as const satisfies readonly IconName[];

export type PlaceSymbol = (typeof SYMBOLS)[number];

/** What a container carries for its look; the API's own fields, so any container read fits. */
export interface Look {
  readonly type: 'HUB' | 'COLLECTION';
  readonly icon?: string | null;
  readonly color_token?: string | null;
}

/** The chosen symbol, where it is one this client offers. */
export function chosenSymbol(look: Pick<Look, 'icon'>): PlaceSymbol | undefined {
  const icon = look.icon?.trim();
  return icon && (SYMBOLS as readonly string[]).includes(icon) ? (icon as PlaceSymbol) : undefined;
}

/** The chosen colour, where it is one of the ten label tokens. */
export function chosenColour(look: Pick<Look, 'color_token'>): LabelToken | undefined {
  const token = look.color_token?.trim();
  return token && (labelTokens as readonly string[]).includes(token) ? (token as LabelToken) : undefined;
}

/** The mark to draw: the chosen symbol, or the mark of the container's type where none is. */
export function markOf(look: Pick<Look, 'type' | 'icon'>): IconName {
  return chosenSymbol(look) ?? (look.type === 'HUB' ? 'hub' : 'collection');
}

/**
 * What a create or an edit sends for the two fields: the choice, or `null` for none. `null`
 * rather than leaving the field out, because on an edit "none" is a change - the server clears a
 * field sent as null and leaves one alone that was not sent.
 */
export function lookBody(symbol: PlaceSymbol | undefined, colour: LabelToken | undefined): { icon: string | null; color_token: string | null } {
  return { icon: symbol ?? null, color_token: colour ?? null };
}
