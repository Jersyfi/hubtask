<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One delivery as a Dialog M: how it ended, the attempt, the target's answer as a status and the
  // failure as a sentence - never the target's response body -, the next attempt, and for a dead
  // letter *Replay*. The event's identifier under *Details for support* (PS-15).
  import { Badge, Button, Dialog, Disclosure, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { webhooks, type Delivery } from '../data/webhooks.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { outcomeTone, sentence } from './words.svelte.ts';

  interface Props {
    webhookId: string;
    delivery: Delivery | undefined;
    /** Whether a dead letter may be replayed: the automation permission and webhooks in the plan. */
    mayReplay: boolean;
    outcome: string;
    onClose: () => void;
  }

  const { webhookId, delivery, mayReplay, outcome, onClose }: Props = $props();

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : '');

  async function replay(): Promise<void> {
    if (!delivery) return;
    failure = undefined;
    isWorking = true;
    try {
      await webhooks.replay(webhookId, delivery.id);
      await webhooks.readDeliveries(webhookId, outcome || undefined);
      announcer.say(t('app.webhooks.replayed_announced'));
      onClose();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  const isReplayable = $derived(delivery?.status === 'DEAD_LETTER' && mayReplay);
</script>

<Dialog
  size="m"
  isOpen={delivery !== undefined}
  title={delivery ? t(`app.webhooks.outcome_${delivery.status.toLowerCase()}`) : ''}
  dismissLabel={t('app.page.close')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={isReplayable ? { label: t('app.webhooks.replay'), isBusy: isWorking, busyLabel: t('app.webhooks.replaying'), onclick: () => void replay() } : undefined}
>
  {#if delivery}
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <dl class="facts" data-delivery={delivery.id}>
        <div class="fact"><dt>{t('app.webhooks.delivery_outcome')}</dt><dd><Badge tone={outcomeTone(delivery)}>{t(`app.webhooks.outcome_${delivery.status.toLowerCase()}`)}</Badge></dd></div>
        <div class="fact"><dt>{t('app.webhooks.delivery_when')}</dt><dd>{when(delivery.created_at)}</dd></div>
        {#if delivery.attempt > 0}<div class="fact"><dt>{t('app.webhooks.delivery_attempts')}</dt><dd>{t('app.webhooks.attempt', { count: String(delivery.attempt) })}</dd></div>{/if}
        {#if delivery.response_status}<div class="fact"><dt>{t('app.webhooks.delivery_answer')}</dt><dd>{t('app.webhooks.answered', { status: String(delivery.response_status) })}</dd></div>{/if}
        {#if delivery.next_attempt_at}<div class="fact"><dt>{t('app.webhooks.delivery_next')}</dt><dd>{t('app.webhooks.next_attempt', { at: when(delivery.next_attempt_at) })}</dd></div>{/if}
      </dl>
      {#if delivery.error_code}<p class="failure">{sentence(delivery.error_code)}</p>{/if}
      {#if isReplayable}
        <!-- Said before the button: the replay carries the same event id, so a subscriber that
             deduplicates recognises the repeat. That is what makes it safe to press. -->
        <p class="quiet">{t('app.webhooks.replay_note')}</p>
      {/if}
      <Disclosure label={t('app.audit.support')} tone="quiet">
        <dl class="support">
          <dt>{t('app.webhooks.support_event')}</dt><dd class="mono">{delivery.event_id}</dd>
          <dt>{t('app.webhooks.support_delivery')}</dt><dd class="mono">{delivery.id}</dd>
          {#if delivery.error_code}<dt>{t('app.webhooks.support_error')}</dt><dd class="mono">{delivery.error_code}</dd>{/if}
        </dl>
      </Disclosure>
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .failure { margin: 0; color: var(--text-danger); }
  .facts { margin: 0; display: grid; gap: var(--sp-100); }
  .fact { display: grid; grid-template-columns: minmax(0, 14ch) minmax(0, 1fr); gap: var(--sp-150); align-items: center; }
  .fact dt { color: var(--text-secondary); }
  .fact dd { margin: 0; }
  .support { margin: 0; display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: var(--sp-050) var(--sp-150); font-size: var(--fs-075); }
  .support dt { color: var(--text-secondary); }
  .support dd { margin: 0; }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
</style>
