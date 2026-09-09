// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What rules did, what one would do, and how a failed run is finished (G-07, `automation.md` §3).
 *
 * **The dry run writes nothing.** `POST /automation/rules:test` evaluates a rule against a sample
 * event and answers what it *would* do — both arms of every branch, because the honest answer to
 * "what would happen" includes "and what if it had not". Naming a real entry lets a condition
 * resolve it, which is a read and never a write.
 *
 * **A replay completes a failed run rather than starting a new one.** The remaining actions run
 * under the same idempotency keys, so an action the original run already completed is not done
 * twice. Confusing a replay with a re-trigger is how somebody sends the same mail twice, which is
 * why this module names them differently and the screen says which is which.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const RUNS = '/automation/runs';
const RULES = '/automation/rules';

/** How one condition answered, in the order the rule declares them. */
export interface ConditionResult {
  readonly index: number;
  readonly matched: boolean;
  /** Present when it could not be evaluated at all — distinct from a condition that simply did not hold. */
  readonly error_code?: string;
}

/** One action's outcome, in the order the run reached them. */
export interface ActionResult {
  readonly index: number;
  readonly kind: string;
  readonly path?: string;
  readonly matched?: boolean;
  readonly status: string;
  readonly error_code?: string;
}

/** One run, as `RuleRun` answers it. */
export interface Run {
  readonly id: string;
  readonly rule_id: string;
  readonly trigger: string;
  readonly triggered_by?: string | null;
  readonly subject_id?: string | null;
  readonly status: string;
  readonly condition_results: readonly ConditionResult[];
  readonly action_results: readonly ActionResult[];
  readonly started_at: string;
  readonly finished_at?: string | null;
  readonly causation_depth: number;
  readonly is_dry_run?: boolean;
  readonly error_code?: string;
}

interface RunPage {
  readonly data?: readonly Run[];
  readonly next_cursor?: string | null;
}

/** One action of a rule, with whether it would have run. Both arms of every branch appear. */
export interface TestAction {
  readonly path: string;
  readonly kind: string;
  readonly would_run: boolean;
  readonly summary?: string;
}

/** What a dry run answers. */
export interface TestResult {
  readonly matched: boolean;
  readonly condition_results: readonly ConditionResult[];
  readonly actions: readonly TestAction[];
}

/** The listing's path for a filter. Built here so the store and its callers agree on one string. */
export function runsPath(filter: { ruleId?: string; status?: string } = {}, cursor?: string): string {
  const query = new URLSearchParams();
  if (filter.ruleId) query.set('rule_id', filter.ruleId);
  if (filter.status) query.set('status', filter.status);
  if (cursor) query.set('cursor', cursor);
  const written = query.toString();
  return written ? `${RUNS}?${written}` : RUNS;
}

class Runs {
  #pages = $state<Record<string, ResourceState<RunPage>>>({});
  #held = $state<Record<string, readonly Run[]>>({});
  #cursors = $state<Record<string, string | undefined>>({});
  #details = $state<Record<string, Run>>({});

  stateOf(filter: { ruleId?: string; status?: string }): ResourceState<RunPage> {
    return this.#pages[runsPath(filter)] ?? { status: 'idle' };
  }

  of(filter: { ruleId?: string; status?: string }): readonly Run[] {
    return this.#held[runsPath(filter)] ?? [];
  }

  moreAfter(filter: { ruleId?: string; status?: string }): string | undefined {
    return this.#cursors[runsPath(filter)];
  }

  /** One run with every condition and every action result. */
  detail(runId: string): Run | undefined {
    return this.#details[runId];
  }

  /** Starts a listing. **From `untrack`**, for the reason every other store records. */
  open(filter: { ruleId?: string; status?: string } = {}): () => void {
    const key = runsPath(filter);
    return engine.subscribe<RunPage>({ path: key }, (next) => {
      this.#pages = { ...this.#pages, [key]: next };
      if (next.status === 'ready') {
        this.#held = { ...this.#held, [key]: next.data.data ?? [] };
        this.#cursors = { ...this.#cursors, [key]: next.data.next_cursor ?? undefined };
      }
    });
  }

  /** The next page, appended. Cursor pagination, never page numbers. */
  async more(filter: { ruleId?: string; status?: string } = {}): Promise<void> {
    const key = runsPath(filter);
    const cursor = this.#cursors[key];
    if (!cursor) return;
    const next = await engine.refresh<RunPage>({ path: runsPath(filter, cursor) });
    if (next.status !== 'ready') return;
    this.#held = { ...this.#held, [key]: [...(this.#held[key] ?? []), ...(next.data.data ?? [])] };
    this.#cursors = { ...this.#cursors, [key]: next.data.next_cursor ?? undefined };
  }

  async read(runId: string): Promise<void> {
    const state = await engine.refresh<Run>({ path: `${RUNS}/${runId}` });
    if (state.status === 'ready') this.#details = { ...this.#details, [runId]: state.data };
  }

  /** Evaluates a stored rule against a sample event. Writes nothing. */
  async dryRun(ruleId: string, event: { type: string; subject?: string }): Promise<TestResult> {
    return engine.mutate<TestResult>('POST', `${RULES}:test`, {
      rule_id: ruleId,
      sample_event: { type: event.type, ...(event.subject ? { subject: event.subject } : {}) },
    });
  }

  /** Runs it now, for real. A different thing from a dry run, and named differently. */
  async trigger(ruleId: string): Promise<Run> {
    return engine.mutate<Run>('POST', `${RULES}/${ruleId}:trigger`, {}, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [RUNS],
    });
  }

  /** Finishes a failed run. The actions it already completed are not done again. */
  async replay(runId: string): Promise<Run> {
    return engine.mutate<Run>('POST', `${RUNS}/${runId}:replay`, {}, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [RUNS],
    });
  }
}

export const runs = new Runs();
