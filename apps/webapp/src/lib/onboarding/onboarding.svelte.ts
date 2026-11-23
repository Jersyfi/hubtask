// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The onboarding coordinator (user-experience.md §3.1): what the starter tells the tour, so the
 * two never open together. The first start's offer is the workspace's; whether its dialog is open
 * is this tab's.
 */

import { mayReadWorkspace } from '../data/capability.svelte.ts';
import { workspace } from '../data/workspace.svelte.ts';
import { tourMayStart } from './order.ts';

class Coordinator {
  #starterOpen = $state(false);

  /** The starter's dialog says whether it is open. */
  starterOpen(open: boolean): void {
    this.#starterOpen = open;
  }

  /** Whether the starter's dialog is open in this tab: a tour already walking steps aside. */
  get starterIsOpen(): boolean {
    return this.#starterOpen;
  }

  /** Whether the base tour may start now; a workspace that cannot be read offers no starter. */
  get tourMayStart(): boolean {
    const read = workspace.state;
    // A reader who may not read the workspace is offered no starter and does not ask for it.
    const starterOffered =
      read.status === 'ready' ? read.data.starter_offered === true : read.status === 'failed' || mayReadWorkspace() === false ? false : undefined;
    return tourMayStart({ starterOffered, starterOpen: this.#starterOpen });
  }
}

export const onboarding = new Coordinator();
