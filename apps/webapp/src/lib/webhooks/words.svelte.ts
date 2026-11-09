// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** A webhook's words, shared by its list, its record page and its dialogs (`automation.md` §5). */

import { containers } from '../data/containers.svelte.ts';
import type { Delivery, Subscription } from '../data/webhooks.svelte.ts';
import { eventKey, scopeChoiceOf } from '../data/webhooks.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';

export const stateWord = (subscription: Pick<Subscription, 'state'>): string => t(`app.webhooks.state_${subscription.state.toLowerCase()}`);

export const stateTone = (subscription: Pick<Subscription, 'state'>): 'success' | 'neutral' | 'danger' =>
  subscription.state === 'ACTIVE' ? 'success' : subscription.state === 'PAUSED' ? 'neutral' : 'danger';

export const outcomeTone = (delivery: Pick<Delivery, 'status'>): 'success' | 'danger' | 'warning' | 'neutral' =>
  delivery.status === 'SUCCEEDED'
    ? 'success'
    : delivery.status === 'DEAD_LETTER'
      ? 'danger'
      : delivery.status === 'FAILED' || delivery.status === 'FILTER_FAILED'
        ? 'warning'
        : 'neutral';

/** An event type in words (user-experience.md §5.4, pickers never keys); a type without words keeps its name. */
export const eventWord = (type: string): string => {
  const code = `app.webhooks.event.${eventKey(type)}`;
  return messages.has(code) ? t(code) : type;
};

/** A server code as a sentence, or the code where it has none. */
export const sentence = (code: string | null | undefined): string | undefined => (code ? (messages.has(code) ? t(code) : code) : undefined);

/**
 * Where a subscription may listen: the workspace, a hub with its collections, or one collection -
 * the hubs and collections the reader sees, as the rule editor offers them. Read from `untrack`ed
 * `containers.start()` and `openLevel` by the screen that draws it.
 */
export function webhookScopes(): { value: string; label: string }[] {
  return [
    { value: 'TENANT', label: t('app.rules.scope_tenant') },
    ...containers.hubs.map((hub) => ({ value: `HUB:${hub.id}`, label: t('app.rules.scope_hub', { name: hub.name }) })),
    ...containers.hubs.flatMap((hub) =>
      containers.collectionsOf(hub.id).map((collection) => ({
        value: `COLLECTION:${collection.id}`,
        label: t('app.rules.scope_collection', { name: `${hub.name} · ${collection.name}` }),
      })),
    ),
  ];
}

/** Where a stored subscription listens, in the picker's words; a container the reader does not see is named by nothing more. */
export function listensAt(subscription: Subscription, scopes: readonly { value: string; label: string }[]): string {
  const choice = scopeChoiceOf(subscription.scope);
  return scopes.find((each) => each.value === choice)?.label ?? t('app.webhooks.scope_unseen');
}
