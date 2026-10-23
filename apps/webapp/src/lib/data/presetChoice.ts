// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How a preset is chosen (UC-WRK-30): by one value - Hubtask's key, or a saved preset's id. The two
 * never meet, a key being a word and an id a uuid; a request names the first as `preset`, the
 * second as `preset_id`.
 */

import type { Preset } from '@hubtask/sync-engine';

/** Hubtask's four, by key. */
export const BUILT_IN = ['LIST', 'TASKS', 'PROJECT', 'FLOW'] as const;
export type BuiltInKey = (typeof BUILT_IN)[number];

export const isBuiltIn = (value: string | null | undefined): value is BuiltInKey => (BUILT_IN as readonly string[]).includes(value ?? '');

/** The value a preset is chosen by. */
export const presetValue = (preset: Preset): string => preset.key ?? preset.id ?? '';

/** What a request names the chosen preset by; nothing where none is chosen. */
export function presetChoice(value: string | null | undefined): { preset?: BuiltInKey; preset_id?: string } {
  if (!value) return {};
  return isBuiltIn(value) ? { preset: value } : { preset_id: value };
}
