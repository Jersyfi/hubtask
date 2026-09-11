// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * An external system's standing request to be told what happens here (`automation.md` §5).
 *
 * **The secret exists once.** `POST /integrations/webhooks` and `:rotate-secret` are the only two
 * answers that carry it, and nothing here holds one: the value is returned to the caller, the
 * screen shows it in `OneTimeSecret`, and this module's own state never sees it. A store that kept
 * it would be a second place a credential lives, and it would outlive the panel that shows it.
 *
 * **The target URL is not validated here.** What is reachable is the installation's answer: the
 * guarded client refuses a private range or the cloud metadata address unless private networks
 * were deliberately released (T-07), and a client that decided in advance would refuse a target
 * this installation permits. So the refusal is rendered rather than predicted.
 *
 * **A replay is not a resend.** It carries the event's own identifier, which is what makes it safe
 * for a subscriber that deduplicates, and the attempt counter continues rather than resetting so
 * that the delivery log stays a true account of how many times this event was sent.
 *
 * **`filter` is not here at all.** `integration.RefuseFilter` still refuses a non-empty one, and a
 * field that is accepted and ignored would be a subscriber receiving events they asked not to.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const WEBHOOKS = '/integrations/webhooks';

/** What a subscription is in, and what it means. `DISABLED` is a conclusion, not a setting. */
export type SubscriptionState = 'ACTIVE' | 'PAUSED' | 'DISABLED';

export interface Subscription {
  readonly id: string;
  readonly target_url: string;
  readonly event_types: readonly string[];
  readonly state: SubscriptionState;
  /** Consecutive failures, reset by the first success. What the auto-disable counts against. */
  readonly failure_count: number;
  /** A message code for the last failure, never a response body from the target. */
  readonly last_error?: string | null;
  readonly created_at: string;
  readonly version: number;
}

/** A subscription answered together with the secret. The one shape that carries one. */
export interface SubscriptionWithSecret extends Subscription {
  readonly secret: string;
}

/** One attempt at one event against one subscription. */
export interface Delivery {
  readonly id: string;
  readonly subscription_id: string;
  readonly event_id: string;
  /** Which attempt this is, from one. It carries on across a replay rather than resetting. */
  readonly attempt: number;
  readonly status: 'PENDING' | 'SUCCEEDED' | 'FAILED' | 'DEAD_LETTER';
  readonly response_status?: number | null;
  readonly error_code?: string | null;
  readonly next_attempt_at?: string | null;
  readonly created_at: string;
}

interface DeliveryPage {
  readonly data?: readonly Delivery[];
  readonly page?: { readonly next_cursor?: string | null; readonly has_more?: boolean };
}

/** The deliveries path for one subscription and one optional outcome. */
function deliveriesPath(webhookId: string, status?: string, cursor?: string): string {
  const query = new URLSearchParams();
  if (status) query.set('status', status);
  if (cursor) query.set('cursor', cursor);
  const written = query.toString();
  return `${WEBHOOKS}/${webhookId}/deliveries${written ? `?${written}` : ''}`;
}

class Webhooks {
  #state = $state<ResourceState<readonly Subscription[]>>({ status: 'idle' });
  #deliveries = $state<Record<string, ResourceState<DeliveryPage>>>({});
  #held = $state<Record<string, readonly Delivery[]>>({});
  #cursors = $state<Record<string, string | undefined>>({});

  get state(): ResourceState<readonly Subscription[]> {
    return this.#state;
  }

