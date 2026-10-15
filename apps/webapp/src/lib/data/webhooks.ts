// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A subscription's narrowing, as the form chooses it and the contract spells it (`automation.md`
 * §3.1): a condition, and where it listens.
 *
 * The scope is chosen from one list whose values say both halves - `TENANT`, `HUB:<id>`,
 * `COLLECTION:<id>` - the way the rule editor's scope does, so that a level can never be sent
 * without its container or a container without its level.
 */

/** Where a subscription listens. `TENANT` names nothing, because it is everything. */
export interface WebhookScope {
  readonly type: 'TENANT' | 'HUB' | 'COLLECTION';
  readonly id?: string;
}

/** The scope as one choice of the picker. Absent is the whole workspace, as the contract reads it. */
export function scopeChoiceOf(scope: WebhookScope | null | undefined): string {
  if (!scope || scope.type === 'TENANT' || !scope.id) return 'TENANT';
  return `${scope.type}:${scope.id}`;
}

/** The picker's choice as the contract's scope; the whole workspace is sent as nothing at all. */
export function scopeOfChoice(choice: string): WebhookScope | undefined {
  const [type, id] = choice.split(':', 2);
  if ((type === 'HUB' || type === 'COLLECTION') && id) return { type, id };
  return undefined;
}

/** What a subscription asks for, as the form holds it before it is sent. */
export interface SubscriptionDraft {
  readonly target_url: string;
  readonly event_types: readonly string[];
  readonly condition: string;
  readonly scope: string;
}

/** The body of a subscription: a blank condition and the whole workspace are left out, not sent empty. */
export function subscriptionBody(draft: SubscriptionDraft): {
  target_url: string;
  event_types: readonly string[];
  filter?: string;
  scope?: WebhookScope;
} {
  const condition = draft.condition.trim();
  const scope = scopeOfChoice(draft.scope);
  return {
    target_url: draft.target_url.trim(),
    event_types: draft.event_types,
    ...(condition ? { filter: condition } : {}),
    ...(scope ? { scope } : {}),
  };
}
