<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *New secret…* as a Dialog M: what rotation costs, before it is pressed. Signatures use the new
  // secret from the moment of the call; the grace period is the subscriber's side of the check,
  // which is the half that cannot be deployed atomically.
  import { Button, Dialog, Select, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { webhooks, type Subscription } from '../data/webhooks.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    subscription: Subscription | undefined;
    onClose: () => void;
    /** The new secret, to be shown once. */
    onRotated: (secret: string) => void;
  }

  const { subscription, onClose, onRotated }: Props = $props();

  /** The rotation choices, in seconds. Zero is its own decision and says so. */
  const GRACE_CHOICES = ['3600', '86400', '604800', '0'];
  let grace = $state('86400');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  async function rotate(): Promise<void> {
    if (!subscription) return;
    failure = undefined;
    isWorking = true;
    try {
      const rotated = await webhooks.rotate(subscription.id, Number.parseInt(grace, 10));
      announcer.say(t('app.webhooks.rotated_announced'));
      onClose();
      onRotated(rotated.secret);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  isOpen={subscription !== undefined}
  title={t('app.webhooks.rotate_title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { failure = undefined; grace = '86400'; onClose(); }}
  primary={{ label: t('app.webhooks.rotate_confirm'), isBusy: isWorking, busyLabel: t('app.webhooks.rotating'), onclick: () => void rotate() }}
>
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <p class="quiet">{t('app.webhooks.rotate_note')}</p>
    <Select
      label={t('app.webhooks.grace')}
      hint={t('app.webhooks.grace_hint')}
      bind:value={grace}
      options={GRACE_CHOICES.map((seconds) => ({ value: seconds, label: t(`app.webhooks.grace_${seconds}`) }))}
    />
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
