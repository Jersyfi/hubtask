<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The service desk's home (user-experience.md §5.1, §7.4): the trail, the title with its figure
  // (*12 open · 2 about to breach · 3 unassigned*), *New request* as the primary - capture opened on
  // the desk's collection - then *Queues · Desks · Settings*. The queues span every desk
  // collection of the hubs where the desk is on; the desks are listed with their hub; the settings
  // are a settings page of their own (ServiceDeskView, page-system §9.6).

  import { untrack } from 'svelte';
  import type { DeadlinesKept, HubQueueList, ResourceState } from '@hubtask/sync-engine';
  import { Button, Dialog, Icon, ListRow, ProductBand, Skeleton, SummaryCard, SummaryStrip, ViewBar, WorkPage, type IconName, type ViewLink } from '@hubtask/design-system/components';

  import { capture } from '../capture/capture.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { homeTabs } from '../product/home.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import ProductEmpty from '../product/ProductEmpty.svelte';
  import { markOf } from '../workspace/look.ts';
  import DeskQueues from './DeskQueues.svelte';
  import { groups } from '../data/groups.svelte.ts';
  import { KEPT_SHOWN, keptRows, percentOf, type KeptRow } from './kept.ts';
  import { desksAnswered, type DeskQueue } from './queues.ts';

  interface Props {
    /** The hubs where the desk is on, once each answered. */
    hubs: readonly { readonly id: string; readonly name: string }[];
    /** Whether the hubs or their modules are still being read. */
    isReading: boolean;
    tab?: string;
    /** The act that starts a hub, where the reader may; a desk is a template's collection in one. */
    createHub?: { label: string; onclick: () => void };
    /** The desk's templates while no desk exists (user-experience.md §4), each starting a hub from one. */
    templates?: readonly { label: string; onclick: () => void; description?: string; icon?: IconName }[];
    onnavigate: (path: string) => void;
  }
  const { hubs, isReading, tab, createHub, templates = [], onnavigate }: Props = $props();

  const name = $derived(t('app.nav.desk'));
  // The report is the workspace's, read by a holder of STRUCTURE there (P-05), as its section was.
  const homeViews = $derived<ViewLink[]>(homeTabs('SERVICE_DESK').filter((each) => each.id !== 'reports' || holdsHere('STRUCTURE') === true).map((each) => ({ id: each.id, label: t(each.code), href: each.path })));
  const current = $derived(homeViews.some((each) => each.id === tab) ? (tab as string) : 'foryou');

  const hubKey = $derived(hubs.map((hub) => hub.id).join(','));
  // Which collections are desks is the server's answer (`desks` of a hub's queues), not this
  // client's reading of the entry types (concept §10, "no desk takes requests").
  let queueAnswers = $state<Record<string, ResourceState<HubQueueList>>>({});
  $effect(() => {
    const ids = hubKey === '' ? [] : hubKey.split(',');
    return untrack(() => {
      const stops = [
        ...ids.map((hubId) => containers.openLevel(hubId)),
        ...ids.map((hubId) => engine.subscribe<HubQueueList>({ path: `/hubs/${hubId}/queues` }, (next) => {
          queueAnswers = { ...queueAnswers, [hubId]: next };
        })),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  const desksAt = $derived(
    !isReading && hubs.every((hub) => !containers.isLevelLoading(hub.id)) ? desksAnswered(queueAnswers, hubs.map((hub) => hub.id)) : undefined,
  );
  const desks = $derived(
    desksAt === undefined
      ? []
      : hubs.flatMap((hub) => containers.collectionsOf(hub.id).filter((collection) => desksAt[hub.id]?.includes(collection.id)).map((desk) => ({ desk, hub }))),
  );

  let totals = $state<Partial<Record<DeskQueue, number>>>({});
  const figures = $derived(
    totals.open === undefined
      ? []
      : [
          t('app.desk_home.open', { count: String(totals.open) }),
          ...(totals.breaching ? [t('app.desk_home.breaching', { count: String(totals.breaching) })] : []),
          ...(totals.unassigned ? [t('app.desk_home.unassigned', { count: String(totals.unassigned) })] : []),
        ],
  );

  // The handler's own deadlines kept over the last 30 days (UC-SVC-02 check 15): every kind, worst
  // first, three rows and "+ n more" for the whole table; groups named by `/groups`.
  let kept = $state<ResourceState<DeadlinesKept>>({ status: 'idle' });
  $effect(() => untrack(() => engine.subscribe<DeadlinesKept>({ path: '/deadlines-kept?days=30' }, (next) => (kept = next))));
  const keptAll = $derived(kept.status === 'ready' ? keptRows(kept.data.data) : []);
  const hasOla = $derived(keptAll.some((row) => row.kind === 'OLA'));
  $effect(() => {
    if (hasOla) return untrack(() => groups.open());
  });
  let isKeptOpen = $state(false);
  function keptLabel(row: KeptRow): string {
    const goal = t(`app.desk_home.goal_${row.goal.toLowerCase()}`);
    if (row.kind === 'SLA') return goal;
    const group = groups.all.find((each) => each.id === row.groupId)?.name ?? t('app.desk_home.group_unknown');
    return t('app.desk_home.ola_goal', { group, goal });
  }

  const primary = $derived.by(() => {
    const first = desks[0];
    if (!first) return createHub ? { label: createHub.label, icon: 'plus' as const, onclick: createHub.onclick } : undefined;
    return {
      label: t('app.desk_home.new_request'),
      icon: 'plus' as const,
      // A request is an entry of the desk, added by capture there (§5.2: the act creates).
      onclick: () => capture.open({ collectionId: first.desk.id }),
      menu: desks.length > 1
        ? {
            label: t('app.split_more', { action: t('app.desk_home.new_request') }),
            items: desks.map(({ desk }) => ({ id: desk.id, label: t('app.desk_home.in_desk', { desk: desk.name }) })),
            onselect: (id: string) => capture.open({ collectionId: id }),
          }
        : undefined,
    };
  });
</script>

<WorkPage>
  {#snippet top()}
  <ProductBand
    product="desk"
    icon="inbox"
    {name}
    figures={figures}
    {primary}
    breadcrumb={{
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: name },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  {/snippet}
  {#snippet tabs()}<ViewBar label={name} views={homeViews} isCompact={viewport.isCompact} closeLabel={t('app.sheet_close')} {current} onnavigate={(id) => onnavigate(homeViews.find((each) => each.id === id)?.href ?? '/desk')} />{/snippet}

  {#if isReading || desksAt === undefined}
    {#if hubs.length === 0 && !isReading}
      <ProductEmpty product="SERVICE_DESK" place={{ type: 'TENANT' }} kind="unused" icon="inbox" title={t('app.product_home.none', { product: name })}
        description={t(createHub ? 'app.product_home.none_body_create' : 'app.product_home.none_body')} create={createHub} isInHead {templates} />
    {:else}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {/if}
  {:else if desks.length === 0}
    <ProductEmpty product="SERVICE_DESK" place={{ type: 'TENANT' }} kind="unused" icon="inbox" title={t('app.desk_home.no_desk')} description={t('app.desk_home.no_desk_body')} create={createHub} isInHead {templates}
      isProductHome side={createHub && templates.length > 0 ? { label: t('app.product_empty.start_empty'), onclick: createHub.onclick } : undefined} />
  {:else if current === 'desks'}
    <ul class="rows" aria-label={t('app.desk_queues.desks')}>
      {#each desks as { desk, hub } (desk.id)}
        <li>
          <ListRow href={`/collections/${desk.id}`} onactivate={() => onnavigate(`/collections/${desk.id}`)} label={desk.name}>
            {#snippet leading()}<Icon name={markOf(desk)} size="sm" />{/snippet}
            <span class="name" title={desk.name}>{desk.name}</span>
            <span class="hub">{hub.name}</span>
          </ListRow>
        </li>
      {/each}
    </ul>
  {:else}
    <!-- A phone keeps mine and my deadlines kept, its worst kind only (§7, decision 4). -->
    <div class="figures" data-desk-figures><SummaryStrip label={t('app.desk_home.figures')} phone={[1, 4]}>
      <SummaryCard lead="figure" title={t('app.desk_home.fig_mine')} figure={totals.mine === undefined ? '…' : String(totals.mine)} line={t('app.desk_home.fig_mine_line')} />
      <SummaryCard lead="figure" title={t('app.desk_home.fig_unassigned')} figure={totals.unassigned === undefined ? '…' : String(totals.unassigned)} line={t('app.desk_home.fig_unassigned_line')} />
      <SummaryCard lead="figure" title={t('app.desk_home.fig_breaching')} figure={totals.breaching === undefined ? '…' : String(totals.breaching)} line={t('app.desk_home.fig_breaching_line')} />
      <div class="kept" data-desk-kept>
        <SummaryCard lead="figure" title={t('app.desk_home.fig_kept')}
          figure={keptAll[0] ? t('app.desk_home.percent', { share: String(percentOf(keptAll[0].share)) }) : kept.status === 'ready' ? '–' : '…'}
          line={keptAll.length === 0 && kept.status === 'ready' ? t('app.desk_home.kept_none') : t('app.desk_home.kept_window')}>
          {#if keptAll.length > 0}
            <ul class="kept-rows">
              {#each keptAll.slice(0, KEPT_SHOWN) as row, index (index)}
                <li data-kept-row>
                  <span class="kept-kind">{keptLabel(row)}</span>
                  <span class="kept-share">{t('app.desk_home.percent', { share: String(percentOf(row.share)) })}</span>
                  {#if row.breached > 0}<span class="kept-breached">{t('app.desk_home.kept_breached', { count: String(row.breached) })}</span>{/if}
                </li>
              {/each}
            </ul>
            {#if keptAll.length > KEPT_SHOWN}
              <Button size="sm" tone="subtle" onclick={() => (isKeptOpen = true)}>{t('app.desk_home.kept_more', { count: String(keptAll.length - KEPT_SHOWN) })}</Button>
            {/if}
          {/if}
        </SummaryCard>
      </div>
    </SummaryStrip></div>
    <Dialog bind:isOpen={isKeptOpen} title={t('app.desk_home.fig_kept')} dismissLabel={t('app.sheet_close')} size="m">
      <p class="kept-window">{t('app.desk_home.kept_window')}</p>
      <ul class="kept-all" aria-label={t('app.desk_home.fig_kept')}>
        {#each keptAll as row, index (index)}
          <li data-kept-row>
            <span class="kept-kind">{keptLabel(row)}</span>
            <span class="number">{t('app.desk_home.percent', { share: String(percentOf(row.share)) })}</span>
            <span class="kept-of">{t('app.desk_home.kept_of', { kept: String(row.kept), ended: String(row.ended) })}</span>
            {#if row.breached > 0}<span class="kept-breached">{t('app.desk_home.kept_breached', { count: String(row.breached) })}</span>{/if}
          </li>
        {/each}
      </ul>
    </Dialog>
    <DeskQueues desks={desksAt} withHub={hubs.length > 1} {onnavigate} ontotals={(next) => (totals = next)} />
  {/if}
</WorkPage>

<style>
  .figures { margin-block-end: var(--sp-200); }
  .kept-rows { display: grid; gap: var(--sp-025); margin: 0; padding: 0; list-style: none; font-size: var(--fs-075); }
  .kept-rows li { display: flex; flex-wrap: wrap; gap: var(--sp-050); align-items: baseline; }
  .kept-kind { color: var(--text-secondary); }
  .kept-share, .number { font-variant-numeric: tabular-nums; }
  .kept-breached { color: var(--text-danger); }
  .kept-all { display: grid; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .kept-all li { display: grid; grid-template-columns: minmax(0, 1fr) auto auto auto; gap: var(--sp-150); align-items: baseline; padding-block-end: var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .kept-of { color: var(--text-secondary); font-size: var(--fs-075); }
  .kept-window { margin: 0 0 var(--sp-100); color: var(--text-secondary); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) { .kept-rows li:not(:first-child) { display: none; } }
  .rows { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hub { display: block; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
</style>
