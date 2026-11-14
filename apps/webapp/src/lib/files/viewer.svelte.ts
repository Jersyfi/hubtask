// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The one file viewer (F-1): whatever shows files - an entry, a page, a board, a post, an asset, a
 * portal request - opens it here with every file of its object, and the frame draws it once, over
 * everything.
 */

import type { MediaObject } from '@hubtask/sync-engine';

/** What the files hang on, for "2 of 9 · on TAMA-1" and the details. */
export interface ViewerOwner {
  readonly key?: string;
  readonly title: string;
}

export interface ViewerOpening {
  readonly files: readonly MediaObject[];
  readonly index: number;
  readonly owner?: ViewerOwner;
}

class Viewer {
  opened = $state<ViewerOpening | undefined>(undefined);
  index = $state(0);

  open(opening: ViewerOpening): void {
    if (opening.files.length === 0) return;
    this.opened = opening;
    this.index = Math.min(Math.max(opening.index, 0), opening.files.length - 1);
  }

  close(): void {
    this.opened = undefined;
  }
}

export const viewer = new Viewer();
