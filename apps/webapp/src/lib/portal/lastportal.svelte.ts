// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The portal an outsider chose last at the top of the frame (PO-2): where they start next time on
 * this browser. A convenience of the device, never the account's (apps/webapp/AGENTS.md); without
 * storage the first portal stands.
 */

const KEY = 'hubtask.portal.last';

function storage(): Storage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

class LastPortal {
  #id = $state<string | undefined>(undefined);

  constructor() {
    try {
      this.#id = storage()?.getItem(KEY) ?? undefined;
    } catch {
      this.#id = undefined;
    }
  }

  get id(): string | undefined {
    return this.#id;
  }

  remember(id: string): void {
    if (this.#id === id) return;
    this.#id = id;
    try {
      storage()?.setItem(KEY, id);
    } catch {
      // A full or refused storage keeps the choice for this visit only.
    }
  }
}

export const lastPortal = new LastPortal();
