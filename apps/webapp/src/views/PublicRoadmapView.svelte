<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A portfolio's roadmap opened by its public link, without signing in (UC-GOL-03 check 15). The
  // token is in the fragment and stays there. The visitor reads the roadmap as the link's maker
  // reads it, as a table of lanes, projects and bars in the visitor's own zone - read only, and no
  // row leads into the workspace. An unknown or dead link says only that there is no roadmap here.
  import { untrack } from 'svelte';
  import { Badge, Banner, Skeleton, Table, type Column } from '@hubtask/design-system/components';
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
  const columns = $derived<Column[]>([
    { id: 'what', label: t('app.roadmap.col_what') },
    { id: 'start', label: t('app.planning.col_start') },
    { id: 'finish', label: t('app.planning.col_finish') },
    { id: 'state', label: t('app.roadmap.col_state') },
  ]);
</script>

<main class="shared">
  <h1 class="title">{roadmap?.name ?? t('app.roadmap.title')}</h1>
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
      <Table label={t('app.roadmap.table')} {columns}>
        {#each rows as row (row.id)}
          <tr data-depth={String(row.depth)}>
            <th scope="row" class="what">
              {row.title}
              {#if row.level !== 'entry'}<span class="note">{t(`app.roadmap.level_${row.level}`)}</span>{/if}
            </th>
            <td>{row.kind === 'milestone' ? '—' : dayLabel(row.start)}</td>
            <td>{dayLabel(row.due)}</td>
            <td>
              {#if row.level === 'project' && row.status && row.status !== 'NONE'}
                <Badge tone={goalTone(row.status)}>{t(`app.goals.status_${row.status}`)}</Badge>
              {:else if row.isDone}
                {t('app.roadmap.done')}
              {:else if row.isCritical}
                {t('app.planning.critical')}
              {:else if row.kind === 'milestone'}
                {t('app.roadmap.milestone')}
              {/if}
            </td>
          </tr>
        {/each}
      </Table>
    {/if}
  {/if}
</main>

<style>
  .shared { display: flex; flex-direction: column; gap: var(--sp-150); margin: 0 auto; padding: var(--sp-300) var(--sp-150); min-width: 0; }
  .title { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  tr[data-depth='1'] .what { padding-inline-start: var(--sp-150); }
  tr[data-depth='2'] .what { padding-inline-start: var(--sp-300); }
</style>
