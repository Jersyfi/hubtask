// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Hubtask's presets - *List*, *Tasks*, *Project*, *Flow* - as this installation offers them
 * (`GET /presets`, UC-WRK-23 check 1). Four rows, each with its eight settings, read where a
 * screen offers a preset: the create dialog and the *Structure* panel. A screen that offers them
 * opens the read and lets it go.
 */

import type { Preset, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/presets';

class Presets {
  #state = $state<ResourceState<readonly Preset[]>>({ status: 'idle' });

  get all(): readonly Preset[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  /** The preset by its key, where the read has answered. */
  find(key: string | null | undefined): Preset | undefined {
    return key ? this.all.find((preset) => preset.key === key) : undefined;
  }

  /** Starts the read; call from `untrack` (the listener writes what a tracking effect would read). */
  open(): () => void {
    return engine.subscribe<readonly Preset[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }
}

export const presets = new Presets();
