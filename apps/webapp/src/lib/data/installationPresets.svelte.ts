// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's presets (UC-INS-18), as *Installation → Defaults* reads and writes them: the
 * list, creating one, changing one - its name, its settings again from one of Hubtask's, whether it
 * is the default, the default's locks - and withdrawing one.
 *
 * Every write goes through the elevated hour's step-up, as the plans' do, and carries the version
 * it read (`If-Match`): somebody who changed the preset first is answered `version_conflict`, never
 * overwritten. A write makes stale what offers the presets too - `/presets`, where this operator is
 * also somebody in a workspace - and the journal.
 */

import type { InstallationPreset, InstallationPresetCreate, InstallationPresetUpdate, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';
import { stepUp } from './stepup.svelte.ts';

const PATH = '/admin/presets';
const STALE = [PATH, '/presets', '/admin/journal'];

class InstallationPresets {
  #list = $state<ResourceState<{ readonly data: readonly InstallationPreset[] }>>({ status: 'idle' });

  get state(): ResourceState<{ readonly data: readonly InstallationPreset[] }> {
    return this.#list;
  }

  /** The presets, by name; empty until read. */
  get all(): readonly InstallationPreset[] {
    return this.#list.status === 'ready' ? this.#list.data.data : [];
  }

  /** The default for new workspaces, where one is chosen. */
  get default(): InstallationPreset | undefined {
    return this.all.find((preset) => preset.is_default);
  }

  /** Starts the listing. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<{ readonly data: readonly InstallationPreset[] }>({ path: PATH }, (next) => {
      this.#list = next;
    });
  }

  async create(body: InstallationPresetCreate): Promise<InstallationPreset> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<InstallationPreset>('POST', PATH, body, {
        idempotencyKey: crypto.randomUUID(),
        stepUpToken,
        invalidates: STALE,
      }),
    );
  }

  /** Changes what the body names, on the version read; a field left out stays as it is. */
  async change(id: string, body: InstallationPresetUpdate, version: number): Promise<InstallationPreset> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<InstallationPreset>('PATCH', `${PATH}/${id}`, body, {
        ifMatch: etagFor(version),
        stepUpToken,
        invalidates: STALE,
      }),
    );
  }

  /** Withdraws it: offered to no workspace any more, what was made from it keeps its settings. */
  async withdraw(id: string, version: number): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('DELETE', `${PATH}/${id}`, undefined, {
        ifMatch: etagFor(version),
        stepUpToken,
        invalidates: STALE,
      }),
    );
  }
}

export const installationPresets = new InstallationPresets();
