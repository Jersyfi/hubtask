// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A collection's public form (UC-SVC-13), the parts that hold no state: where its link points and
 * how the screen reads the token back.
 *
 * The token rides in the fragment, `/form#hbt_form_…`: a fragment never reaches a server log or a
 * Referer, and the token is the form's whole credential. Unlike a reset link it is left in the
 * address, since a public form is meant to be opened again.
 */

const PREFIX = 'hbt_form_';

/** The address a collection's public form opens, on this server. */
export const publicFormAddress = (origin: string, token: string) => `${origin.replace(/\/+$/u, '')}/form#${token}`;

/** The token a fragment carries, or empty for anything that is not one. */
export function formTokenOf(fragment: string): string {
  const token = fragment.replace(/^#/u, '').trim();
  return token.startsWith(PREFIX) && /^[A-Za-z0-9_-]+$/u.test(token) ? token : '';
}

/** The words a submission carries, trimmed, with what was left empty left out. */
export function submissionOf(typed: {
  entryType: string; title: string; notes: string; name: string; email: string; answers: Record<string, unknown>;
}): { entry_type: string; title: string; notes?: string; requester_name?: string; requester_email?: string; custom_fields?: Record<string, unknown> } {
  const out: ReturnType<typeof submissionOf> = { entry_type: typed.entryType, title: typed.title.trim() };
  if (typed.notes.trim() !== '') out.notes = typed.notes.trim();
  if (typed.name.trim() !== '') out.requester_name = typed.name.trim();
  if (typed.email.trim() !== '') out.requester_email = typed.email.trim();
  if (Object.keys(typed.answers).length > 0) out.custom_fields = typed.answers;
  return out;
}
