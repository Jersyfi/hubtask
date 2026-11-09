<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A preset's withdrawal (page-system §5.2, Dialog S), saying what it costs: nothing made from it.
  // The default is not offered here - the list's menu says why instead (design-system.md §4).
  import { Button, Dialog } from '@hubtask/design-system/components';
  import type { Preset, TransportError } from '@hubtask/sync-engine';

  import { presets } from '../data/presets.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { preset, onClose }: { preset?: Preset; onClose: () => void } = $props();

  let isOpen = $state(false);
  let isWorking = $state(false);
  let failure = $state<string | undefined>(undefined);

  $effect(() => {
    isOpen = preset !== undefined;
    failure = undefined;
  });

  async function withdraw(): Promise<void> {
    if (!preset?.id || isWorking) return;
    isWorking = true;
    failure = undefined;
    try {
      await presets.withdraw(preset.id, preset.version ?? 0);
      toast.show({ text: t('app.presets.withdrawn'), tone: 'success' });
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as TransportError, messages).message;
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="s"
  title={t('app.presets.withdraw_question', { name: preset?.name ?? '' })}
  bind:isOpen
  dismissLabel={t('app.presets.cancel')}
  {onClose}
  primary={{ label: t('app.presets.withdraw'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.presets.withdrawing'), onclick: () => void withdraw() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body" data-withdraw-preset>{t('app.presets.withdraw_about')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.presets.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
