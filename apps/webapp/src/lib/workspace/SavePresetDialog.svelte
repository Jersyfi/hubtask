<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Save as preset* (UC-WRK-30 check 1): the structure in force on this hub or collection, saved
  // under a name for the reader alone or - with the right to change the workspace's shape - for
  // the workspace; or copied into a preset saved before, which keeps its name unless a new one is
  // typed. What is saved is what the server holds, not the panel's unsaved draft: the panel says so.
  // The workspace is not offered to a reader who may not shape it (P-05).

  import { untrack } from 'svelte';

  import { Button, Dialog, Input, Radio, Stack } from '@hubtask/design-system/components';
  import type { TransportError } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { presets } from '../data/presets.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { focusFirst } from '../forms/missing.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    /** The hub or collection whose structure in force is saved. */
    containerId: string;
    /** Whether the reader may change the workspace's shape: only then is the workspace offered. */
    mayShapeWorkspace: boolean;
  }

  let { isOpen = $bindable(), containerId, mayShapeWorkspace }: Props = $props();

  /** `OWN`, `WORKSPACE`, or the id of a saved preset to copy into. */
  let target = $state('OWN');
  let name = $state('');
  let isSaving = $state(false);
  let isNameMissing = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let body = $state<HTMLElement | null>(null);

  $effect(() => {
    if (!isOpen) return;
    return untrack(() => presets.open());
  });
  // Reopening starts a new answer.
  $effect(() => {
    if (isOpen) return;
    target = 'OWN';
    name = '';
    isNameMissing = false;
    failure = undefined;
  });

  /** The saved presets the reader may copy into: their own, and the workspace's where they may. */
  const replaceable = $derived(
    presets.all.filter((preset) => preset.origin === 'OWN' || (preset.origin === 'WORKSPACE' && mayShapeWorkspace)),
  );
  const targetOptions = $derived([
    { value: 'OWN', label: t('app.presets.for_me'), hint: t('app.presets.for_me_about') },
    ...(mayShapeWorkspace ? [{ value: 'WORKSPACE', label: t('app.presets.for_workspace'), hint: t('app.presets.for_workspace_about') }] : []),
    ...replaceable.map((preset) => ({
      value: preset.id ?? '',
      label: t('app.presets.replace', { name: preset.name ?? '' }),
      hint: t(`app.structure.preset_origin.${preset.origin}`),
    })),
  ]);
  const isReplacing = $derived(target !== 'OWN' && target !== 'WORKSPACE');

  async function save() {
    if (isSaving) return;
    const typed = name.trim();
    if (!isReplacing && typed === '') {
      isNameMissing = true;
      queueMicrotask(() => focusFirst(body));
      return;
    }
    isSaving = true;
    failure = undefined;
    try {
      if (isReplacing) {
        const replaced = presets.find(target);
        if (!replaced) return;
        await presets.change(target, { container_id: containerId, ...(typed === '' ? {} : { name: typed }) }, replaced.version ?? 0);
      } else {
        await presets.save({ name: typed, origin: target as 'OWN' | 'WORKSPACE', container_id: containerId });
      }
      announcer.say(t('app.presets.saved'));
      toast.show({ text: t('app.presets.saved') });
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog bind:isOpen title={t('app.presets.save_title')} dismissLabel={t('app.workspace.cancel')}>
  <div bind:this={body} data-save-preset>
    <Stack gap="150">
      <p class="hint">{t('app.presets.save_about')}</p>
      <Radio label={t('app.presets.save_where')} options={targetOptions} bind:value={target} />
      <Input
        label={t(isReplacing ? 'app.presets.new_name_optional' : 'app.presets.name')}
        maxlength={100}
        bind:value={name}
        oninput={() => (isNameMissing = false)}
        error={isNameMissing ? t('app.presets.name_required') : failure?.fields.has('/name') ? failure.message : undefined}
      />
      {#if failure && !failure.fields.has('/name')}
        <p class="failure" role="alert">{failure.message}</p>
      {/if}
    </Stack>
  </div>

  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
    <Button isBusy={isSaving} busyLabel={t('app.presets.saving')} onclick={save}>{t('app.presets.save')}</Button>
  {/snippet}
</Dialog>

<style>
  .hint { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
