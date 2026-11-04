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

  /** Whether the base tour may start now. */
  get tourMayStart(): boolean {
    return tourMayStart({ starterOffered: workspace.workspace?.starter_offered === true, starterOpen: this.#starterOpen });
  }
}

export const onboarding = new Coordinator();
