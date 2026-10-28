// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The settings catalogue (`GET /settings/catalogue`): every declared setting, the same for every
 * workspace and changed only by a release. Read while something draws from it - the palette's
 * settings search (configuration/ui.md A7).
 */

import type { ResourceState, SettingsCatalogue } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/settings/catalogue';

class Catalogue {
  /** Reads the catalogue; the answer goes to the caller. **From `untrack`**. */
  read(onNext: (state: ResourceState<SettingsCatalogue>) => void): () => void {
    return engine.subscribe<SettingsCatalogue>({ path: PATH }, onNext);
  }
}

export const settingsCatalogue = new Catalogue();
