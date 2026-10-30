<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A portfolio's *Roadmap* (M2, planning.md §2): a lane per hub, each project as the span the
  // portfolio plans it for, and its top-level entries as the bars its own plan schedules - drawn
  // as the timeline and, beside it, as a table, the keyboard and screen-reader path (P-13).
  //
  // **Read only.** A bar is moved on its project's *Plan* screen, where the dependencies and the
  // proposals are; the roadmap is the picture across teams, so every row is locked and a press
  // opens the hub, the project's plan or the entry.

  import { untrack } from 'svelte';
  import { Badge, Banner, Button, PageHeader, Select, Skeleton, Table, Timeline, type Column } from '@hubtask/design-system/components';
  import type { PortfolioRoadmap } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { shift, type Window } from '../lib/data/due.ts';
  import { engine } from '../lib/data/engine.ts';
  import { roadmapRows } from '../lib/data/roadmap.ts';
  import { anchorOf, gridOf, scaleOf, windowOf, type Scale } from '../lib/data/schedule.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { goalTone } from '../lib/goals/tone.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { firstWeekdayOf } from '../lib/i18n/week.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    portfolioId: string;
    onnavigate: (path: string) => void;
  }

  const { portfolioId, onnavigate }: Props = $props();

  const zone = $derived(actor.zone);
  const today = $derived(todayIn(zone));

  let roadmap = $state<PortfolioRoadmap | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = `/portfolios/${portfolioId}/roadmap?zone=${encodeURIComponent(zone)}`;
    return untrack(() => engine.subscribe<PortfolioRoadmap>({ path }, (next) => {
      if (next.status === 'ready') { roadmap = next.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  $effect(() => frame.entitle(roadmap ? t('app.roadmap.title_of', { portfolio: roadmap.name }) : t('app.roadmap.title')));
  $effect(() => untrack(() => workspace.open()));

  const rows = $derived(roadmap ? roadmapRows(roadmap, t('app.roadmap.unseen_hub')) : []);
  const hrefOf = $derived(new Map(rows.map((row) => [row.id, row.href])));
  const timelineRows = $derived(rows.map((row) => ({
    id: row.id, title: row.title, start: row.start, due: row.due, kind: row.kind, isCritical: row.isCritical,
    depth: row.depth, isLocked: true,
  })));

  const weekStart = $derived(firstWeekdayOf(actor.weekStart, messages.locale, manifest.supportedLocales, workspace.weekStartAbove));
  let scale = $state<Scale>('quarter');
  let shown = $state<Window | undefined>(undefined);
  let anchor = $state<string | undefined>(undefined);
  $effect(() => {
    if (anchor !== undefined || !roadmap) return;
    const dated = untrack(() => rows.flatMap((row) => [row.start, row.due]).filter((day): day is string => !!day));
    anchor = anchorOf(dated, today, untrack(() => scale), weekStart);
  });
  const current = $derived(shown ?? windowOf(anchor ?? today, scale, weekStart));
  const gridlines = $derived.by(() => {
    const format = new Intl.DateTimeFormat(messages.locale, scale === 'year' ? { year: 'numeric', timeZone: 'UTC' } : { month: 'short', year: '2-digit', timeZone: 'UTC' });
    return gridOf(current, scale, weekStart).map((at) => ({ at, label: format.format(Date.parse(`${at}T00:00:00Z`)) }));
  });
  const dayLabel = (day: string | null | undefined) =>
    day ? new Intl.DateTimeFormat(messages.locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(Date.parse(`${day}T00:00:00Z`)) : '—';

  function move(direction: 1 | -1) {
    shown = shift(current, direction);
    announcer.say(t('app.timeline.moved', { from: current.from, to: current.to }));
  }

  const columns = $derived<Column[]>([
    { id: 'what', label: t('app.roadmap.col_what') },
    { id: 'start', label: t('app.planning.col_start') },
    { id: 'finish', label: t('app.planning.col_finish') },
    { id: 'state', label: t('app.roadmap.col_state') },
  ]);
  const portfolioPath = $derived(`/portfolios/${portfolioId}`);
</script>

<div class="screen">
  <PageHeader
    title={t('app.roadmap.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'portfolios', label: t('app.goals.tab_portfolios'), href: '/goals?tab=portfolios' },
        { id: 'portfolio', label: roadmap?.name ?? '', href: portfolioPath },
        { id: 'roadmap', label: t('app.roadmap.title') },
      ],
      label: t('app.goals.trail'),
      expandLabel: t('app.goals.expand_trail'),
      onnavigate: (href?: string) => onnavigate(href ?? portfolioPath),
    }}
  />

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !roadmap}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <p class="quiet">
      {roadmap.start && roadmap.finish ? t('app.roadmap.reach', { from: dayLabel(roadmap.start), to: dayLabel(roadmap.finish) }) : t('app.roadmap.no_dates')}
    </p>
    <div class="controls" role="group" aria-label={t('app.roadmap.chart')}>
      <Button size="sm" tone="secondary" icon="chevron-left" onclick={() => move(-1)}>{t('app.timeline.previous')}</Button>
      <Button size="sm" tone="secondary" onclick={() => { anchor = today; shown = undefined; }}>{t('app.timeline.today')}</Button>
      <Button size="sm" tone="secondary" icon="chevron-right" onclick={() => move(1)}>{t('app.timeline.next')}</Button>
      <Select
        label={t('app.planning.zoom')}
        size="sm"
        value={scale}
        options={(['month', 'quarter', 'year'] as const).map((value) => ({ value, label: t(`app.planning.scale_${value}`) }))}
        onchange={(event: Event) => { scale = scaleOf((event.currentTarget as HTMLSelectElement).value); shown = undefined; }}
      />
      <span class="quiet">{t('app.timeline.window', { from: current.from, to: current.to })}</span>
    </div>

    <div class="window">
      <Timeline
        label={t('app.roadmap.chart')}
        from={current.from}
        to={current.to}
        {scale}
        rows={timelineRows}
        {gridlines}
        {today}
        todayLabel={t('app.timeline.today_is', { date: dayLabel(today) })}
        undatedLabel={t('app.timeline.undated')}
        emptyLabel={t('app.roadmap.empty')}
        startHandleLabel={t('app.timeline.move_start')}
        dueHandleLabel={t('app.timeline.move_due')}
        onSelect={(id) => { const href = hrefOf.get(id); if (href) onnavigate(href); }}
      />
    </div>

    <section aria-labelledby="roadmap-table" class="block">
      <h2 id="roadmap-table" class="heading">{t('app.roadmap.table')}</h2>
      <Table label={t('app.roadmap.table')} isLabelHidden {columns}>
        {#each rows as row (row.id)}
          <tr data-depth={String(row.depth)}>
            <th scope="row" class="what">
              {#if row.href}<a href={row.href}>{row.title}</a>{:else}{row.title}{/if}
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
    </section>
  {/if}
</div>

<style>
  .screen { display: flex; flex-direction: column; gap: var(--sp-150); min-width: 0; }
  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .window { min-width: 0; }
  .block { display: flex; flex-direction: column; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .note { margin-inline-start: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); }
  tr[data-depth='1'] .what { padding-inline-start: var(--sp-150); }
  tr[data-depth='2'] .what { padding-inline-start: var(--sp-300); }
</style>
