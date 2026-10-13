// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A person's own choice about AI (`data-protection.md` §4.1): whether there is anything to choose,
 * the workspace's position, and their own *Keep my content out of AI*.
 *
 * **The switch is a consent record, both ways.** Switching it on records a withdrawal for
 * `ai_processing`; switching it off records the person agreeing again. Neither deletes anything:
 * the record of what was allowed when is the point, and the server keeps both.
 *
 * **The notice is told once.** Where the workspace made AI part of everybody's work after the
 * person kept their content out, the frame shows a notice until it is acknowledged on the server -
 * so it is told once on any device, not once per browser.
 */

import type { OwnAiPosition, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/privacy/ai-objection';
const PURPOSE = 'ai_processing';

/**
 * What a change makes stale: the view itself, and the manifest - a person who keeps their content
 * out is offered no AI action, and every AI control is drawn from the manifest.
 */
const TOUCHES = [PATH, '/meta/capabilities'];

class AiObjection {
  #state = $state<ResourceState<OwnAiPosition>>({ status: 'idle' });

  get state(): ResourceState<OwnAiPosition> {
    return this.#state;
  }

  get position(): OwnAiPosition | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /** Whether the frame owes the person the one-time notice. */
  get noticePending(): boolean {
    return this.position?.notice_pending === true;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<OwnAiPosition>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Keeps the person's own content out: a withdrawal of `ai_processing`, recorded. */
  async keepOut(): Promise<unknown> {
    return engine.mutate<unknown>('POST', '/privacy/consents:withdraw', { purpose: PURPOSE }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: TOUCHES,
    });
  }

  /** Lets the person's own content back in: their agreement, recorded. */
  async letIn(): Promise<unknown> {
    return engine.mutate<unknown>('POST', '/privacy/consents:give', { purpose: PURPOSE }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: TOUCHES,
    });
  }

  /** Records that the person read the notice. */
  async acknowledge(): Promise<OwnAiPosition> {
    return engine.mutate<OwnAiPosition>('POST', `${PATH}:noticed`, undefined, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [PATH],
    });
  }
}

export const aiObjection = new AiObjection();
export { PATH as aiObjectionPath };
