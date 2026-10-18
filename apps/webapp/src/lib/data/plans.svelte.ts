// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's plans (multi-tenancy.md §4.2), as the `/instance` area reads and writes them:
 * the list, one plan, and moving a workspace onto one.
 *
 * Every write goes through the elevated hour's step-up, as the instance store's do, and makes stale
 * what it changes - the plans, the workspaces whose plan it names, the overview and the journal.
 * A plan's save carries the version it read (`If-Match`): somebody who saved first is answered
 * `version_conflict`, never overwritten. A move's dry run writes nothing and so makes nothing stale.
 */

import type { Plan, PlanChange, PlanCreate, ResourceState, TenantPlanMoved } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';

const PLANS = '/admin/plans';
const TENANTS = '/admin/tenants';
/** What a plan's write makes stale: the plans, the workspaces' plan column, the overview, the journal. */
const STALE = [PLANS, TENANTS, '/admin/overview', '/admin/journal'];

class Plans {
  #list = $state<ResourceState<{ readonly data: readonly Plan[] }>>({ status: 'idle' });

  get state(): ResourceState<{ readonly data: readonly Plan[] }> {
    return this.#list;
  }

  /** The plans not removed, oldest first; empty until read. */
  get all(): readonly Plan[] {
    return this.#list.status === 'ready' ? this.#list.data.data : [];
  }

  /** One plan by its short name, from the list. */
  named(name: string): Plan | undefined {
    return this.all.find((plan) => plan.name === name);
  }

  /** Starts the listing. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<{ readonly data: readonly Plan[] }>({ path: PLANS }, (next) => {
      this.#list = next;
    });
  }

  async create(plan: PlanCreate): Promise<Plan> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<Plan>('POST', PLANS, plan, {
        idempotencyKey: crypto.randomUUID(),
        stepUpToken,
        invalidates: STALE,
      }),
    );
  }

  /** Replaces what the plan decides, on the version read: a value the body leaves out is undecided. */
  async change(name: string, change: PlanChange, version: number): Promise<Plan> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<Plan>('PUT', `${PLANS}/${encodeURIComponent(name)}`, change, {
        ifMatch: `"${version}"`,
        stepUpToken,
        invalidates: STALE,
      }),
    );
  }

  async remove(name: string): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate('DELETE', `${PLANS}/${encodeURIComponent(name)}`, undefined, {
        stepUpToken,
        invalidates: STALE,
      }),
    );
  }

  /**
   * Moves a workspace onto a plan (`null` for none) on the version read, or - `dryRun` - answers what
   * the move would change and changes nothing. Each call carries a key of its own: a dry run's and
   * the move's bodies differ, and one key for both would be refused as reused. A workspace read
   * without a version moves unguarded: a guess would be refused as a conflict every time.
   */
  async move(tenantId: string, plan: string | null, version: number | undefined, dryRun: boolean): Promise<TenantPlanMoved> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<TenantPlanMoved>(
        'POST',
        `${TENANTS}/${tenantId}:move-plan`,
        { plan, ...(version === undefined ? {} : { expected_version: version }), ...(dryRun ? { dry_run: true } : {}) },
        {
          idempotencyKey: crypto.randomUUID(),
          stepUpToken,
          invalidates: dryRun ? [] : STALE,
        },
      ),
    );
  }
}

export const plans = new Plans();
