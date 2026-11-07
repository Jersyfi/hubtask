// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Which AI answers in a workspace, in one word the head and the column say (user-experience.md
// §7.10) - pure, so that it is tested without a server.

/** What answers: a model the installation offers, the workspace's own, or why nothing does. */
export type AiAnswering = 'offered' | 'own' | 'own_not_allowed' | 'own_missing' | 'withdrawn' | 'none';

/**
 * From the source in force (`/ai-source`) and the own model's configuration. Before the source
 * arrives the own model is assumed, as the screen assumes it. An offered source without its model
 * is the installation having withdrawn the one chosen: nothing answers until somebody chooses
 * again, and that is the one state that broke rather than was never set up.
 */
export function aiAnsweringOf(
  source: { readonly source: string; readonly offered_model_id?: string | null } | undefined,
  provider: { readonly kind: string; readonly processing_allowed: boolean } | undefined,
): AiAnswering {
  const kind = source?.source ?? 'OWN';
  if (kind === 'NONE') return 'none';
  if (kind === 'OFFERED') return source?.offered_model_id ? 'offered' : 'withdrawn';
  if (!provider || provider.kind === 'NOOP') return 'own_missing';
  return provider.processing_allowed ? 'own' : 'own_not_allowed';
}

export const AI_ANSWERING_TONE: Readonly<Record<AiAnswering, 'success' | 'warning' | 'danger' | 'neutral'>> = {
  offered: 'success',
  own: 'success',
  own_not_allowed: 'warning',
  own_missing: 'neutral',
  withdrawn: 'danger',
  none: 'neutral',
};

/** The column's figure on *AI*: only a source that stopped answering, nothing otherwise. */
export function aiColumnFigure(answering: AiAnswering, t: (code: string) => string): string | undefined {
  return answering === 'withdrawn' ? t('app.ai.column_withdrawn') : undefined;
}
