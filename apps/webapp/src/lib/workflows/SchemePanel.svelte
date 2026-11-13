<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where a workflow runs, and putting it in force for a hub or a collection (UC-FLW-02 check 4):
  // one row per hub or collection and type, the collection's own row winning over its hub's. A
  // change is previewed first - how many entries move, how the board's columns change - and is
  // made only on *Use it*, which sends the confirmation the server asks for.

  import { untrack } from 'svelte';
  import { Button, IconButton, Inline, Select, Stack } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { BoardPreview, Workflow, WorkflowPreview, WorkflowSchemeChange } from '@hubtask/sync-engine';

  import { containers } from '../data/containers.svelte.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { statusLabel, workflows } from '../data/workflows.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { UNMAPPED, mappedColumns, statusPlaces } from '../entries/boardlevel.ts';

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

  /** A column by its status's name in this workflow, else its key. */
  const columnName = (key: string) => {
    const status = workflow.published.statuses.find((each) => each.key === key);
    return status ? statusLabel(status, t) : key;
  };

  let hub = $state('');
  let collection = $state('');
  let entryType = $state('');

  $effect(() => {
    const wanted = hub;
    if (!wanted) return;
    return untrack(() => containers.openLevel(wanted));
  });

  // A hub's own workflow runs only in that hub and its collections (UC-FLW-02 check 9).
  const hubOptions = $derived(
    containers.hubs.filter((each) => !workflow.hub_id || each.id === workflow.hub_id).map((each) => ({ value: each.id, label: each.name })),
  );
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
      const preview = await workflows.previewScheme(change);
      pending = { change, preview };
      placed = Object.fromEntries((preview.boards ?? []).map((board) => [board.collection_id, statusPlaces(board)]));
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

  /**
   * Where each status stands on each board the change reaches (build-plan §5): the server's
   * proposal, editable per status, sent as `columns` with the change.
   */
  let placed = $state<Record<string, Record<string, string>>>({});
  const columnsOf = (board: BoardPreview) => mappedColumns(board, placed[board.collection_id] ?? {});

  async function confirm() {
    if (!pending) return;
    isBusy = true;
    refusal = undefined;
    try {
      const boards = (pending.preview.boards ?? []).filter((board) => board.level_after === 'WORKFLOW');
      const columns = boards.flatMap(columnsOf);
      const result = await workflows.setScheme({ ...pending.change, ...(columns.length > 0 ? { columns } : {}) });
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
        <p class="quiet">{t('app.workflow_designer.columns_after')} {pending.preview.columns_after.map(columnName).join(' · ') || t('app.workflow_designer.no_status')}</p>
        {#each (pending.preview.boards ?? []).filter((board) => board.level_after === 'WORKFLOW') as board (board.collection_id)}
          {@const options = [...board.columns.map((column) => ({ value: column.bucket_id, label: column.name })), { value: UNMAPPED, label: t('app.board.unmapped') }]}
          <section class="board" data-board-mapping={board.collection_id} aria-label={t('app.workflow_designer.board_mapping', { name: placeName(board.collection_id) })}>
            <h4 class="board-name">{t('app.workflow_designer.board_mapping', { name: placeName(board.collection_id) })}</h4>
            <ul class="mapping">
              {#each Object.keys(placed[board.collection_id] ?? {}) as status (status)}
                <li>
                  <span class="status">{columnName(status)}</span>
                  <Select label={t('app.workflow_designer.status_column', { status: columnName(status) })} isLabelHidden {options}
                    bind:value={() => placed[board.collection_id]?.[status] ?? UNMAPPED, (chosen) => (placed = { ...placed, [board.collection_id]: { ...placed[board.collection_id], [status]: chosen ?? UNMAPPED } })} />
                </li>
              {/each}
            </ul>
            {#if board.moved > 0}<p class="quiet">{t('app.workflow_designer.board_moved', { count: board.moved })}</p>{/if}
          </section>
        {/each}
        <Inline gap="100">
          <Button tone="secondary" size="sm" onclick={() => (pending = undefined)}>{t('app.workflow_designer.cancel')}</Button>
          <Button tone="primary" size="sm" data-confirm-scheme isBusy={isBusy} onclick={confirm}>
            {pending.change.workflow ? t('app.workflow_designer.use_it') : t('app.workflow_designer.stop_it')}
          </Button>
        </Inline>
      </div>
    {/if}
    {#if refusal}<FailureNote problem={refusal} />{/if}
  </Stack>
</section>

<style>
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .board { display: grid; gap: var(--sp-100); padding-block: var(--sp-100); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .board-name { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .mapping { display: grid; gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .mapping li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: var(--sp-100); align-items: center; }
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
