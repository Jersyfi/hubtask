<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A portfolio's roadmap opened by its public link, without signing in (UC-GOL-03 check 15). The
  // token is in the fragment and stays there. The visitor reads the roadmap as the link's maker
  // reads it, as a table of lanes, projects and bars in the visitor's own zone - read only, and no
  // row leads into the workspace. An unknown or dead link says only that there is no roadmap here.
  import { untrack } from 'svelte';
  import { Badge, Banner, DataTable, Skeleton, StandalonePage, type DataColumn } from '@hubtask/design-system/components';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import type { PortfolioRoadmap } from '@hubtask/sync-engine';

  import { roadmapRows } from '../lib/data/roadmap.ts';
  import { outsiderRows, roadmapTokenOf, sharedRoadmapPath } from '../lib/data/roadmaplink.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { goalTone } from '../lib/goals/tone.ts';
  import { formatPlainDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';

  const token = roadmapTokenOf(globalThis.location?.hash ?? '');
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  const road = token === '' ? undefined : untrack(() => resource<PortfolioRoadmap>(sharedRoadmapPath(token, zone)));
  const roadmap = $derived(road?.state.status === 'ready' ? road.state.data : undefined);
  const gone = $derived(token === '' || road?.state.status === 'failed');
  const rows = $derived(roadmap ? outsiderRows(roadmapRows(roadmap, t('app.roadmap.unseen_hub'))) : []);

  $effect(() => {
    document.title = roadmap ? t('app.roadmap.title_of', { portfolio: roadmap.name }) : t('app.roadmap.title');
  });

  const dayLabel = (day: string | null | undefined) => formatPlainDay(day, messages.locale);
  const columns = $derived<DataColumn[]>([
    { id: 'what', label: t('app.roadmap.col_what'), priority: 1 },
    { id: 'start', label: t('app.planning.col_start'), priority: 2 },
    { id: 'finish', label: t('app.planning.col_finish'), priority: 2 },
    { id: 'state', label: t('app.roadmap.col_state'), priority: 1, isStatus: true },
  ]);
  type Row = (typeof rows)[number];
</script>

<StandalonePage variant="public" title={roadmap?.name ?? t('app.roadmap.title')}>
  {#if gone}
    <Banner tone="warning">{t('app.roadmap.shared_gone')}</Banner>
  {:else if !roadmap}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <p class="quiet">
      {roadmap.start && roadmap.finish ? t('app.roadmap.reach', { from: dayLabel(roadmap.start), to: dayLabel(roadmap.finish) }) : t('app.roadmap.no_dates')}
    </p>
    <p class="quiet">{t('app.roadmap.shared_note')}</p>
    {#if rows.length === 0}
      <ProductEmpty product="GOALS" kind="unused" title={t('app.roadmap.empty')} />
    {:else}
      <DataTable label={t('app.roadmap.table')} {columns} {rows} key={(row: Row) => row.id}>
        {#snippet cell(row: Row, column: DataColumn)}
          {#if column.id === 'what'}
            <span class="what" data-depth={String(row.depth)}>
              {row.title}
              {#if row.level !== 'entry'}<span class="note">{t(`app.roadmap.level_${row.level}`)}</span>{/if}
            </span>
          {:else if column.id === 'start'}{row.kind === 'milestone' ? '—' : dayLabel(row.start)}
          {:else if column.id === 'finish'}{dayLabel(row.due)}
          {:else if row.level === 'project' && row.status && row.status !== 'NONE'}
            <Badge tone={goalTone(row.status)}>{t(`app.goals.status_${row.status}`)}</Badge>
          {:else if row.isDone}
            {t('app.roadmap.done')}
          {:else if row.isCritical}
            {t('app.planning.critical')}
          {:else if row.kind === 'milestone'}
            {t('app.roadmap.milestone')}
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  {/if}
</StandalonePage>

<style>
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  .what[data-depth='1'] { padding-inline-start: var(--sp-150); }
  .what[data-depth='2'] { padding-inline-start: var(--sp-300); }
</style>
