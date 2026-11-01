// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The suite's modules at the workspace or one hub (UC-INS-21): which are on, which level decided
 * it and which locked it, and the switch. The answer of a switch is every module at the place, so
 * the read is invalidated rather than patched, and the capabilities with it - `modules_on` is what
 * every other screen asks.
 */

import type { ModuleList, ModulesOn, ModuleState, ModuleSwitch, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export { asksFirst, isSetHere, modulesHere, moduleCodes, unavailableBecause, type ModulePlace } from './modules.ts';

const PATH = '/tenant/modules';
const CAPABILITIES = '/meta/capabilities';
// Every place's own set, which a switch at the workspace or a hub can change.
const PLACES = '/containers/*/modules';

const pathAt = (hubId?: string) => (hubId ? `${PATH}?hub_id=${encodeURIComponent(hubId)}` : PATH);

class Modules {
  /**
   * Reads the modules at the workspace or one hub; the answer goes to the caller, because the
   * administration's section and a hub's settings each draw their own place. **From `untrack`**.
   */
  at(hubId: string | undefined, onNext: (state: ResourceState<ModuleList>) => void): () => void {
    return engine.subscribe<ModuleList>({ path: pathAt(hubId) }, onNext);
  }

  /**
   * Reads the modules on at a hub, or at the hub a collection is in, for anybody who may read the
   * place: what its screens are drawn for. **From `untrack`**.
   */
  here(containerId: string, onNext: (state: ResourceState<ModulesOn>) => void): () => void {
    return engine.subscribe<ModulesOn>({ path: `/containers/${encodeURIComponent(containerId)}/modules` }, onNext);
  }

  /** Switches one module on, off, or back to the level above. */
  async switch(module: ModuleState['module'], to: ModuleSwitch['switch'], hubId?: string): Promise<ModuleList> {
    const body: ModuleSwitch = hubId ? { switch: to, hub_id: hubId } : { switch: to };
    return engine.mutate<ModuleList>('PUT', `${PATH}/${encodeURIComponent(module)}`, body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [PATH, CAPABILITIES, PLACES],
    });
  }
}

export const modules = new Modules();
