<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One dashboard (UC-GOL-04 check 8): its widgets on the twelve-column grid, each drawn with the
  // design system's colours and a table beside its chart, computed now for this reader. A widget
  // the reader cannot run says why in place, and the others still draw (check 4).
  //
  // **Editing** happens on the grid: a widget is added, changed, moved and resized by whole cells
  // with buttons - which a keyboard and a screen reader can press - and the dashboard is saved
  // whole, held to the version read. A phone stacks the widgets in reading order.
  //
  // **No figure is stored or computed here** (check 2): the figures are read again after a save,
  // and a widget's table is handed over as a CSV file of the rows it shows.
  //
  // **By mail** (check 9): every reader may have the dashboard sent to them, daily or on one
  // weekday at an hour of their zone; the server computes the mail as them when it is sent.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Dialog, IconButton, Inline, Input, PageHeader, Select, Skeleton, Stack,
  } from '@hubtask/design-system/components';
  import type { Dashboard, DashboardDigest, DashboardFigures, DashboardWidgetWrite, GoalList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import {
    MAX_WIDGETS, csvOf, dashboardPath, digestPath, figureRows, figuresPath, goals, moved, nextBox, readingOrder, resized, widgetWrite,
  } from '../lib/data/goals.svelte.ts';
  import { views } from '../lib/data/views.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import WidgetChart from '../lib/goals/WidgetChart.svelte';
  import WidgetEditor from '../lib/goals/WidgetEditor.svelte';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { platform } from '../lib/platform/index.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    dashboardId: string;
    onnavigate: (path: string) => void;
  }

  const { dashboardId, onnavigate }: Props = $props();

  let dashboard = $state<Dashboard | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = dashboardPath(dashboardId);
    return untrack(() => engine.subscribe<Dashboard>({ path }, (next) => {
      if (next.status === 'ready') { dashboard = next.data; refusal = undefined; }
      if (next.status === 'failed') refusal = renderProblem(next.error, messages).message;
    }));
  });
  let figures = $state<DashboardFigures | undefined>(undefined);
  let figuresRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    const path = figuresPath(dashboardId);
    return untrack(() => engine.subscribe<DashboardFigures>({ path }, (next) => {
      if (next.status === 'ready') { figures = next.data; figuresRefusal = undefined; }
      if (next.status === 'failed') figuresRefusal = renderProblem(next.error, messages).message;
    }));
  });
  $effect(() => frame.entitle(dashboard ? dashboard.name : t('app.dashboards.dashboard')));

  /** A private dashboard is its owner's; a shared one is changed by whoever the server lets. */
  const mayEdit = $derived(dashboard !== undefined && (dashboard.sharing === 'SCOPE' || dashboard.owner_id === actor.account?.id));
  const placeName = $derived(dashboard?.hub_id ? (containers.find(dashboard.hub_id)?.name ?? t('app.goals.hub_unread')) : t('app.goals.workspace'));
  const locale = $derived(messages.locale);

  let failure = $state<string | undefined>(undefined);
  async function act(run: () => Promise<unknown>, done?: string): Promise<boolean> {
    failure = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    }
  }

  // --- Editing on the grid ----------------------------------------------------------------------

  let isEditing = $state(false);
  let layout = $state<DashboardWidgetWrite[]>([]);
  function startEditing() {
    if (!dashboard) return;
    layout = dashboard.widgets.map(widgetWrite);
    isEditing = true;
  }
  const boxOf = (each: DashboardWidgetWrite) => ({ x: each.x ?? 0, y: each.y ?? 0, w: each.w ?? 6, h: each.h ?? 2 });
  const shown = $derived<DashboardWidgetWrite[]>(isEditing ? layout : (dashboard?.widgets ?? []).map(widgetWrite));
  const ordered = $derived(readingOrder(shown.map((widget, index) => ({ ...boxOf(widget), widget, index }))));
  function place(index: number, next: { x: number; y: number; w: number; h: number }) {
    layout = layout.map((each, i) => (i === index ? { ...each, ...next } : each));
  }
  async function saveLayout() {
    const at = dashboard;
    if (!at) return;
    const ok = await act(() => goals.updateDashboard(at, { name: at.name, sharing: at.sharing, widgets: layout }), t('app.dashboards.saved'));
    if (ok) isEditing = false;
  }

  // The widget dialog: a new one (`editingIndex` -1) or a change.
  let isWidgetOpen = $state(false);
  let editingIndex = $state(-1);
  $effect(() => {
    if (!isWidgetOpen) return;
    const hub = dashboard?.hub_id ?? undefined;
    return untrack(() => {
      const closers = [views.open()];
      if (hub) closers.push(views.open(hub));
      return () => closers.forEach((close) => close());
    });
  });
  const summaryViews = $derived(isWidgetOpen
    ? [...views.of(), ...(dashboard?.hub_id ? views.of(dashboard.hub_id) : [])].filter((each, i, all) => each.query && 'aggregate' in each.query && all.findIndex((other) => other.id === each.id) === i)
    : []);
  let goalChoices = $state<GoalList['data']>([]);
  $effect(() => {
    if (!isWidgetOpen) return;
    return untrack(() => engine.subscribe<GoalList>({ path: '/goals' }, (next) => { if (next.status === 'ready') goalChoices = next.data.data; }));
  });
  function openWidget(index: number) {
    editingIndex = index;
    isWidgetOpen = true;
  }
  function saveWidget(write: DashboardWidgetWrite) {
    if (editingIndex < 0) {
      layout = [...layout, { ...write, ...nextBox(layout.map(boxOf)) }];
    } else {
      layout = layout.map((each, i) => (i === editingIndex ? { ...write, ...boxOf(each) } : each));
    }
    isWidgetOpen = false;
  }

  // --- Name and sharing, deleting ---------------------------------------------------------------

  let isRenaming = $state(false);
  const draft = $state({ name: '', sharing: 'PRIVATE' as 'PRIVATE' | 'SCOPE' });
  async function saveName() {
    const at = dashboard;
    if (!at) return;
    const ok = await act(() => goals.updateDashboard(at, { name: draft.name.trim(), sharing: draft.sharing, widgets: at.widgets.map(widgetWrite) }), t('app.dashboards.saved'));
    if (ok) isRenaming = false;
  }
  let isDeleting = $state(false);
  async function remove() {
    const at = dashboard;
    if (!at) return;
    const ok = await act(() => goals.removeDashboard(at), t('app.dashboards.deleted', { name: at.name }));
    if (ok) { isDeleting = false; onnavigate('/goals?tab=dashboards'); }
  }

  // --- By mail ----------------------------------------------------------------------------------

  let isDigestOpen = $state(false);
  let digest = $state<DashboardDigest | undefined>(undefined);
  const digestDraft = $state({ cadence: 'WEEKLY' as 'DAILY' | 'WEEKLY', weekday: '1', hour: '8' });
  $effect(() => {
    if (!isDigestOpen) return;
    const path = digestPath(dashboardId);
    return untrack(() => engine.subscribe<DashboardDigest>({ path }, (next) => {
      if (next.status === 'ready') {
        digest = next.data;
        digestDraft.cadence = next.data.cadence;
        digestDraft.weekday = String(next.data.weekday ?? 1);
        digestDraft.hour = String(next.data.hour);
      }
      // Not found is a reader without a digest: the form starts empty.
      if (next.status === 'failed') digest = undefined;
    }));
  });
  const weekdays = $derived(Array.from({ length: 7 }, (_, i) => ({
    value: String(i + 1),
    // 5 January 2026 is a Monday.
    label: new Intl.DateTimeFormat(locale, { weekday: 'long', timeZone: 'UTC' }).format(new Date(Date.UTC(2026, 0, 5 + i))),
  })));
  const hours = $derived(Array.from({ length: 24 }, (_, h) => ({
    value: String(h),
    label: new Intl.DateTimeFormat(locale, { hour: 'numeric', minute: '2-digit', timeZone: 'UTC' }).format(new Date(Date.UTC(2026, 0, 5, h))),
  })));
  async function saveDigest() {
    const ok = await act(() => goals.setDigest(dashboardId, {
      cadence: digestDraft.cadence,
      hour: Number(digestDraft.hour),
      ...(digestDraft.cadence === 'WEEKLY' ? { weekday: Number(digestDraft.weekday) } : {}),
      time_zone: actor.zone,
    }), t('app.dashboards.digest_saved'));
    if (ok) isDigestOpen = false;
  }
  async function stopDigest() {
    const ok = await act(() => goals.removeDigest(dashboardId), t('app.dashboards.digest_stopped'));
    if (ok) { digest = undefined; isDigestOpen = false; }
  }

  // --- Figures ----------------------------------------------------------------------------------

  const figureOf = (id: string | undefined) => (id ? figures?.widgets.find((each) => each.id === id) : undefined);
  function exportWidget(title: string, id: string | undefined) {
    const rows = figureRows(figureOf(id)?.data ?? []);
    const csv = csvOf([t('app.dashboards.col_group'), t('app.dashboards.col_period'), t('app.dashboards.col_value')], rows.map((row) => [row.label, row.column ?? '', row.text]));
    platform.saveFile(new Blob([csv], { type: 'text/csv;charset=utf-8' }), `${title.replace(/[^\p{L}\p{N}]+/gu, '-').slice(0, 60) || 'widget'}.csv`);
    toast.show({ text: t('app.dashboards.exported', { title }), tone: 'success' });
  }
