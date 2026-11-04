<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The service desk's home (user-experience.md §5.1, §7.4): the trail, the title with its figure
  // (*12 open · 2 about to breach · 3 unassigned*), *New request* as the primary - into the desk's
  // list, where an entry is captured - then *Queues · Desks · Settings*. The queues span every desk
  // collection of the hubs where the desk is on; the desks are listed with their hub; the settings
  // are the workspace's calendars, policies, replies and organisations, read by every member and
  // changed by whoever shapes the workspace (ServiceDeskView).

  import { untrack } from 'svelte';
  import { Icon, ListRow, PageHeader, PlaceTabs, Skeleton, Stack, type PlaceTab } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { homeTabs } from '../product/home.ts';
  import ProductEmpty from '../product/ProductEmpty.svelte';
  import { markOf } from '../workspace/look.ts';
  import ServiceDeskView from '../../views/ServiceDeskView.svelte';
  import DeskQueues from './DeskQueues.svelte';
  import { desksOf, type DeskQueue } from './queues.ts';

  interface Props {
    /** The hubs where the desk is on, once each answered. */
    hubs: readonly { readonly id: string; readonly name: string }[];
    /** Whether the hubs or their modules are still being read. */
    isReading: boolean;
    tab?: string;
    /** The act that starts a hub, where the reader may; a desk is a template's collection in one. */
    createHub?: { label: string; onclick: () => void };
    /** The desk's templates while no desk exists (user-experience.md §4), each starting a hub from one. */
    templates?: readonly { label: string; onclick: () => void }[];
    onnavigate: (path: string) => void;
  }
  const { hubs, isReading, tab, createHub, templates = [], onnavigate }: Props = $props();

  const name = $derived(t('app.nav.desk'));
  const tabs = $derived<PlaceTab[]>(homeTabs('SERVICE_DESK').map((each) => ({ id: each.id, label: t(each.code), href: each.path })));
  const current = $derived(tabs.some((each) => each.id === tab) ? (tab as string) : 'queues');

  const hubKey = $derived(hubs.map((hub) => hub.id).join(','));
  $effect(() => {
    const ids = hubKey === '' ? [] : hubKey.split(',');
    return untrack(() => {
      const stops = [...ids.map((hubId) => containers.openLevel(hubId)), ...(ids.length > 0 ? [entryTypes.open()] : [])];
      return () => stops.forEach((stop) => stop());
    });
  });
  const typesReady = $derived(entryTypes.state !== undefined && entryTypes.state.status !== 'idle' && entryTypes.state.status !== 'loading');
  const desksAt = $derived(
    !isReading && typesReady && hubs.every((hub) => !containers.isLevelLoading(hub.id))
      ? Object.fromEntries(hubs.map((hub) => [hub.id, desksOf(containers.collectionsOf(hub.id), entryTypes.all)]))
      : undefined,
  );
  const desks = $derived(
    desksAt === undefined
      ? []
      : hubs.flatMap((hub) => containers.collectionsOf(hub.id).filter((collection) => desksAt[hub.id]?.includes(collection.id)).map((desk) => ({ desk, hub }))),
  );

  let totals = $state<Partial<Record<DeskQueue, number>>>({});
  const figureText = $derived(
    totals.open === undefined
      ? ''
      : [
          t('app.desk_home.open', { count: String(totals.open) }),
          ...(totals.breaching ? [t('app.desk_home.breaching', { count: String(totals.breaching) })] : []),
          ...(totals.unassigned ? [t('app.desk_home.unassigned', { count: String(totals.unassigned) })] : []),
        ].join(' · '),
  );

  const primary = $derived.by(() => {
    const first = desks[0];
    if (!first) return createHub ? { label: createHub.label, icon: 'plus' as const, onclick: createHub.onclick } : undefined;
    return {
      label: t('app.desk_home.new_request'),
      icon: 'plus' as const,
      onclick: () => onnavigate(`/collections/${first.desk.id}`),
      menu: desks.length > 1
        ? {
            label: t('app.desk_home.new_request_in'),
            items: desks.map(({ desk }) => ({ id: desk.id, label: t('app.desk_home.in_desk', { desk: desk.name }) })),
            onselect: (id: string) => onnavigate(`/collections/${id}`),
          }
        : undefined,
    };
  });
</script>

<Stack gap="300">
  <PageHeader
    title={name}
    subtitle={t('app.modules.hint_service_desk')}
    isTitleInBar={viewport.isCompact}
    {primary}
    breadcrumb={{
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: name },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  >
    {#snippet figure()}{#if figureText}<span class="figure" data-desk-figure>{figureText}</span>{/if}{/snippet}
  </PageHeader>

  <PlaceTabs label={name} {tabs} {current} onnavigate={(id) => onnavigate(tabs.find((each) => each.id === id)?.href ?? '/desk')} />

  {#if current === 'settings'}
    <ServiceDeskView isEmbedded />
  {:else if isReading || desksAt === undefined}
    {#if hubs.length === 0 && !isReading}
      <ProductEmpty kind="unused" icon="inbox" title={t('app.product_home.none', { product: name })}
        description={t(createHub ? 'app.product_home.none_body_create' : 'app.product_home.none_body')} create={createHub} {templates} />
    {:else}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {/if}
  {:else if desks.length === 0}
    <ProductEmpty kind="unused" icon="inbox" title={t('app.desk_home.no_desk')} description={t('app.desk_home.no_desk_body')} create={createHub} {templates} />
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
    <DeskQueues desks={desksAt} withHub={hubs.length > 1} {onnavigate} ontotals={(next) => (totals = next)} />
  {/if}
</Stack>

<style>
  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .rows { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hub { display: block; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
</style>
