<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Each workspace's use (user-experience.md §8.4): people, entries and stored bytes on the latest
  // day `GET /admin/usage` answers - counts, never content. Metering is off by default; while it
  // is, the screen says how to switch it on instead of showing the refusal as a failure. A log page
  // (page-system §7.6, §9.4): a workspace's row opens its latest day as a Dialog M.

  import { untrack } from 'svelte';

  import { Button, DataTable, EmptyState, FilterBar, LogPage, Spinner, type DataColumn } from '@hubtask/design-system/components';
  import type { WorkspaceUsage } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { usage } from '../lib/data/usage.svelte.ts';
  import { USAGE_COLUMNS, USAGE_DAYS, latestOf } from '../lib/instance/usage.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import LogRecordDialog from '../lib/instance/LogRecordDialog.svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    if (!instance.isElevated) return;
    const stops = untrack(() => [usage.open(), instance.openWorkspaces()]);
    return () => stops.forEach((stop) => stop());
  });

  const reading = $derived(usage.state);
  const rows = $derived(usage.rows);
  const unreadable = $derived(
    reading.status === 'failed' && !usage.isMeteringOff ? renderProblem(reading.error, messages) : undefined,
  );
  const slugs = $derived(new Map(instance.workspaces.map((workspace) => [workspace.id, workspace.slug])));

  const columns = $derived<DataColumn[]>([
    { id: 'workspace', label: t('app.instance.column_workspace'), priority: 1 },
    ...USAGE_COLUMNS.map((name) => ({ id: name, label: t(`app.instance.usage_column.${name}`), align: 'end' as const, priority: 2 as const })),
  ]);

  const nameOf = (row: WorkspaceUsage): string =>
    row.purged ? t('app.instance.usage_purged') : (slugs.get(row.workspace_id) ?? `${row.workspace_id.slice(0, 8)}…`);
  const valueOf = (row: WorkspaceUsage, name: (typeof USAGE_COLUMNS)[number]): string => {
    const value = latestOf(row)[name];
    return name === 'media_bytes' ? (value === undefined ? '' : formatBytes(value, messages.locale)) : number(value);
  };

  // The search narrows the workspaces loaded, by address.
  let search = $state('');
  const shown = $derived(rows.filter((row) => matchesSearch(search, [nameOf(row)])));

  let opened = $state<WorkspaceUsage | undefined>(undefined);
  const openedFacts = $derived(
    opened ? USAGE_COLUMNS.map((name) => ({ label: t(`app.instance.usage_column.${name}`), value: valueOf(opened!, name) })) : [],
  );

  const number = (value: number | undefined) =>
    value === undefined ? '' : new Intl.NumberFormat(messages.locale).format(value);

  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);
  async function more(): Promise<void> {
    const before = usage.rows.length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await usage.more();
      arrived = t('app.instance.usage_arrived', { count: usage.rows.length - before });
    } catch (error) {
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  $effect(() => page.entitle(t('app.instance.usage')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <LogPage head={{ title: t('app.instance.usage'), breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.usage') }) }} refused={door}>
      {#snippet intro()}<p class="quiet">{t('app.instance.usage_intro', { days: USAGE_DAYS })}</p>{/snippet}
      {#snippet filters()}
        {#if !usage.isMeteringOff}
          <FilterBar
            searchLabel={t('app.instance.usage_search')}
            clearSearchLabel={t('app.page.clear_search')}
            bind:search
            countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(rows.length) }) : undefined}
          />
        {/if}
      {/snippet}

      {#if usage.isMeteringOff}
        <!-- No act: the switch is the operator's, set where the process is configured. -->
        <EmptyState kind="unused" icon="gauge" title={t('app.instance.usage_off_title')} description={t('app.instance.usage_off_description')} />
      {:else if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else if unreadable}
        <FailureNote problem={unreadable} />
      {:else}
        <DataTable
          label={t('app.instance.usage')}
          {columns}
          rows={shown}
          key={(row: WorkspaceUsage) => row.workspace_id}
          onopen={(row: WorkspaceUsage) => (opened = row)}
          rowLabel={nameOf}
          shownLabel={rows.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
          more={usage.moreAfter || arrived || moreFailed
            ? {
                label: t('app.instance.usage_more'),
                busyLabel: t('app.instance.usage_loading_more'),
                hasMore: Boolean(usage.moreAfter),
                isBusy: isLoadingMore,
                arrivedLabel: arrived,
                failedLabel: moreFailed,
                onLoadMore: () => void more(),
              }
            : undefined}
        >
          {#snippet cell(row: WorkspaceUsage, column: DataColumn)}
            {#if column.id === 'workspace'}
              <span class="strong" data-usage={row.workspace_id}>{nameOf(row)}</span>
            {:else}
              <span class="figure">{valueOf(row, column.id as (typeof USAGE_COLUMNS)[number])}</span>
            {/if}
          {/snippet}
          {#snippet empty()}
            {#if rows.length === 0}
              <EmptyState kind="settled" title={t('app.instance.usage_none')} />
            {:else}
              <EmptyState kind="filtered" title={t('app.instance.workspaces_none_found')}>
                {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
              </EmptyState>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </LogPage>
  {/snippet}
</InstanceGate>

<LogRecordDialog
  isOpen={opened !== undefined}
  title={opened ? nameOf(opened) : ''}
  facts={openedFacts}
  support={opened ? [{ label: t('app.instance.journal_support_workspace'), value: opened.workspace_id }] : []}
  onClose={() => (opened = undefined)}
/>

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .strong { display: block; font-weight: var(--fw-medium); }
  .figure { font-variant-numeric: tabular-nums; }
</style>
