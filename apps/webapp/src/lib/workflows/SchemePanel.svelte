<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where a workflow runs, and putting it in force for a hub or a collection (UC-FLW-02 check 4):
  // one row per hub or collection and type, the collection's own row winning over its hub's. A
  // change is previewed first - how many entries move, how the board's columns change - and is
  // made only on *Use it*, which sends the confirmation the server asks for.

  import { untrack } from 'svelte';
  import { Banner, Button, IconButton, Inline, Select, Stack } from '@hubtask/design-system/components';
  import type { Workflow, WorkflowPreview, WorkflowSchemeChange } from '@hubtask/sync-engine';

  import { containers } from '../data/containers.svelte.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { workflows } from '../data/workflows.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    workflow: Workflow;
  }

  const { workflow }: Props = $props();

  $effect(() => untrack(() => entryTypes.open()));

  const rows = $derived((workflows.list?.scheme ?? []).filter((row) => row.workflow === workflow.key));

  const placeName = (id: string) => containers.find(id)?.name ?? t('app.workflow_designer.unknown_place');
  const typeName = (key: string | null | undefined) => {
    if (!key) return t('app.workflow_designer.every_type');
    const type = entryTypes.all.find((each) => each.key === key);
    return type?.name ?? key;
  };

  let hub = $state('');
  let collection = $state('');
  let entryType = $state('');

  $effect(() => {
    const wanted = hub;
    if (!wanted) return;
    return untrack(() => containers.openLevel(wanted));
  });

  const hubOptions = $derived(containers.hubs.map((each) => ({ value: each.id, label: each.name })));
  const collectionOptions = $derived([
    { value: '', label: t('app.workflow_designer.whole_hub') },
    ...(hub ? containers.collectionsOf(hub) : []).map((each) => ({ value: each.id, label: each.name })),
  ]);
  const typeOptions = $derived([
    { value: '', label: t('app.workflow_designer.every_type') },
    ...entryTypes.all.filter((each) => !each.archived && each.workflow).map((each) => ({ value: each.key, label: each.name ?? each.key })),
  ]);

  let pending = $state<{ change: WorkflowSchemeChange; preview: WorkflowPreview } | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let isBusy = $state(false);

  async function previewOf(change: WorkflowSchemeChange) {
    isBusy = true;
    refusal = undefined;
    try {
      pending = { change, preview: await workflows.previewScheme(change) };
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }

  function propose() {
    const container = collection || hub;
    if (!container) return;
    void previewOf({ container_id: container, workflow: workflow.key, ...(entryType ? { entry_type: entryType } : {}) });
  }

  function proposeRemoval(row: { container_id: string; entry_type?: string | null }) {
    void previewOf({ container_id: row.container_id, ...(row.entry_type ? { entry_type: row.entry_type } : {}) });
  }

  async function confirm() {
    if (!pending) return;
    isBusy = true;
    refusal = undefined;
    try {
      const result = await workflows.setScheme(pending.change);
      toast.show({ text: t('app.workflow_designer.scheme_done', { count: result.entries_moved }), tone: 'success' });
      pending = undefined;
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }
</script>

<section class="panel" aria-labelledby="workflow-scheme" data-scheme>
  <Stack gap="150">
    <h3 id="workflow-scheme" class="title">{t('app.workflow_designer.used_in')}</h3>
    {#if rows.length === 0}
      <p class="quiet">{t('app.workflow_designer.used_nowhere')}</p>
    {:else}
      <ul class="rows">
        {#each rows as row (`${row.container_id}:${row.entry_type ?? ''}`)}
          <li class="row" data-scheme-row={row.container_id}>
            <span>{placeName(row.container_id)} · {typeName(row.entry_type)}</span>
            <IconButton
              icon="x" size="sm" tone="subtle"
              label={t('app.workflow_designer.stop_using', { place: placeName(row.container_id) })}
              onclick={() => proposeRemoval(row)}
            />
          </li>
        {/each}
      </ul>
    {/if}
    <div class="add">
      <Select label={t('app.workflow_designer.hub')} options={hubOptions} placeholder={t('app.workflow_designer.choose_hub')} bind:value={hub} />
      <Select label={t('app.workflow_designer.collection')} options={collectionOptions} bind:value={collection} />
      <Select label={t('app.workflow_designer.entry_type')} options={typeOptions} bind:value={entryType} />
      <Button tone="secondary" size="sm" data-propose-scheme isBusy={isBusy}
        disabledReason={hub ? undefined : t('app.workflow_designer.choose_hub')} onclick={propose}>
        {t('app.workflow_designer.preview_use')}
      </Button>
    </div>
    {#if pending}
      <div class="pending" data-scheme-preview>
        <p>{t('app.workflow_designer.publish_entries', { count: pending.preview.entries })}</p>
        <p class="quiet">{t('app.workflow_designer.columns_after')} {pending.preview.columns_after.join(' · ') || t('app.workflow_designer.no_status')}</p>
        <Inline gap="100">
          <Button tone="secondary" size="sm" onclick={() => (pending = undefined)}>{t('app.workflow_designer.cancel')}</Button>
          <Button tone="primary" size="sm" data-confirm-scheme isBusy={isBusy} onclick={confirm}>
            {pending.change.workflow ? t('app.workflow_designer.use_it') : t('app.workflow_designer.stop_it')}
          </Button>
        </Inline>
      </div>
    {/if}
    {#if refusal}<Banner tone="danger" title={refusal} />{/if}
  </Stack>
</section>

<style>
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .title { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); }
  p { margin: 0; }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .add {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 24ch), 1fr));
    gap: var(--sp-100);
    align-items: end;
  }
  .pending {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
  }
</style>
