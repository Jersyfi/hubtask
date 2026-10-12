// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The consent register for AI (UC-PRV-05 check 12): who keeps their own content out, since when,
 * and whether they or an administrator recorded it.
 *
 * **The one place the list appears.** Read by the screen that answers data subject requests, for
 * the people who manage members; a reader the server refuses has no register, not an empty one.
 */

import type { ConsentRegisterEntry, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/privacy/consents?purpose=ai_processing';

interface Register {
  readonly data?: readonly ConsentRegisterEntry[];
}

class ConsentRegister {
  #state = $state<ResourceState<Register>>({ status: 'idle' });

  get state(): ResourceState<Register> {
    return this.#state;
  }

  /** The people, or none while unread or refused. */
  get entries(): readonly ConsentRegisterEntry[] {
    return this.#state.status === 'ready' ? (this.#state.data.data ?? []) : [];
  }

  /** Whether the register was answered at all - refused is absent, not empty. */
  get isReadable(): boolean {
    return this.#state.status === 'ready';
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<Register>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }
}

export const consentRegister = new ConsentRegister();
