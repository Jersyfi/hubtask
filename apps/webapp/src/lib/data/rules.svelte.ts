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

/**
 * One step of a rule, as `RuleAction` answers it: a kind and its parameters. `BRANCH` nests inside
 * `params` - `then` and `else` are what the kind takes, beside `condition` - because that is where
 * the domain reads them and where a finding's path points; the shape that put the arms beside
 * `params` was refused on every write and read back empty (issue 853).
 */
export interface RuleAction {
  readonly kind: string;
  readonly params?: Record<string, unknown>;
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
  /** When an `INBOUND_WEBHOOK` rule's address was last minted; the moment and nothing else. */
  readonly inbound_rotated_at?: string | null;
  /** What the check found (ADR-0060), and when it last ran; empty and absent for a rule never checked. */
  readonly findings?: readonly RuleFinding[];
  readonly checked_at?: string | null;
  /** The most recent run - when it started and how it ended - read beside the rule (F8-21); absent for a rule that never ran. */
  readonly last_run?: { readonly at: string; readonly status: string } | null;
  readonly version: number;
}

/** One thing the check found: a level, the pointer into the rule's document, a code and its parameters. */
export interface RuleFinding {
  readonly level: 'ATTENTION' | 'BROKEN';
  readonly path: string;
  readonly code: string;
  readonly params?: Record<string, string>;
}

/** A freshly minted inbound address, for the only time the token exists outside the server's hash. */
export interface InboundToken {
  readonly rule_id: string;
  readonly token: string;
  readonly rotated_at: string;
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

  /**
   * The check (ADR-0060): every rule the caller may read, its references resolved against what
   * exists now, the findings written on the rules and answered here. The list calls it when it
   * opens, which is what makes "after an update, the rules that need attention are shown" true
   * without anything enumerating tenants.
   */
  async check(): Promise<readonly Rule[]> {
    const answer = await engine.mutate<{ data?: readonly Rule[] }>('POST', `${PATH}:check`, {}, { invalidates: [PATH] });
    return answer.data ?? [];
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

  /**
   * Mints the address an `INBOUND_WEBHOOK` rule answers on, and answers the token once. Rotating
   * is revoking: the replacement happens in one statement, so the old address stops at this
   * moment (`automation.md` §1.1). The listing is re-read for `inbound_rotated_at`.
   */
  async rotateInbound(ruleId: string): Promise<InboundToken> {
    return engine.mutate<InboundToken>('POST', `${PATH}/${ruleId}:rotate-inbound-token`, {}, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [PATH],
    });
  }
}

export const rules = new Rules();
export { PATH as rulesPath };
