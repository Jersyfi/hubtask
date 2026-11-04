<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's *Desk* tab (user-experience.md §2.4): the handler's queues across the hub's desk
  // collections, then the desks themselves. A hub with no desk says how a collection becomes one.
  import { untrack } from 'svelte';
  import { EmptyState, Icon, ListRow, PageHeader, Skeleton, Stack } from '@hubtask/design-system/components';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { entryTypes } from '../lib/data/entrytypes.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import DeskQueues from '../lib/servicedesk/DeskQueues.svelte';
  import { desksOf } from '../lib/servicedesk/queues.ts';
  import { markOf } from '../lib/workspace/look.ts';
  import PlaceStrip from '../lib/workspace/PlaceStrip.svelte';

  interface Props {
    hubId: string;
    onnavigate: (path: string) => void;
  }
  const { hubId, onnavigate }: Props = $props();

  const hub = $derived(containers.find(hubId));
  $effect(() => frame.entitle(hub ? t('app.hub_desk.title_of', { place: hub.name }) : t('app.place_tabs.desk')));

  $effect(() => {
    const at = hubId;
    return untrack(() => containers.openLevel(at));
  });
  $effect(() => untrack(() => entryTypes.open()));

  // A refused read of the types reads as no desk: what the reader cannot read offers no portal.
  const typesStatus = $derived(entryTypes.state?.status);
  const isReading = $derived(containers.isLevelLoading(hubId) || typesStatus === undefined || typesStatus === 'idle' || typesStatus === 'loading');
  const deskIds = $derived(isReading ? [] : desksOf(containers.collectionsOf(hubId), entryTypes.all));
  const desks = $derived(containers.collectionsOf(hubId).filter((collection) => deskIds.includes(collection.id)));
  // *New request* opens the desk's list, where an entry is captured (§5.2); each other desk in its list.
  const primary = $derived.by(() => {
    const first = desks[0];
    if (!first) return undefined;
    return {
      label: t('app.desk_home.new_request'),
      icon: 'plus' as const,
      onclick: () => onnavigate(`/collections/${first.id}`),
      menu: desks.length > 1
        ? { label: t('app.desk_home.new_request_in'), items: desks.map((desk) => ({ id: desk.id, label: t('app.desk_home.in_desk', { desk: desk.name }) })), onselect: (id: string) => onnavigate(`/collections/${id}`) }
        : undefined,
    };
  });
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.place_tabs.desk')}
    isTitleInBar={viewport.isCompact}
    {primary}
    breadcrumb={hub
      ? {
          trail: [{ id: 'place', label: hub.name, href: `/hubs/${hubId}` }, { id: 'desk', label: t('app.place_tabs.desk') }],
          label: t('app.hub_desk.trail'),
          expandLabel: t('app.admin.expand_trail'),
          onnavigate: () => onnavigate(`/hubs/${hubId}`),
        }
      : undefined}
  />
  <PlaceStrip placeId={hubId} current="desk" {onnavigate} />

  {#if isReading}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if desks.length === 0}
    <EmptyState kind="unused" icon="inbox" title={t('app.desk_queues.none')} description={t('app.desk_queues.none_body')} />
  {:else}
    <DeskQueues desks={{ [hubId]: desks.map((desk) => desk.id) }} {onnavigate} />
    <section aria-labelledby="hub-desks">
      <Stack gap="100">
        <h2 class="heading" id="hub-desks">{t('app.desk_queues.desks')}</h2>
        <ul class="rows">
          {#each desks as desk (desk.id)}
            <li>
              <ListRow href={`/collections/${desk.id}`} onactivate={() => onnavigate(`/collections/${desk.id}`)} label={desk.name}>
                {#snippet leading()}<Icon name={markOf(desk)} size="sm" />{/snippet}
                <span class="name" title={desk.name}>{desk.name}</span>
              </ListRow>
            </li>
          {/each}
        </ul>
      </Stack>
    </section>
  {/if}
</Stack>

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .rows { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
