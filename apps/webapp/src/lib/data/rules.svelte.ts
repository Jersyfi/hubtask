// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Automation rules: written, read back, switched on (G-05, `automation.md` §1).
 *
 * **A rule is created switched off**, and `:enable` and `:disable` are what move it — so the trail
 * says which of the two somebody did. There is no `enabled` field to write, which is why the form
 * has no such control and this module offers no such argument.
 *
 * **Writing a rule needs the automation permission *and* the rights the rule's own actions need.**
 * The second half is not something a client can compute — it depends on what each action's use case
 * demands of the account the rule runs as — so nothing here pre-empts it. The server refuses and
 * the screen renders the refusal.
 *
 * **The vocabulary is the manifest's.** Which triggers and which action kinds exist is
 * `/meta/capabilities`, not a table in here: an installation that serves one more action gets one
 * more option without a release of this client.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/automation/rules';

/** One step of a run. `BRANCH` nests, because the contract nests. */
export interface RuleAction {
  readonly kind: string;
  readonly params?: Record<string, unknown>;
  readonly then?: readonly RuleAction[];
  readonly else?: readonly RuleAction[];
}

/**
 * An action while it is being edited.
 *
 * Mutable, and separate from `RuleAction` on purpose: what the server answers is read-only, and a
 * form that edited the answer in place would be editing the cache. `paramsText` is the editor's
 * own — the parameters are typed as JSON and parsed on the way out, so that a half-typed object is
 * a thing the reader can still see rather than a parse error that ate their input.
 */
export interface DraftAction {
  kind: string;
  params?: Record<string, unknown>;
  paramsText?: string;
  then?: DraftAction[];
  else?: DraftAction[];
}

/** What starts a rule. Each kind takes its own fields and no others. */
export interface RuleTrigger {
  readonly kind: string;
  readonly event_type?: string;
  readonly changed_fields?: readonly string[];
  readonly rrule?: string;
  readonly timezone?: string;
  readonly anchor?: string;
  readonly offset?: string;
}

/** A rule, as `AutomationRule` answers it. */
export interface Rule {
  readonly id: string;
  readonly name: string;
  readonly scope: { readonly type: string; readonly id?: string | null };
  readonly enabled: boolean;
  readonly run_as: string;
  readonly trigger: RuleTrigger;
  readonly conditions: readonly { readonly expr: string }[];
  readonly actions: readonly RuleAction[];
  readonly throttle?: { readonly max_runs_per_hour?: number; readonly dedupe_key_expr?: string };
  readonly on_error: string;
  readonly failure_count: number;
  readonly next_run_at?: string | null;
  readonly version: number;
}

interface RulePage {
  readonly data?: readonly Rule[];
  readonly next_cursor?: string | null;
}

/** What writing a rule takes. No `enabled`: a rule is created switched off, always. */
export interface RuleDraft {
  readonly name: string;
  readonly scope: { type: string; id?: string };
  readonly run_as: string;
  readonly trigger: RuleTrigger;
  readonly conditions: readonly { expr: string }[];
  readonly actions: readonly RuleAction[];
  readonly throttle?: { max_runs_per_hour?: number; dedupe_key_expr?: string };
  readonly on_error?: string;
}

class Rules {
  #page = $state<ResourceState<RulePage>>({ status: 'idle' });

  get state(): ResourceState<RulePage> {
    return this.#page;
  }

  get all(): readonly Rule[] {
    return this.#page.status === 'ready' ? (this.#page.data.data ?? []) : [];
  }

  /** Starts the listing. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<RulePage>({ path: PATH }, (next) => {
      this.#page = next;
    });
  }

  async write(draft: RuleDraft): Promise<Rule> {
    return engine.mutate<Rule>('POST', PATH, draft, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [PATH],
    });
  }

  async change(ruleId: string, draft: RuleDraft, version: number): Promise<Rule> {
    return engine.mutate<Rule>('PATCH', `${PATH}/${ruleId}`, draft, {
      ifMatch: String(version),
      invalidates: [PATH],
    });
  }

  /** Two calls rather than a flag, so the trail says which of the two somebody did. */
  async enable(ruleId: string): Promise<void> {
    await engine.mutate('POST', `${PATH}/${ruleId}:enable`, {}, { invalidates: [PATH] });
  }

  async disable(ruleId: string): Promise<void> {
    await engine.mutate('POST', `${PATH}/${ruleId}:disable`, {}, { invalidates: [PATH] });
  }

  async remove(ruleId: string): Promise<void> {
    await engine.mutate('DELETE', `${PATH}/${ruleId}`, undefined, { invalidates: [PATH] });
  }
}

export const rules = new Rules();
export { PATH as rulesPath };
