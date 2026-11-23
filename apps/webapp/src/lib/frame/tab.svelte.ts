// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace the tab's title names while somebody is signed in, told by the frame, which reads
 * the workspace; `App.svelte` writes the title from it (`tabtitle.ts`). The frame is a chunk of its
 * own, so the first screen carries this holder and not the workspace's store.
 */

class Tab {
  #workspace = $state<string | undefined>(undefined);

  get workspace(): string | undefined {
    return this.#workspace;
  }

  /** Names the workspace for as long as the caller's effect lives; the cleanup clears it. */
  name(workspace: string | undefined): () => void {
    this.#workspace = workspace;
    return () => {
      if (this.#workspace === workspace) this.#workspace = undefined;
    };
  }
}

export const tab = new Tab();
