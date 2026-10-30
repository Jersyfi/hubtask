// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * *View as* (UC-ID-29): whether this session shows the app as another person, read only and
 * online only. Read once the session is up; the frame draws its banner from it, and the stream and
 * the replica are not started while it runs - the server refuses a copy of another person's work.
 *
 * Starting and ending reload the page whole: every screen read as one person, and a cache that
 * kept the other's answers would show them for a moment as the new one's.
 */

import type { ResourceState, ViewAs } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export const VIEW_AS_PATH = '/auth/sessions:view-as';

class ViewAsState {
  #state = $state<ResourceState<ViewAs>>({ status: 'idle' });

  /** Whether the answer arrived - or could not, which is no view: the server judges each request. */
  get settled(): boolean {
    return this.#state.status === 'ready' || this.#state.status === 'failed';
  }

  /** The running view, or undefined. */
  get running(): ViewAs | undefined {
    return this.#state.status === 'ready' && this.#state.data.active ? this.#state.data : undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<ViewAs>({ path: VIEW_AS_PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Shows the app as the person; the page then starts over as them. */
  async start(accountId: string): Promise<void> {
    await engine.mutate<ViewAs>('POST', VIEW_AS_PATH, { account_id: accountId });
    await restart();
  }

  /** Ends the view; the page starts over as the session's own person. */
  async end(): Promise<void> {
    await engine.mutate('DELETE', VIEW_AS_PATH, undefined);
    await restart();
  }
}

async function restart(): Promise<void> {
  await engine.reset();
  globalThis.location?.assign('/');
}

export const viewAs = new ViewAsState();
