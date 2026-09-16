// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's AI provider: the one read, the `PUT` that configures it, the `DELETE` that
 * takes it away (J-02, ADR-0049, F5-05).
 *
 * **The key goes one way.** It is sent on a `PUT` and sealed there; the read answers `has_api_key`
 * and nothing else about it, so this module never holds one after the request leaves. An empty
 * string on a later `PUT` keeps the sealed key - that is the contract's own rule, and the screen
 * says so beside the field rather than inventing a rule about which case it is in.
 *
 * **Not configured is not a failure.** The read answers `404 ai.not_configured` where a workspace
 * has never set one, and that is an empty screen with a form on it. The state is handed out whole
 * so the screen can tell the two apart.
 *
 * **Consent is a field of the configuration**, `processing_allowed`, false unless it is said -
 * configuring a provider and consenting to send the workspace's content to it are two decisions
 * (`ai-first.md` §2), and this module keeps them two by sending exactly what the form holds.
 */

import type { AiProvider, AiProviderConfiguration, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/ai-provider';

/**
 * What a change to the provider makes stale: the read itself, and the manifest - `ai_suggestions`
 * and `semantic_search` are answered from this configuration, and every AI control in the
 * product is rendered from the manifest.
 */
const TOUCHES = [PATH, '/meta/capabilities', '/meta/health'];

class AiProviderStore {
  #state = $state<ResourceState<AiProvider>>({ status: 'idle' });

  get state(): ResourceState<AiProvider> {
    return this.#state;
  }

  /** The configuration, where there is one. */
  get provider(): AiProvider | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<AiProvider>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Sets it, whole. `api_key` absent keeps the sealed one; empty clears it; a value replaces it. */
  async configure(body: AiProviderConfiguration): Promise<AiProvider> {
    return engine.mutate<AiProvider>('PUT', PATH, body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: TOUCHES,
    });
  }

  /** Takes it away: the configuration and its sealed key. Nothing already accepted is undone. */
  async remove(): Promise<void> {
    await engine.mutate('DELETE', PATH, undefined, { idempotencyKey: crypto.randomUUID(), invalidates: TOUCHES });
  }
}

export const aiProvider = new AiProviderStore();
export { PATH as aiProviderPath };
