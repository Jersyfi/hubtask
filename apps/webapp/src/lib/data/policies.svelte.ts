// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The rules that delete things, the preview that comes first, and the holds that outrank them.
 *
 * **Not `retention.svelte.ts`, and the two must not be merged.** That module reads
 * `/retention-policies` for the one sentence the trash screen shows — "it will be gone anyway in a
 * week" — and asks *silently*, because most members hold no `retention:read` and a refusal there
 * is not a message. This is the other reader: an administrative screen where every refusal is
 * rendered and where the answer is the whole listing rather than one number.
 *
 * **The preview is the point.** A rule that deletes is written, previewed, and only then armed;
 * `:preview` writes nothing and answers what the rule would catch, how much of the workspace that
 * is, and a handful of examples. It is a `POST` on a stored rule, so the rule exists before it can
 * be previewed — which is why the screen writes it disabled and arms it afterwards.
 *
 * **The effective listing is a different question.** `?effective=true&container_id=…` answers the
 * rules actually in force in one place, inheritance included, and that is what "what happens to
 * what is in here" needs: the answer is rarely the rule written at this level.
 *
 * **A hold is not a `:retain`.** `:retain` takes one entry out of the running period; a hold is an
 * instruction that overrides every rule and every deletion anybody asks for. The screen keeps them
 * apart because confusing them is how somebody thinks a case is protected when one object is.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const POLICIES = '/retention-policies';
const HOLDS = '/legal-holds';

/** What a rule does when its period is up. `NOTIFY_ONLY` reports and removes nothing. */
export type RetentionAction =
  | 'ARCHIVE' | 'TRASH' | 'ANONYMIZE' | 'HARD_DELETE' | 'EXPORT_THEN_DELETE' | 'NOTIFY_ONLY';

/** One rule, as `RetentionPolicy` answers it. */
export interface Policy {
  readonly id: string;
  readonly scope?: { readonly kind?: string; readonly id?: string | null };
  readonly data_kind: string;
  readonly condition?: string | null;
  readonly retain_days: number;
  readonly action: RetentionAction;
  readonly then_after_days?: number | null;
  readonly then_action?: string | null;
  readonly grace_days?: number;
  readonly notify?: { readonly before_days?: number; readonly recipients?: readonly string[] };
  readonly justification?: string | null;
  readonly enabled?: boolean;
  readonly export_target_id?: string | null;
  /**
   * Whether this is the rule that would act in the container the listing named.
   *
   * Absent from a listing that named no container, and that is the contract's own shape rather
   * than a gap: with nothing to be in force *in*, the question has no answer.
   */
  readonly in_force?: boolean;
}

/** What `:preview` answers. It writes nothing. */
export interface Preview {
  readonly matched?: number;
  /** Why objects were kept back, by reason: `legal_hold`, `restriction`, `tombstone_window`. */
  readonly blocked?: Readonly<Record<string, number>>;
  /** The share of the holdings, 0..1. Above 0.05 on a first run is what forces notify-only. */
  readonly share_of_scope?: number;
  readonly samples?: readonly {
    readonly id: string;
    readonly title: string;
    readonly effective_at: string;
  }[];
}

/** One instruction not to delete something. */
export interface Hold {
  readonly id: string;
  readonly scope: { readonly kind: string; readonly id?: string | null };
  readonly reason: string;
  readonly placed_by: string;
  readonly placed_at: string;
  readonly released_by?: string | null;
  readonly released_at?: string | null;
  readonly released_reason?: string | null;
}

/** The listing's path. Built here so the store and its callers agree on one string. */
export function policiesPath(filter: { containerId?: string; effective?: boolean } = {}): string {
  const query = new URLSearchParams();
  if (filter.containerId) query.set('container_id', filter.containerId);
  if (filter.effective) query.set('effective', 'true');
  const written = query.toString();
  return written ? `${POLICIES}?${written}` : POLICIES;
}