  get all(): readonly Subscription[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  deliveryState(webhookId: string, status?: string): ResourceState<DeliveryPage> {
    return this.#deliveries[deliveriesPath(webhookId, status)] ?? { status: 'idle' };
  }

  deliveriesOf(webhookId: string, status?: string): readonly Delivery[] {
    return this.#held[deliveriesPath(webhookId, status)] ?? [];
  }

  moreAfter(webhookId: string, status?: string): string | undefined {
    return this.#cursors[deliveriesPath(webhookId, status)];
  }

  /** Starts the listing. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly Subscription[]>({ path: WEBHOOKS }, (next) => {
      this.#state = next;
    });
  }

  /** Reads one subscription's deliveries, optionally narrowed to one outcome. */
  async readDeliveries(webhookId: string, status?: string): Promise<void> {
    const key = deliveriesPath(webhookId, status);
    this.#deliveries = { ...this.#deliveries, [key]: { status: 'loading' } };
    const answer = await engine.refresh<DeliveryPage>({ path: key });
    this.#deliveries = { ...this.#deliveries, [key]: answer };
    if (answer.status !== 'ready') return;
    this.#held = { ...this.#held, [key]: answer.data.data ?? [] };
    this.#cursors = { ...this.#cursors, [key]: answer.data.page?.next_cursor ?? undefined };
  }

  /** The next page, appended. Cursor pagination, never page numbers. */
  async more(webhookId: string, status?: string): Promise<void> {
    const key = deliveriesPath(webhookId, status);
    const cursor = this.#cursors[key];
    if (!cursor) return;
    const next = await engine.refresh<DeliveryPage>({
      path: deliveriesPath(webhookId, status, cursor),
    });
    if (next.status !== 'ready') return;
    this.#held = { ...this.#held, [key]: [...(this.#held[key] ?? []), ...(next.data.data ?? [])] };
    this.#cursors = { ...this.#cursors, [key]: next.data.page?.next_cursor ?? undefined };
  }

  /**
   * Subscribes, and answers the secret for the only time.
   *
   * The value is handed to the caller and not held: what the screen does with it is show it in
   * `OneTimeSecret` and drop it when the panel closes.
   */
  async subscribe(draft: {
    target_url: string;
    event_types: readonly string[];
  }): Promise<SubscriptionWithSecret> {
    return engine.mutate<SubscriptionWithSecret>('POST', WEBHOOKS, draft, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [WEBHOOKS],
    });
  }

  /**
   * Changes a subscription. Merge-patch: an omitted field is left alone.
   *
   * `DISABLED` is not settable by hand — it is what the system concludes from a run of failures —
   * so re-enabling one is `state: 'ACTIVE'`, and the trail records who decided the target is
   * reachable again.
   */
  async update(
    webhookId: string,
    change: { target_url?: string; event_types?: readonly string[]; state?: 'ACTIVE' | 'PAUSED' },
    version?: number,
  ): Promise<Subscription> {
    return engine.mutate<Subscription>('PATCH', `${WEBHOOKS}/${webhookId}`, change, {
      invalidates: [WEBHOOKS],
      ...(version === undefined ? {} : { ifMatch: String(version) }),
    });
  }

  /** Unsubscribes. The deliveries go with it: a log for an address nobody knows is a record of nothing. */
  async unsubscribe(webhookId: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${WEBHOOKS}/${webhookId}`, undefined, {
      invalidates: [WEBHOOKS],
    });
  }

  /**
   * Sends a dead-lettered delivery again, with the event id it always had.
   *
   * The subscriber that deduplicates on `X-Hubtask-Event-Id` sees the repeat for what it is, which
   * is what makes this safe to press.
   */
  async replay(webhookId: string, deliveryId: string): Promise<Delivery> {
    return engine.mutate<Delivery>(
      'POST',
      `${WEBHOOKS}/${webhookId}/deliveries/${deliveryId}:replay`,
      {},
      { idempotencyKey: crypto.randomUUID() },
    );
  }

  /** Delivers one event to this subscription by hand, through the pipeline every delivery takes. */
  async send(webhookId: string, eventId: string): Promise<Delivery> {
    return engine.mutate<Delivery>('POST', `${WEBHOOKS}/${webhookId}:send`, { event_id: eventId }, {
      idempotencyKey: crypto.randomUUID(),
    });
  }

  /**
   * Issues a new signing secret, and answers it once.
   *
   * `graceSeconds` is how long the previous one keeps verifying, so that a subscriber which cannot
   * deploy atomically does not drop what arrives in between. Zero retires the old one at once,
   * which is what a leak calls for — and that is a different decision from a routine rotation,
   * which is why the screen asks rather than choosing.
   */
  async rotate(webhookId: string, graceSeconds?: number): Promise<SubscriptionWithSecret> {
    return engine.mutate<SubscriptionWithSecret>(
      'POST',
      `${WEBHOOKS}/${webhookId}:rotate-secret`,
      graceSeconds === undefined ? {} : { grace_seconds: graceSeconds },
      { idempotencyKey: crypto.randomUUID(), invalidates: [WEBHOOKS] },
    );
  }
}

export const webhooks = new Webhooks();
