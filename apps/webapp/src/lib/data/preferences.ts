// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What this client decides about somebody's own preferences. Pure, so each decision is tested.
 *
 * **Clearing a preference is not setting it to nothing.** An empty value means "the workspace
 * default applies again" — the second link of the chain request → account → tenant → installation
 * (`i18n-l10n.md` §2) — and the screen says so in those words rather than showing a blank field
 * and letting somebody guess what it will do.
 *
 * **A default is not a choice.** `is_default: true` says nobody has written that pair, and the form
 * shows the value *and* that nobody chose it. A form that drew a default as a decision would be
 * lying about who made it, which is the contract's own reasoning for carrying the flag at all.
 *
 * **Nothing is compiled in.** The locales come from `supported_locales`, the categories from
 * `notification_categories`, the channels from `notification_channels`. A category this version has
 * no phrase for still renders, because `t` humanises a code it has never met.
 */

import type { Capabilities, NotificationPreference } from '@hubtask/sync-engine';

/** The three the contract's enum declares, plus the empty choice that clears the preference. */
export const WEEK_STARTS = ['MONDAY', 'SUNDAY', 'SATURDAY'] as const;

/**
 * What a `PATCH` sends for a field somebody cleared.
 *
 * `null` rather than `''`: the schema types the three as `[string, "null"]`, and a null is the
 * unambiguous way to say "no preference" — an empty string is a value that happens to be empty, and
 * this client has met a server that reads the two differently often enough to say which it means.
 */
export function clearedOr(value: string): string | null {
  return value.trim() === '' ? null : value.trim();
}

/**
 * The zones this platform knows, or an empty list where it will not say.
 *
 * `Intl.supportedValuesOf` is the only honest source: a list compiled into a client is a list that
 * is wrong the next time a country moves its clocks, and every browser this project supports
 * (ADR-0044) has had it for years. Where it is missing, the field takes a typed zone and the server
 * refuses an unknown one by name — which is a worse offer, not a broken one.
 */
export function knownZones(): readonly string[] {
  const supported = (Intl as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf;
  if (typeof supported !== 'function') return [];
  try {
    return supported('timeZone');
  } catch {
    return [];
  }
}

/** The locales this installation serves, in the order it reports them. */
export function localesOf(manifest: Capabilities | undefined): readonly { locale: string; direction: string }[] {
  return (manifest?.supported_locales ?? []).flatMap((entry) =>
    typeof entry?.locale === 'string'
      ? // A locale that states no direction is left to right, which is the honest default rather
        // than a guess: the manifest would say so if it were not.
        [{ locale: entry.locale, direction: entry.direction === 'rtl' ? 'rtl' : 'ltr' }]
      : [],
  );
}

/** The categories this installation tells people about, in the order it reports them. */
export function categoriesOf(manifest: Capabilities | undefined): readonly string[] {
  return (manifest?.notification_categories ?? []) as readonly string[];
}

/** The channels it delivers on. `EMAIL` today; a client tolerates a value it does not know. */
export function channelsOf(manifest: Capabilities | undefined): readonly string[] {
  const declared = (manifest?.notification_channels ?? []) as readonly string[];
  return declared.length > 0 ? declared : ['EMAIL'];
}

/** One row of the form, found among what the server answered. */
export function preferenceFor(
  rows: readonly NotificationPreference[],
  category: string,
  channel: string,
): NotificationPreference | undefined {
  return rows.find((row) => row.category === category && row.channel === channel);
}

/**
 * The category no preference switches off.
 *
 * The manifest says so in its own words — "`INVITATION` is in the list and is the one no preference
 * switches off" — and a switch that could be moved and would not be obeyed is worse than one that
 * says why it cannot.
 */
export function isAlwaysOn(category: string): boolean {
  return category === 'INVITATION';
}
