<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The time tolerance the workspace decides for every plan (configuration.md §10): its days, where
  // they come from and who locked them, and - for whoever may shape the workspace - the days set,
  // locked for the hubs and collections, or left to the level above. A plan beyond it is told to
  // the person who set it.

  import { untrack } from 'svelte';
  import { Button, Checkbox, Inline, Input, SettingRow } from '@hubtask/design-system/components';
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
  {@const held = read}
  <!-- A row of the modules' section beside Planning, acting at once like the switches above it
       (page-system PS-10): *Set* and *Follow the level above* are its one-act buttons. -->
  <div class="tolerance" data-setting={KEY}>
    <SettingRow label={t('app.planning.workspace_tolerance')} help={t('app.planning.workspace_tolerance_hint')} fieldWidth="lg">
      {#snippet source()}
        <span class="state">
          {held.value === null ? t('app.planning.tolerance_none') : t('app.planning.tolerance_days', { days: String(held.value) })}
          {#if held.value !== null} · {t('app.planning.tolerance_from', { level: t(`app.planning.tolerance_level_${held.source}`) })}{/if}
          {#if held.locked_by} · {t('app.planning.tolerance_locked', { level: t(`app.planning.tolerance_level_${held.locked_by}`) })}{/if}
        </span>
      {/snippet}
      {#snippet control(nameId)}
        {#if mayShape && !lockedAbove}
          <Inline gap="100" align="end" isWrapping>
            <Input label={t('app.planning.tolerance')} aria-describedby={nameId} size="sm" type="number" min="0" max="365" bind:value={draft} />
            <Checkbox label={t('app.planning.tolerance_lock_workspace')} bind:checked={lock} />
            <Button size="sm" onclick={() => void save(false)}>{t('app.planning.tolerance_set')}</Button>
            <Button size="sm" tone="subtle" onclick={() => void save(true)}>{t('app.planning.tolerance_inherit')}</Button>
          </Inline>
        {/if}
        {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      {/snippet}
    </SettingRow>
  </div>
{/if}

<style>
  .tolerance { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .state { color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-regular); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
