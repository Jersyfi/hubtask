// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The suite's modules at the workspace or one hub (UC-INS-21): which are on, which level decided
 * it and which locked it, and the switch. The answer of a switch is every module at the place, so
 * the read is invalidated rather than patched, and the capabilities with it - `modules_on` is what
 * every other screen asks.
 */

import type { ModuleList, ModuleState, ModuleSwitch, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export { asksFirst, isSetHere, moduleCodes, unavailableBecause, type ModulePlace } from './modules.ts';

const PATH = '/tenant/modules';
const CAPABILITIES = '/meta/capabilities';

const pathAt = (hubId?: string) => (hubId ? `${PATH}?hub_id=${encodeURIComponent(hubId)}` : PATH);

class Modules {
  #state = $state<ResourceState<ModuleList>>({ status: 'idle' });

  get state(): ResourceState<ModuleList> {
    return this.#state;
  }

  /** The modules, or none while they are unread or refused. */
  get all(): readonly ModuleState[] {
    return this.#state.status === 'ready' ? this.#state.data.modules : [];
  }

  /** Starts the read for the workspace or one hub. **From `untrack`**. */
  open(hubId?: string): () => void {
    return engine.subscribe<ModuleList>({ path: pathAt(hubId) }, (next) => {
      this.#state = next;
    });
  }

  /** Switches one module on, off, or back to the level above. */
  async switch(module: ModuleState['module'], to: ModuleSwitch['switch'], hubId?: string): Promise<ModuleList> {
    const body: ModuleSwitch = hubId ? { switch: to, hub_id: hubId } : { switch: to };
    return engine.mutate<ModuleList>('PUT', `${PATH}/${encodeURIComponent(module)}`, body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [PATH, CAPABILITIES],
    });
  }
}

export const modules = new Modules();