</script>

<Stack gap="300">
  <PageHeader
    title={dashboard?.name ?? t('app.dashboards.dashboard')}
    subtitle={dashboard ? t(`app.dashboards.subtitle_${dashboard.sharing}`, { place: placeName }) : undefined}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [{ id: 'dashboards', label: t('app.goals.tab_dashboards'), href: '/goals?tab=dashboards' }, { id: 'dashboard', label: dashboard?.name ?? '' }],
      label: t('app.goals.trail'),
      expandLabel: t('app.goals.expand_trail'),
      onnavigate: () => onnavigate('/goals?tab=dashboards'),
    }}
    secondary={mayEdit
      ? isEditing
        ? [{ label: t('app.dashboards.save_layout'), icon: 'check', onclick: () => void saveLayout() }, { label: t('app.dashboards.add_widget'), icon: 'plus', onclick: () => openWidget(-1), ...(layout.length >= MAX_WIDGETS ? { disabledReason: t('dashboards.too_many_widgets') } : {}) }]
        : [{ label: t('app.dashboards.edit'), icon: 'pencil', onclick: startEditing }]
      : []}
    menu={dashboard ? {
      label: t('app.goals.more'),
      items: [
        { id: 'digest', label: t('app.dashboards.digest'), icon: 'mail' as const },
        ...(mayEdit ? [
          ...(isEditing ? [{ id: 'cancel', label: t('app.dashboards.discard'), icon: 'x' as const }] : []),
          { id: 'rename', label: t('app.dashboards.rename'), icon: 'pencil' as const },
          { id: 'delete', label: t('app.dashboards.delete'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true },
        ] : []),
      ],
      onselect: (id: string) => {
        if (id === 'digest') isDigestOpen = true;
        if (id === 'cancel') isEditing = false;
        if (id === 'rename' && dashboard) { draft.name = dashboard.name; draft.sharing = dashboard.sharing; isRenaming = true; }
        if (id === 'delete') isDeleting = true;
      },
    } : undefined}
  />

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !dashboard}
    <div aria-busy="true"><Skeleton lines={8} /></div>
  {:else}
    {#if figures && !isEditing}<p class="quiet">{t('app.dashboards.computed', { when: formatDateTime(figures.computed_at, locale) })}</p>{/if}
    {#if figuresRefusal}<Banner tone="danger">{figuresRefusal}</Banner>{/if}
    {#if isEditing}<p class="quiet">{t('app.dashboards.editing_hint')}</p>{/if}

    {#if ordered.length === 0}
      <p class="quiet">{t('app.dashboards.no_widgets')}</p>
    {:else}
      <div class="grid" class:stacked={viewport.isCompact}>
        {#each ordered as each (each.widget.id ?? `new-${each.index}`)}
          {@const figure = figureOf(each.widget.id)}
          <article class="widget" style:--x={each.x + 1} style:--y={each.y + 1} style:--w={each.w} style:--h={each.h} aria-label={each.widget.title}>
            <header class="widget-head">
              <h2 class="heading">{each.widget.title}</h2>
              {#if !isEditing && figure && !figure.error}
                <IconButton icon="file-text" size="sm" tone="subtle" label={t('app.dashboards.export', { title: each.widget.title })} onclick={() => exportWidget(each.widget.title, each.widget.id)} />
              {/if}
            </header>
            {#if isEditing}
              {@const index = each.index}
              <p class="quiet">{t('app.dashboards.where', { x: String(each.x + 1), y: String(each.y + 1), w: String(each.w), h: String(each.h) })}</p>
              <Inline gap="050" isWrapping>
                <IconButton icon="arrow-left" size="sm" label={t('app.dashboards.move_left', { title: each.widget.title })} onclick={() => place(index, moved(each, -1, 0))} />
                <IconButton icon="arrow-right" size="sm" label={t('app.dashboards.move_right', { title: each.widget.title })} onclick={() => place(index, moved(each, 1, 0))} />
                <IconButton icon="chevron-up" size="sm" label={t('app.dashboards.move_up', { title: each.widget.title })} onclick={() => place(index, moved(each, 0, -1))} />
                <IconButton icon="chevron-down" size="sm" label={t('app.dashboards.move_down', { title: each.widget.title })} onclick={() => place(index, moved(each, 0, 1))} />
                <IconButton icon="minus" size="sm" label={t('app.dashboards.narrower', { title: each.widget.title })} onclick={() => place(index, resized(each, -1, 0))} />
                <IconButton icon="plus" size="sm" label={t('app.dashboards.wider', { title: each.widget.title })} onclick={() => place(index, resized(each, 1, 0))} />
                <IconButton icon="fold-vertical" size="sm" label={t('app.dashboards.shorter', { title: each.widget.title })} onclick={() => place(index, resized(each, 0, -1))} />
                <IconButton icon="chevrons-up-down" size="sm" label={t('app.dashboards.taller', { title: each.widget.title })} onclick={() => place(index, resized(each, 0, 1))} />
                <IconButton icon="pencil" size="sm" label={t('app.dashboards.change_widget', { title: each.widget.title })} onclick={() => openWidget(index)} />
                <IconButton icon="trash" size="sm" tone="danger" label={t('app.dashboards.remove_widget', { title: each.widget.title })} onclick={() => (layout = layout.filter((_, i) => i !== index))} />
              </Inline>
            {:else if !figures}
              <div aria-busy="true"><Skeleton lines={3} /></div>
            {:else if !figure}
              <p class="quiet">{t('app.dashboards.not_computed')}</p>
            {:else if figure.error}
              <p class="refused" role="status">{t(figure.error)}</p>
            {:else}
              <WidgetChart title={each.widget.title} chart={each.widget.chart} rows={figureRows(figure.data)} />
              {#if figure.truncated}<p class="quiet">{t('app.dashboards.truncated')}</p>{/if}
            {/if}
          </article>
        {/each}
      </div>
    {/if}
  {/if}
</Stack>

{#if isWidgetOpen}
  <Dialog bind:isOpen={isWidgetOpen} title={editingIndex < 0 ? t('app.dashboards.add_widget') : t('app.dashboards.change_widget', { title: layout[editingIndex]?.title ?? '' })} dismissLabel={t('app.links.cancel')}>
    <WidgetEditor widget={editingIndex < 0 ? undefined : layout[editingIndex]} {summaryViews} goals={goalChoices} onsave={saveWidget} />
  </Dialog>
{/if}

{#if isDigestOpen}
  <Dialog bind:isOpen={isDigestOpen} title={t('app.dashboards.digest_title')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.dashboards.digest_hint')}</p>
      <Select label={t('app.dashboards.digest_cadence')} bind:value={digestDraft.cadence}
        options={[{ value: 'DAILY', label: t('app.dashboards.digest_DAILY') }, { value: 'WEEKLY', label: t('app.dashboards.digest_WEEKLY') }]} />
      {#if digestDraft.cadence === 'WEEKLY'}
        <Select label={t('app.dashboards.digest_weekday')} bind:value={digestDraft.weekday} options={weekdays} />
      {/if}
      <Select label={t('app.dashboards.digest_hour')} hint={t('app.dashboards.digest_zone', { zone: actor.zone })} bind:value={digestDraft.hour} options={hours} />
      {#if digest}
        <p class="quiet">{t('app.dashboards.digest_next', { when: formatDateTime(digest.next_at, locale) })}</p>
      {/if}
      <Inline justify="end">
        {#if digest}<Button tone="danger" onclick={() => void stopDigest()}>{t('app.dashboards.digest_stop')}</Button>{/if}
        <Button tone="primary" onclick={() => void saveDigest()}>{t('app.dashboards.digest_save')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

{#if isRenaming}
  <Dialog bind:isOpen={isRenaming} title={t('app.dashboards.rename')} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <Input label={t('app.dashboards.col_name')} bind:value={draft.name} maxlength={200} />
      <Select label={t('app.dashboards.col_sharing')} hint={t('app.dashboards.sharing_hint')} bind:value={draft.sharing}
        options={[{ value: 'PRIVATE', label: t('app.dashboards.sharing_PRIVATE') }, { value: 'SCOPE', label: t('app.dashboards.sharing_SCOPE') }]} />
      <Inline justify="end"><Button disabledReason={draft.name.trim() === '' ? t('app.dashboards.name_needed') : undefined} onclick={() => void saveName()}>{t('app.goals.save')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

{#if isDeleting && dashboard}
  <Dialog bind:isOpen={isDeleting} title={t('app.dashboards.delete_title', { name: dashboard.name })} dismissLabel={t('app.links.cancel')}>
    <Stack gap="150">
      <p class="quiet">{t('app.dashboards.delete_hint')}</p>
      <Inline justify="end"><Button tone="danger" onclick={() => void remove()}>{t('app.dashboards.delete')}</Button></Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(12, minmax(0, 1fr));
    grid-auto-rows: minmax(var(--sp-1000), auto);
    gap: var(--sp-200);
  }
  .widget {
    grid-column: var(--x) / span var(--w);
    grid-row: var(--y) / span var(--h);
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-width: 0;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .grid.stacked { display: flex; flex-direction: column; }
  .widget-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .refused { margin: 0; color: var(--text-warning); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
