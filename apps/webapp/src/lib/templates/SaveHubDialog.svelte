<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Save from a hub* (page-system §9.1, Dialog M): a hub's structure in force saved under a name
  // as a preset of the workspace (UC-WRK-30 check 1) - the same write as a collection's *Save as
  // preset* for the workspace, started from the list of presets rather than from the hub.
  import { Dialog, Input, Select, Stack, Button } from '@hubtask/design-system/components';
  import type { TransportError } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { presets } from '../data/presets.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { isComplete, type Missing } from '../forms/missing.ts';
  import { marked } from '../forms/press.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    /** Ran once the preset is saved. */
    onsaved?: () => void;
  }

  let { isOpen = $bindable(), onsaved }: Props = $props();

  let hub = $state('');
  let name = $state('');
  let missing = $state<Missing>({});
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isSaving = $state(false);
  let form = $state<HTMLFormElement | undefined>(undefined);

  const hubs = $derived(containers.hubs.map((each) => ({ value: each.id, label: each.name })));

  function reset(): void {
    hub = '';
    name = '';
    missing = {};
    failure = undefined;
  }
  const guard = new DiscardGuard(() => hub !== '' || name.trim() !== '', () => {
    isOpen = false;
    reset();
  });

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const typed = name.trim();
    missing = marked(event, [
      { kind: 'field', key: 'hub', code: 'app.forms.required', isFilled: hub !== '' },
      { kind: 'field', key: 'name', code: 'app.presets.name_required', isFilled: typed !== '' },
    ]);
    if (!isComplete(missing) || isSaving) return;
    isSaving = true;
    failure = undefined;
    try {
      await presets.save({ name: typed, origin: 'WORKSPACE', container_id: hub });
      announcer.say(t('app.presets.saved'));
      toast.show({ text: t('app.presets.saved'), tone: 'success' });
      isOpen = false;
      reset();
      onsaved?.();
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.templates.save_hub_title')}
  bind:isOpen
  dismissLabel={t('app.presets.cancel')}
  onDismiss={guard.dismiss}
  onClose={reset}
  primary={hubs.length === 0 ? undefined : { label: t('app.presets.save'), isBusy: isSaving, busyLabel: t('app.presets.saving'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={save} data-save-hub>
    <Stack gap="150">
      <p class="quiet">{t('app.templates.save_hub_about')}</p>
      {#if failure && !failure.fields.has('/name')}<FailureNote problem={failure} />{/if}
      {#if hubs.length === 0}
        <p class="quiet">{t('app.templates.save_hub_none')}</p>
      {:else}
        <Select label={t('app.templates.save_hub_hub')} placeholder={t('app.templates.save_hub_choose')} options={hubs} bind:value={hub}
          isRequired error={missing.hub && hub === '' ? t(missing.hub) : undefined} />
        <Input label={t('app.presets.name')} maxlength={100} bind:value={name} isRequired
          error={missing.name && name.trim() === '' ? t(missing.name) : failure?.fields.has('/name') ? failure.message : undefined} />
      {/if}
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) { isOpen = false; reset(); } }}>{t('app.presets.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  form { margin: 0; }
</style>
