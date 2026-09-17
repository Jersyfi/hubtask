// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How names are ordered when the client orders them (i18n-l10n.md §6 line 3, F5-09).
 *
 * The server sorts under one collation on every installation (M-08, `natural_ordering`), and a
 * list that arrives sorted is left as it arrives. A list the client *builds* - the people a
 * picker may name, gathered from four memberships - has no order until something gives it one,
 * and `<` on strings gives it the order of code units: `Ä` after `Z`, `é` after `z`. The
 * collator for the resolved locale gives it the reader's order instead.
 */

const collators = new Map<string, Intl.Collator>();

/** The collator for a locale, made once. `sensitivity: 'base'` - a name is not two names by case. */
export function collatorFor(locale: string): Intl.Collator {
  let collator = collators.get(locale);
  if (!collator) {
    collator = new Intl.Collator(locale, { sensitivity: 'base', numeric: true });
    collators.set(locale, collator);
  }
  return collator;
}

/** A comparison of two names under the locale, for `Array.prototype.sort`. */
export function byName(locale: string): (left: string, right: string) => number {
  const collator = collatorFor(locale);
  return (left, right) => collator.compare(left, right);
}

