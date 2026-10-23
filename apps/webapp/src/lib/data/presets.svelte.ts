// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The presets a hub or collection can start from (`GET /presets`, UC-WRK-23 check 1, UC-WRK-30):
 * Hubtask's four - *List*, *Tasks*, *Project*, *Flow* - and the saved ones, the workspace's and the
 * reader's own, each with its origin and its eight settings. Read where a screen offers a preset:
 * the create dialog, the *Structure* panel and the workspace's *Presets* view. A screen that offers
 * them opens the read and lets it go.
 *
 * A preset is chosen by one value, Hubtask's key or a saved preset's id (`presetChoice.ts`).
 */

import type { Preset, PresetCreate, PresetUpdate, ResourceState } from '@hubtask/sync-engine';

import { t } from '../i18n/i18n.svelte.ts';
import { engine } from './engine.ts';
import { etagFor } from './etag.ts';
import { isBuiltIn, presetValue } from './presetChoice.ts';

export { BUILT_IN, isBuiltIn, presetChoice, presetValue, type BuiltInKey } from './presetChoice.ts';

const PATH = '/presets';

/** A preset's name: Hubtask's translated, a saved one's as a person gave it. */
export const presetName = (preset: Preset): string => (preset.key ? t(`app.structure.preset.${preset.key}`) : (preset.name ?? ''));

/** Where a preset comes from, in words (UC-WRK-30 check 2). */
export const presetOrigin = (preset: Preset): string => t(`app.structure.preset_origin.${preset.origin}`);

class Presets {
  #state = $state<ResourceState<readonly Preset[]>>({ status: 'idle' });

  get all(): readonly Preset[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  /** The workspace's saved presets. */
  get workspace(): readonly Preset[] {
    return this.all.filter((preset) => preset.origin === 'WORKSPACE');
  }

  /** The preset by the value it is chosen by, where the read has answered. */
  find(value: string | null | undefined): Preset | undefined {
    return value ? this.all.find((preset) => presetValue(preset) === value) : undefined;
  }

  /** The name of the preset a value names: a withdrawn one, or one not yet read, has none. */
  nameOf(value: string | null | undefined): string | undefined {
    if (!value) return undefined;
    if (isBuiltIn(value)) return t(`app.structure.preset.${value}`);
    const found = this.find(value);
    return found ? presetName(found) : undefined;
  }

  /** Starts the read; call from `untrack` (the listener writes what a tracking effect would read). */
  open(): () => void {
    return engine.subscribe<readonly Preset[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Saves a hub's or collection's structure in force as a preset (UC-WRK-30 check 1). */
  async save(body: PresetCreate): Promise<Preset> {
    return engine.mutate<Preset>('POST', PATH, body, { invalidates: [PATH] });
  }

  /** Renames or changes a saved preset; what was made from it keeps its settings. */
  async change(id: string, body: PresetUpdate, version: number): Promise<Preset> {
    return engine.mutate<Preset>('PATCH', `${PATH}/${id}`, body, { ifMatch: etagFor(version), invalidates: [PATH] });
  }

  /** Withdraws a saved preset. */
  async withdraw(id: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', `${PATH}/${id}`, undefined, { ifMatch: etagFor(version), invalidates: [PATH] });
  }
}

export const presets = new Presets();
