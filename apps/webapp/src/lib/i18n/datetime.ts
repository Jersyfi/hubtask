// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A moment, written the way the reader's locale writes moments.
 *
 * Not part of `format.ts`, and that is deliberate. The ICU subset there implements simple
 * arguments, `plural` and `select` and **refuses what it does not implement by name** — a `date`
 * argument type included — so a catalogue message cannot format a date and `catalogue.test.ts`
 * would turn red if one tried. A date reaches a message as text that was formatted here first,
 * which keeps the renderer's refusal honest.
 *
 * The **time zone is the device's**, which is the platform's default and all this client can
 * honestly claim today. A person's own zone is a property of their account and belongs with the
 * time features that read it (F3); asserting one here would be inventing an answer.
 */

/** The reader's rendering of an instant, or the raw text when it is not one. */
export function formatDateTime(iso: string, locale: string): string {
  const at = Date.parse(iso);
  // Never a blank and never `Invalid Date`: a value this cannot read is shown as it arrived, so
  // the reader sees something they can quote rather than nothing.
  if (Number.isNaN(at)) return iso;
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(at);
}
