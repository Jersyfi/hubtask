<!--
  SPDX-License-Identifier: Apache-2.0
  Copyright (c) 2026 Jérôme Bastian Winkel
-->
<script lang="ts">
  // A change the server refused after reconnecting, opened as a draft (user-experience.md §5.11):
  // what the person wrote, prefilled and editable, with the server's reason, sent again as a new
  // write or discarded. The refusal stays on the list until one of the two is done.
  import { Button, Dialog, Input, Stack, Textarea } from '@hubtask/design-system/components';

  import { queue, type ListedRefusal } from '../data/queue.svelte.ts';
  import { draftFields } from '../data/queueplaces.ts';
  import { toast } from './toasts.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    refusal: ListedRefusal;
    isOpen: boolean;
  }

  let { refusal, isOpen = $bindable() }: Props = $props();

  let values = $state<Record<string, string>>({});
  let isSending = $state(false);
  $effect(() => {
    if (isOpen) values = draftFields(refusal.record.local);
  });

  async function send(): Promise<void> {
    isSending = true;
    try {
      if (await queue.resend(refusal.id, values)) {
        toast.show({ text: t('app.sync.draft.sent'), tone: 'success' });
        isOpen = false;
      }
    } finally {
      isSending = false;
    }
  }

  async function discard(): Promise<void> {
    await queue.dismiss(refusal.id);
    isOpen = false;
  }
</script>

<Dialog title={t('app.sync.draft.title')} bind:isOpen dismissLabel={t('app.sync.dismiss')}>
  <Stack gap="200">
    <p>{t('app.sync.draft.intro', { what: refusal.what, where: refusal.where })}</p>
    <p class="reason" role="alert" data-refusal-reason>{refusal.reason}</p>
    {#each Object.keys(values) as field (field)}
      {#if field === 'notes' || field === 'description' || field === 'body'}
        <Textarea label={t(`app.sync.draft.field.${field}`)} error={refusal.fieldReasons[field]} bind:value={values[field]} />
      {:else}
        <Input label={t(`app.sync.draft.field.${field}`)} error={refusal.fieldReasons[field]} bind:value={values[field]} />
      {/if}
    {:else}
      <p>{t('app.sync.draft.no_fields')}</p>
    {/each}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" data-draft-discard onclick={() => void discard()}>{t('app.sync.draft.discard')}</Button>
    <Button tone="primary" data-draft-send isBusy={isSending} busyLabel={t('app.sync.draft.sending')} onclick={() => void send()}>{t('app.sync.draft.send')}</Button>
  {/snippet}
</Dialog>

<style>
  .reason {
    color: var(--text-danger);
  }
</style>
