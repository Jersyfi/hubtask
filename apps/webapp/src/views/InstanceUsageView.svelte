<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Each workspace's use (user-experience.md §8.4): people, entries and stored bytes on the latest
  // day `GET /admin/usage` answers - counts, never content. Metering is off by default; while it
  // is, the screen says how to switch it on instead of showing the refusal as a failure.

  import { untrack } from 'svelte';

  import { EmptyState, LoadMore, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { usage } from '../lib/data/usage.svelte.ts';
  import { USAGE_COLUMNS, USAGE_DAYS, latestOf } from '../lib/instance/usage.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
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

  const columns = $derived([
    { id: 'workspace', label: t('app.instance.column_workspace') },
    ...USAGE_COLUMNS.map((name) => ({ id: name, label: t(`app.instance.usage_column.${name}`), align: 'end' as const })),
  ]);

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

<Stack gap="300">
  <PageHeader
    title={t('app.instance.usage')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={installationTrail({ id: 'screen', label: t('app.instance.usage') })}
  />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.usage_intro', { days: USAGE_DAYS })}</p>
      {#if usage.isMeteringOff}
        <!-- No act: the switch is the operator's, set where the process is configured. -->
        <EmptyState kind="unused" icon="gauge" title={t('app.instance.usage_off_title')} description={t('app.instance.usage_off_description')} />
      {:else if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <FailureNote problem={unreadable} />
      {:else if rows.length === 0}
        <EmptyState kind="settled" title={t('app.instance.usage_none')} />
      {:else}
        <Table isStacked label={t('app.instance.usage')} isLabelHidden {columns}>
          {#each rows as row (row.workspace_id)}
            {@const latest = latestOf(row)}
            <tr data-usage={row.workspace_id}>
              <td>
                {#if row.purged}
                  <span class="mono">{row.workspace_id.slice(0, 8)}…</span>
                  <span class="quiet-cell">{t('app.instance.usage_purged')}</span>
                {:else if slugs.get(row.workspace_id)}
                  <span class="strong">{slugs.get(row.workspace_id)}</span>
                {:else}
                  <span class="mono">{row.workspace_id.slice(0, 8)}…</span>
                {/if}
              </td>
              <td class="figure">{number(latest.accounts)}</td>
              <td class="figure">{number(latest.items)}</td>
              <td class="figure">{latest.media_bytes === undefined ? '' : formatBytes(latest.media_bytes, messages.locale)}</td>
            </tr>
          {/each}
        </Table>
        <LoadMore
          label={t('app.instance.usage_more')}
          busyLabel={t('app.instance.usage_loading_more')}
          hasMore={Boolean(usage.moreAfter)}
          isBusy={isLoadingMore}
          arrivedLabel={arrived}
          failedLabel={moreFailed}
          onLoadMore={() => void more()}
        />
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-cell { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .strong { display: block; font-weight: var(--fw-medium); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
  .figure { font-variant-numeric: tabular-nums; }
</style>
