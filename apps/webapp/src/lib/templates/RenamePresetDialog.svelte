<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A preset's rename (page-system §5.1, Dialog S: one field), under the version read. What was
  // made from the preset keeps its settings: a preset is copied, never followed.
  import { Button, Dialog, Input } from '@hubtask/design-system/components';
  import type { Preset, TransportError } from '@hubtask/sync-engine';

  import { presets } from '../data/presets.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { preset, onClose }: { preset?: Preset; onClose: () => void } = $props();

  let isOpen = $state(false);
  let name = $state('');
  let isSaving = $state(false);
  let failure = $state<string | undefined>(undefined);
  let form = $state<HTMLFormElement | undefined>(undefined);

  $effect(() => {
    isOpen = preset !== undefined;
    name = preset?.name ?? '';
    failure = undefined;
  });

  const isUnchanged = $derived(name.trim() === '' || name.trim() === preset?.name);

  async function rename(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const typed = name.trim();
    if (!preset?.id || isUnchanged || isSaving) return;
    isSaving = true;
    failure = undefined;
    try {
      await presets.change(preset.id, { name: typed }, preset.version ?? 0);
      toast.show({ text: t('app.presets.renamed'), tone: 'success' });
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as TransportError, messages).message;
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog
  size="s"
  title={t('app.presets.rename_title', { name: preset?.name ?? '' })}
  bind:isOpen
  dismissLabel={t('app.presets.cancel')}
  {onClose}
  primary={{
    label: t('app.presets.rename'),
    isBusy: isSaving,
    busyLabel: t('app.presets.saving'),
    disabledReason: name.trim() === '' ? t('app.presets.name_required') : undefined,
    onclick: () => form?.requestSubmit(),
  }}
>
  <form bind:this={form} onsubmit={rename} data-rename-preset>
    {#if failure}<FailureNote problem={failure} />{/if}
    <Input label={t('app.presets.name')} maxlength={100} bind:value={name} isRequired />
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.presets.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  form { margin: 0; display: grid; gap: var(--sp-150); }
</style>
