// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The one model form's values, for a workspace's own model and for a model the installation offers
 * every workspace: which kind, where it is reached, its two models, the key and where it processes
 * content. What only one level asks - the consent at the workspace, the name and processor at the
 * installation - stays with the screen that asks it.
 */

import type { AiJurisdiction, AiProviderKind } from '@hubtask/sync-engine';

export interface ModelDraft {
  kind: AiProviderKind | '';
  baseUrl: string;
  completionModel: string;
  embeddingModel: string;
  /** Typed or empty: empty keeps the sealed one, which nothing can read back. */
  apiKey: string;
  jurisdiction: AiJurisdiction | '';
}

/** Where a model processes content, from the strictest declaration down. */
export const JURISDICTIONS: readonly AiJurisdiction[] = ['SELF_HOSTED', 'EEA', 'ADEQUACY', 'THIRD_COUNTRY'];

const RANK: Record<AiJurisdiction, number> = { THIRD_COUNTRY: 0, ADEQUACY: 1, EEA: 2, SELF_HOSTED: 3 };
const MINIMUM_RANK: Record<string, number> = { ANY: 0, ADEQUACY: 1, EEA: 2, SELF_HOSTED: 3 };

/**
 * The declarations a minimum admits (UC-AI-06 check 4): a looser one is not offered, because the
 * server refuses it (P-05). A stored declaration below the minimum stays, so the form can show it.
 */
export function admittedJurisdictions(minimum: string, stored: AiJurisdiction | '' | undefined): AiJurisdiction[] {
  const floor = MINIMUM_RANK[minimum] ?? 0;
  return JURISDICTIONS.filter((each) => RANK[each] >= floor || each === stored);
}

/** An empty form. */
export function emptyModel(kind: AiProviderKind | '', jurisdiction: AiJurisdiction | ''): ModelDraft {
  return { kind, baseUrl: '', completionModel: '', embeddingModel: '', apiKey: '', jurisdiction };
}

/** The form opened on a stored model. The key stays empty, because nothing can read it. */
export function modelOf(stored: {
  kind: AiProviderKind;
  base_url?: string | null;
  completion_model?: string | null;
  embedding_model?: string | null;
  jurisdiction: AiJurisdiction;
}): ModelDraft {
  return {
    kind: stored.kind,
    baseUrl: stored.base_url ?? '',
    completionModel: stored.completion_model ?? '',
    embeddingModel: stored.embedding_model ?? '',
    apiKey: '',
    jurisdiction: stored.jurisdiction,
  };
}

/**
 * The fields both levels send. Empty text is left out rather than sent empty; the key above all,
 * because "clear the key" is a different intent from "I typed nothing", and absent keeps the sealed
 * one. Called once the kind and the declaration are chosen.
 */
export function modelBody(draft: ModelDraft): {
  kind: AiProviderKind;
  base_url?: string;
  completion_model?: string;
  embedding_model?: string;
  api_key?: string;
  jurisdiction: AiJurisdiction;
} {
  return {
    kind: draft.kind as AiProviderKind,
    ...(draft.baseUrl.trim() ? { base_url: draft.baseUrl.trim() } : {}),
    ...(draft.completionModel.trim() ? { completion_model: draft.completionModel.trim() } : {}),
    ...(draft.embeddingModel.trim() ? { embedding_model: draft.embeddingModel.trim() } : {}),
    ...(draft.apiKey ? { api_key: draft.apiKey } : {}),
    jurisdiction: draft.jurisdiction as AiJurisdiction,
  };
}
