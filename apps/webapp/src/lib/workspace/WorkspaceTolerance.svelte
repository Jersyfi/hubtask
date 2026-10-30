<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The time tolerance the workspace decides for every plan (standards.md P12): its days, where
  // they come from and who locked them, and - for whoever may shape the workspace - the days set,
  // locked for the hubs and collections, or left to the level above. A plan beyond it is told to
  // the person who set it.

  import { untrack } from 'svelte';
  import { Button, Checkbox, Inline, Input, Stack } from '@hubtask/design-system/components';
  import type { WorkspaceTolerance } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const PATH = '/plans:tolerance';
  const KEY = 'planning.time_tolerance_days';

  let read = $state<WorkspaceTolerance[typeof KEY] | undefined>(undefined);
  $effect(() => untrack(() => engine.subscribe<WorkspaceTolerance>({ path: PATH }, (next) => {
    if (next.status === 'ready') {
      read = next.data[KEY];
      draft = read.value === null ? '' : String(read.value);
      lock = read.locked_by === 'WORKSPACE';
    }
  })));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const lockedAbove = $derived(!!read?.locked_by && read.locked_by !== 'WORKSPACE');

  let draft = $state('');
  let lock = $state(false);
  let failure = $state<string | undefined>(undefined);
  async function save(inherit: boolean) {
    failure = undefined;
    const days = inherit || draft.trim() === '' ? null : Number(draft);
    if (days !== null && (!Number.isInteger(days) || days < 0 || days > 365)) {
      failure = t('app.planning.tolerance_range');
      return;
    }
    try {
      await engine.mutate<WorkspaceTolerance>('PUT', PATH, { time_tolerance_days: days, lock: days !== null && lock }, { invalidates: [PATH] });
      toast.show({ text: t('app.planning.tolerance_saved'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

{#if read}
  <section class="tolerance" aria-labelledby="workspace-tolerance">
    <Stack gap="100">
      <h2 id="workspace-tolerance" class="heading">{t('app.planning.workspace_tolerance')}</h2>
      <p class="quiet">{t('app.planning.workspace_tolerance_hint')}</p>
      <p class="quiet">
        {read.value === null ? t('app.planning.tolerance_none') : t('app.planning.tolerance_days', { days: String(read.value) })}
        {#if read.value !== null} · {t('app.planning.tolerance_from', { level: t(`app.planning.tolerance_level_${read.source}`) })}{/if}
        {#if read.locked_by} · {t('app.planning.tolerance_locked', { level: t(`app.planning.tolerance_level_${read.locked_by}`) })}{/if}
      </p>
      {#if mayShape && !lockedAbove}
        <Inline gap="100" align="end">
          <Input label={t('app.planning.tolerance')} size="sm" type="number" min="0" max="365" bind:value={draft} />
          <Checkbox label={t('app.planning.tolerance_lock_workspace')} bind:checked={lock} />
          <Button size="sm" onclick={() => void save(false)}>{t('app.planning.tolerance_set')}</Button>
          <Button size="sm" tone="subtle" onclick={() => void save(true)}>{t('app.planning.tolerance_inherit')}</Button>
        </Inline>
      {/if}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
    </Stack>
  </section>
{/if}

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
