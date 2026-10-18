// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's subscriptions for a purchase platform, as the instance area reads and writes
 * them (UC-INS-14).
 *
 * **Never switched off.** Unlike a workspace's webhook, a subscription stays on however long the
 * platform is unreachable; what did not arrive waits as dead letters, which the operator redelivers
 * here. The screen says so rather than letting a quiet subscription look like a working one.
 *
 * **The secret is answered once.** A new subscription returns it, the screen shows it through the
 * one-time secret, and this store never holds it.
 */

import type { PlatformSubscription, PlatformSubscriptionSecret, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';

const SUBSCRIPTIONS = '/admin/subscriptions';
/** The installation's journal records every act on a subscription. */
const JOURNAL = '/admin/journal';

class PlatformSubscriptions {
  #subscriptions = $state<ResourceState<readonly PlatformSubscription[]>>({ status: 'idle' });

  get subscriptions(): readonly PlatformSubscription[] {
    return this.#subscriptions.status === 'ready' ? this.#subscriptions.data : [];
  }

  get subscriptionsState(): ResourceState<readonly PlatformSubscription[]> {
    return this.#subscriptions;
  }

  /** Forgets what was read, with the session it was read in. */
  forget(): void {
    this.#subscriptions = { status: 'idle' };
  }

  openSubscriptions(): () => void {
    return engine.subscribe<readonly PlatformSubscription[]>({ path: SUBSCRIPTIONS }, (next) => {
      this.#subscriptions = next;
    });
  }

  /** Subscribes an address, behind a fresh proof; the answer carries the secret this once. */
  async subscribe(targetUrl: string): Promise<PlatformSubscriptionSecret> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<PlatformSubscriptionSecret>('POST', SUBSCRIPTIONS, { target_url: targetUrl }, {
        idempotencyKey: crypto.randomUUID(),
        stepUpToken,
        invalidates: [SUBSCRIPTIONS, JOURNAL],
      }),
    );
  }

  /** Pauses or resumes one. No proof: it sends less, or what it sent before. */
  async setState(id: string, state: 'ACTIVE' | 'PAUSED'): Promise<void> {
    await engine.mutate('PATCH', `${SUBSCRIPTIONS}/${id}`, { state }, {
      invalidates: [SUBSCRIPTIONS, JOURNAL],
    });
  }

  /** Sends every dead letter again; the answer says how many. */
  async redeliver(id: string): Promise<number> {
    const answer = await engine.mutate<{ readonly queued: number }>('POST', `${SUBSCRIPTIONS}/${id}:redeliver`, undefined, {
      invalidates: [SUBSCRIPTIONS, JOURNAL],
    });
    return answer.queued;
  }

  /** Unsubscribes; the deliveries go with it, the events stay readable. */
  async remove(id: string): Promise<void> {
    await engine.mutate('DELETE', `${SUBSCRIPTIONS}/${id}`, undefined, {
      invalidates: [SUBSCRIPTIONS, JOURNAL],
    });
  }
}

export const platformSubscriptions = new PlatformSubscriptions();
