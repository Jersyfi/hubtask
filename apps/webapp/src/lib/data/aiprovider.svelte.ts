// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's AI provider: the one read, the `PUT` that configures it, the `DELETE` that
 * takes it away (ADR-0049).
 *
 * **The key goes one way.** It is sent on a `PUT` and sealed there; the read answers `has_api_key`
 * and nothing else about it, so this module never holds one after the request leaves. An absent
 * `api_key` on a later `PUT` keeps the sealed key and an empty one clears it - that is the
 * contract's own rule, and the screen sends the field only when somebody typed a key.
 *
 * **Which AI answers is a second resource** (ADR-0072 §2): `/ai-source` says what the installation
 * allows, what is offered by name, processor and where, and what the workspace chose. Choosing an
 * offered model there is the workspace's consent to it.
 *
 * **Not configured is not a failure.** The read answers `404 ai.not_configured` where a workspace
 * has never set one, and that is an empty screen with a form on it. The state is handed out whole
 * so the screen can tell the two apart.
 *
 * **Consent is a field of the configuration**, `processing_allowed`, false unless it is said -
 * configuring a provider and consenting to send the workspace's content to it are two decisions
 * (`ai-first.md` §2), and this module keeps them two by sending exactly what the form holds.
 */

import type {
  AiPersonOptOutChoice,
  AiProvider,
  AiProviderConfiguration,
  AiSource,
  AiSourceChoice,
  OwnAiBudget,
  ResourceState,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/ai-provider';
const SOURCE = '/ai-source';
const BUDGET = '/ai-budget';
const QUOTAS = '/quotas';

/**
 * What a change to the provider makes stale: the read itself, and the manifest - `ai_suggestions`
 * and `semantic_search` are answered from this configuration, and every AI control in the
 * product is rendered from the manifest.
 */
const TOUCHES = [PATH, SOURCE, '/meta/capabilities', '/meta/health'];

class AiProviderStore {
  #state = $state<ResourceState<AiProvider>>({ status: 'idle' });
  #source = $state<ResourceState<AiSource>>({ status: 'idle' });

  /** Which AI answers, what is allowed and what is offered. */
  get sourceState(): ResourceState<AiSource> {
    return this.#source;
  }

  get source(): AiSource | undefined {
    return this.#source.status === 'ready' ? this.#source.data : undefined;
  }

  get state(): ResourceState<AiProvider> {
    return this.#state;
  }

  /** The configuration, where there is one. */
  get provider(): AiProvider | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    const stopProvider = engine.subscribe<AiProvider>({ path: PATH }, (next) => {
      this.#state = next;
    });
    const stopSource = engine.subscribe<AiSource>({ path: SOURCE }, (next) => {
      this.#source = next;
    });
    return () => {
      stopProvider();
      stopSource();
    };
  }

  /**
   * Chooses which AI answers. Choosing an offered model is the consent to send this workspace's
   * content to it; its own model or none withdraws that consent. `sources` narrows what the
   * workspace allows itself.
   */
  async choose(body: AiSourceChoice): Promise<AiSource> {
    return engine.mutate<AiSource>('PUT', SOURCE, body, { invalidates: TOUCHES });
  }

  /**
   * Sets how many tokens a day the workspace's own model may spend; null is no budget. What it
   * changes is the AI rows of the quota standing, which every reader of this screen reads.
   */
  async setOwnBudget(body: OwnAiBudget): Promise<void> {
    await engine.mutate('PUT', BUDGET, body, { invalidates: [QUOTAS] });
  }

  /**
   * Whether each person may keep their own content out of AI, or AI is part of everybody's work
   * on the basis named. It moves what each person's own view says, so that view is stale too.
   */
  async setPersonOptOut(body: AiPersonOptOutChoice): Promise<AiSource> {
    return engine.mutate<AiSource>('PUT', '/ai-person-opt-out', body, {
      invalidates: [...TOUCHES, '/privacy/ai-objection'],
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
export { PATH as aiProviderPath, SOURCE as aiSourcePath };
