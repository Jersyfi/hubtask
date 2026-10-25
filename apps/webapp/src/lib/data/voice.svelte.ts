// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Voice assistants (ADR-0093): a workspace's voice settings, a sentence tried without doing
 * anything, and the Alexa interaction model to paste into Amazon's console.
 */

import { engine } from './engine.ts';

const SETTINGS = '/voice/settings';

export interface VoiceAlias {
  readonly phrase: string;
  readonly collection_id: string;
  readonly label_id?: string | null;
}

export interface VoiceSettings {
  readonly enabled: boolean;
  readonly default_collection_id?: string | null;
  readonly skill_id?: string | null;
  readonly installation_skill?: boolean;
  readonly aliases: readonly VoiceAlias[];
}

export interface VoicePlace {
  readonly collection_id: string;
  readonly label_id?: string | null;
  readonly name: string;
}

export interface VoiceInterpretation {
  readonly things: readonly { readonly title: string; readonly amount?: string | null }[];
  readonly found: boolean;
  readonly default: boolean;
  readonly place?: VoicePlace | null;
  readonly ambiguous: readonly VoicePlace[];
}

export async function readVoiceSettings(): Promise<VoiceSettings | undefined> {
  const read = await engine.refresh<VoiceSettings>({ path: SETTINGS });
  if (read.status === 'failed') throw read.error;
  return read.status === 'ready' ? read.data : undefined;
}

export async function saveVoiceSettings(settings: VoiceSettings): Promise<VoiceSettings> {
  const { installation_skill: _ignored, ...body } = settings;
  return engine.mutate<VoiceSettings>('PUT', SETTINGS, body, { idempotencyKey: crypto.randomUUID() });
}

export async function tryVoiceSentence(sentence: string, locale: string): Promise<VoiceInterpretation> {
  return engine.mutate<VoiceInterpretation>('POST', '/voice:try', { sentence, locale });
}

export async function readAlexaModel(locale: string): Promise<unknown> {
  const read = await engine.refresh<unknown>({ path: `/voice/alexa/model?locale=${encodeURIComponent(locale)}` });
  if (read.status === 'failed') throw read.error;
  return read.status === 'ready' ? read.data : undefined;
}

/** The scopes the skill asks for at linking: reading the places and entries, writing entries. */
export const ALEXA_SCOPES = 'containers:read items:read items:write';

/** Alexa's three regional addresses an authorization returns to, for a vendor ID. */
export function alexaRedirects(vendorId: string): string[] {
  const vendor = vendorId.trim();
  if (!vendor) return [];
  return ['https://pitangui.amazon.com', 'https://layla.amazon.com', 'https://alexa.amazon.co.jp'].map(
    (base) => `${base}/api/skill/link/${vendor}`,
  );
}
