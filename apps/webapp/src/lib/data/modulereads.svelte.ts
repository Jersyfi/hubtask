// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A store's read of a module's resource, asked only while the manifest does not say the module is
 * off: read while it is off, the server refuses it with `module.off`, which is a failure in the
 * reader's console rather than the quiet absence it is. Before the manifest arrives the read is
 * asked, since a read withheld on a guess is a list taken away; a module switched on later starts
 * the read, one switched off stops it.
 */
import type { ResourceRequest, ResourceState } from '@hubtask/sync-engine';

import { manifest } from './capabilities.svelte.ts';
import { engine } from './engine.ts';

/** Whether a read of the module's may be asked: not while the manifest says it is off. */
export function mayAskOf(module: string, modulesOn: readonly string[] | undefined): boolean {
  return modulesOn === undefined || modulesOn.includes(module);
}

/** Subscribes while the module may be asked; the closer ends both the watch and the read. */
export function subscribeWhileOn<T>(module: string, request: ResourceRequest, onNext: (state: ResourceState<T>) => void): () => void {
  return $effect.root(() => {
    $effect(() => {
      if (!mayAskOf(module, manifest.value?.modules_on)) return;
      return engine.subscribe<T>(request, onNext);
    });
  });
}