class Policies {
  #listings = $state<Record<string, ResourceState<readonly Policy[]>>>({});
  #holds = $state<ResourceState<readonly Hold[]>>({ status: 'idle' });
  #previews = $state<Record<string, Preview>>({});

  stateOf(filter: { containerId?: string; effective?: boolean } = {}): ResourceState<readonly Policy[]> {
    return this.#listings[policiesPath(filter)] ?? { status: 'idle' };
  }

  of(filter: { containerId?: string; effective?: boolean } = {}): readonly Policy[] {
    const state = this.stateOf(filter);
    return state.status === 'ready' ? state.data : [];
  }

  get holds(): ResourceState<readonly Hold[]> {
    return this.#holds;
  }

  get placed(): readonly Hold[] {
    return this.#holds.status === 'ready' ? this.#holds.data : [];
  }

  /** What the last preview of one rule found. Held per rule, and never persisted. */
  previewOf(policyId: string): Preview | undefined {
    return this.#previews[policyId];
  }

  /** Starts a listing. **From `untrack`**, for the reason every other store records. */
  open(filter: { containerId?: string; effective?: boolean } = {}): () => void {
    const key = policiesPath(filter);
    return engine.subscribe<readonly Policy[]>({ path: key }, (next) => {
      this.#listings = { ...this.#listings, [key]: next };
    });
  }

  /**
   * Starts the holds listing, released ones included where asked.
   *
   * A released hold is what an auditor reads to see that it was lifted, so it is a choice rather
   * than a default: a list that always showed them would bury what is in force.
   */
  openHolds(includeReleased = false): () => void {
    const path = includeReleased ? `${HOLDS}?include_released=true` : HOLDS;
    return engine.subscribe<readonly Hold[]>({ path }, (next) => {
      this.#holds = next;
    });
  }

  async create(draft: {
    scope: { kind: string; id?: string | null };
    data_kind: string;
    retain_days: number;
    action: RetentionAction;
    justification?: string;
    enabled?: boolean;
  }): Promise<Policy> {
    return engine.mutate<Policy>('POST', POLICIES, draft, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [POLICIES],
    });
  }

  /** Corrects a rule. Merge-patch: an absent key changes nothing; the kind and the scope do not move. */
  async update(policyId: string, change: Record<string, unknown>): Promise<Policy> {
    return engine.mutate<Policy>('PATCH', `${POLICIES}/${policyId}`, change, {
      invalidates: [POLICIES],
    });
  }

  /**
   * Withdraws a rule.
   *
   * What it already did stands — retention deletes, and a deletion is not undone by withdrawing
   * the instruction — and what it had marked for a future pass is unmarked.
   */
  async withdraw(policyId: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${POLICIES}/${policyId}`, undefined, {
      invalidates: [POLICIES],
    });
  }

  /** What the rule would catch. Writes nothing, and is what has to be read before it is armed. */
  async preview(policyId: string): Promise<Preview> {
    const answer = await engine.mutate<Preview>('POST', `${POLICIES}/${policyId}:preview`, {});
    this.#previews = { ...this.#previews, [policyId]: answer };
    return answer;
  }

  /**
   * Takes one entry out of the running period.
   *
   * An exception on one object, and not a hold: a hold is an instruction that overrides every rule
   * and reaches everything under it.
   */
  async retain(itemId: string): Promise<void> {
    await engine.mutate<unknown>('POST', `/items/${itemId}:retain`, {}, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: ['/items'],
    });
  }

  /** Places a hold. The reason is mandatory and is kept: it is what an auditor reads. */
  async place(scope: { kind: string; id?: string | null }, reason: string): Promise<Hold> {
    return engine.mutate<Hold>('POST', HOLDS, { scope, reason }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [HOLDS],
    });
  }

  /** Lifts one. The reason is required here too — "released" with no reason is unactionable. */
  async release(holdId: string, reason: string): Promise<Hold> {
    return engine.mutate<Hold>('POST', `${HOLDS}/${holdId}:release`, { reason }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [HOLDS],
    });
  }
}

export const policies = new Policies();
