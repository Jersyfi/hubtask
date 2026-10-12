// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which password form the sign-in card draws, from what the public sign-in rules say.
 *
 * Where the workspace keeps the password, the first field takes an address - or an address or a
 * sign-in name where a managed account exists (ADR-0074 §3). Where it switched the password off,
 * an address is refused at every door, but a sign-in name keeps its password, because no provider
 * can ever recognise an account without an address (identity.md §4.1): so the card draws a form
 * that asks for the sign-in name alone, and only where a managed account exists - elsewhere the
 * providers are the whole card, as before.
 */
export type PasswordForm = 'email' | 'identifier' | 'name' | 'none';

export function passwordFormOf(rules: { hasPassword: boolean; signInNames: boolean }): PasswordForm {
  if (rules.hasPassword) return rules.signInNames ? 'identifier' : 'email';
  return rules.signInNames ? 'name' : 'none';
}
