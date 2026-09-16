// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A number typed the way the reader writes one (i18n-l10n.md §6 line 10, F5-09).
 *
 * The contract carries a `NUMBER` field as a number, and a person under `de` types `1,5`. What
 * separates the decimals is the installation's answer first - `supported_locales[].decimal_separator`
 * (M-05) - and `Intl.NumberFormat`'s for a locale the manifest does not describe. The text is
 * parsed *before* the raw value reaches the contract, so the server sees `1.5` and never a comma.
 *
 * The parse is deliberately narrow: an optional sign, digits, at most one separator. A grouping
 * separator is not read - `1.500` under `de` is one thousand five hundred to a reader and one and
 * a half to a parser that took the dot as decimal, and a field that guessed would be worse than
 * one that asked again.
 */

import type { SupportedLocale } from './locale.ts';

/** The decimal separator the locale writes with: the manifest's word, else `Intl`'s, else a dot. */
export function decimalSeparatorOf(locale: string, supported: readonly SupportedLocale[] = []): string {
  const declared = supported.find((entry) => entry.locale === locale)?.decimal_separator;
  if (declared) return declared;
  try {
    const parts = new Intl.NumberFormat(locale).formatToParts(1.1);
    return parts.find((part) => part.type === 'decimal')?.value ?? '.';
  } catch {
    return '.';
  }
}

/**
 * The number a person typed, or `null` for an empty field, or `undefined` for text that is not a
 * number under this separator - which the caller shows as such rather than sending.
 */
export function parseDecimal(text: string, separator: string): number | null | undefined {
  const trimmed = text.trim();
  if (trimmed === '') return null;
  // The locale's separator, and the dot a person may type on a keypad regardless of locale:
  // both read as the decimal mark, as long as only one of them appears once.
  const marks = [separator, '.'].filter((mark, index, all) => all.indexOf(mark) === index);
  const escaped = marks.map((mark) => mark.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|');
  const pattern = new RegExp(`^[+-]?(?:\\d+(?:(?:${escaped})\\d*)?|(?:${escaped})\\d+)$`);
  if (!pattern.test(trimmed)) return undefined;
  const normalised = trimmed.replace(new RegExp(escaped), '.');
  const value = Number(normalised);
  return Number.isFinite(value) ? value : undefined;
}

/** A number as the locale writes it, without grouping - what a field is filled with to be edited. */
export function formatDecimal(value: number, locale: string): string {
  try {
    return new Intl.NumberFormat(locale, { useGrouping: false, maximumFractionDigits: 20 }).format(value);
  } catch {
    return String(value);
  }
}
