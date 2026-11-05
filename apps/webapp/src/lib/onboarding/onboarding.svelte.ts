// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The onboarding coordinator (user-experience.md §3.1): what the starter tells the tour, so the
 * two never open together. The first start's offer is the workspace's; whether its dialog is open
 * is this tab's.
 */

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
    const starterOffered = read.status === 'ready' ? read.data.starter_offered === true : read.status === 'failed' ? false : undefined;
    return tourMayStart({ starterOffered, starterOpen: this.#starterOpen });
  }
}

export const onboarding = new Coordinator();
