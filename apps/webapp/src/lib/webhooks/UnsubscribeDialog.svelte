<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Unsubscribe?* as a Dialog S. The deliveries go with it: a log of attempts against an address
  // the workspace no longer knows is a record of nothing.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { webhooks, type Subscription } from '../data/webhooks.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    subscription: Subscription | undefined;
    onClose: () => void;
    onRemoved?: () => void;
  }

  const { subscription, onClose, onRemoved }: Props = $props();

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  async function remove(): Promise<void> {
    if (!subscription) return;
    failure = undefined;
    isWorking = true;
    try {
      await webhooks.unsubscribe(subscription.id);
      announcer.say(t('app.webhooks.unsubscribed_announced'));
      onClose();
      onRemoved?.();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="s"
  isOpen={subscription !== undefined}
  title={t('app.webhooks.unsubscribe_title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={{ label: t('app.webhooks.unsubscribe_confirm'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.webhooks.unsubscribing'), onclick: () => void remove() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body">{t('app.webhooks.unsubscribe_note')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
