// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** How a token reads on a screen: its scopes in words and its standing, one answer for every list. */

import { scopeWordCode } from '../data/agents.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';

/**
 * A scope in the consent screen's words, its identifier where this build has none: the renderer
 * would humanise an unknown code into a sentence nobody wrote (ConsentView's `wordFor`).
 */
export const scopeWord = (scope: string): string => (messages.has(scopeWordCode(scope)) ? t(scopeWordCode(scope)) : scope);

/**
 * Which of the three standings a token has. "It ran out" and "somebody pulled it" are different
 * answers, and the contract keeps them in separate columns so that a screen can say which.
 */
export function standingOf(token: { revoked_at?: string | null; expires_at: string }): 'revoked' | 'expired' | 'live' {
  if (token.revoked_at) return 'revoked';
  if (new Date(token.expires_at).getTime() <= Date.now()) return 'expired';
  return 'live';
}
