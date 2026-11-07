<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The tool *Plan*'s *Sprints* (the navigation concept §2.4): the sprints running in the hub's
  // collections, the soonest to end first, each opening its collection's *Sprint* view. A sprint
  // belongs to its collection; this page only gathers them, as the Plans home does across hubs.
  import { untrack } from 'svelte';
  import { EmptyState, Icon, ListRow, PageHeader, Skeleton, Stack } from '@hubtask/design-system/components';
  import type { SprintList } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { sprintsPath } from '../lib/data/sprints.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { sprintsRunning } from '../lib/product/home.ts';
  import { sprintDaysLeft } from '../lib/today.ts';
  import PlaceViews from '../lib/workspace/PlaceViews.svelte';

  interface Props {
    hubId: string;
    onnavigate: (path: string) => void;
  }
  const { hubId, onnavigate }: Props = $props();

  /** More than a hub runs at once; the rest stand in their collections. */
  const SHOWN = 50;

  const hub = $derived(containers.find(hubId));
  $effect(() => frame.entitle(hub ? t('app.planning.title_of', { place: hub.name }) : t('app.planning.title')));
  $effect(() => {
    const at = hubId;
    return untrack(() => containers.openLevel(at));
  });

  const collectionIds = $derived(containers.collectionsOf(hubId).map((each) => each.id));
  const key = $derived(collectionIds.join(','));
  let lists = $state<Record<string, SprintList['data']>>({});
  $effect(() => {
    const ids = key === '' ? [] : key.split(',');
    return untrack(() => {
      const stops = ids.map((id) =>
        engine.subscribe<SprintList>({ path: `${sprintsPath(id)}?state=ACTIVE` }, (next) => {
          if (next.status === 'ready') lists = { ...lists, [id]: next.data.data };
          if (next.status === 'failed') lists = { ...lists, [id]: [] };
        }),
      );
      return () => stops.forEach((stop) => stop());
    });
  });
  const isReading = $derived(containers.isLevelLoading(hubId) || collectionIds.some((id) => lists[id] === undefined));
  const running = $derived(sprintsRunning(lists, collectionIds, SHOWN));
  const today = $derived(todayIn(actor.zone));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.nav.tool_plan')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={hub
      ? {
          trail: [{ id: 'place', label: hub.name, href: `/hubs/${hubId}` }, { id: 'plan', label: t('app.nav.tool_plan') }],
          label: t('app.product_home.trail'),
          expandLabel: t('app.admin.expand_trail'),
          onnavigate: () => onnavigate(`/hubs/${hubId}`),
        }
      : undefined}
  />
  <PlaceViews placeId={hubId} tool="plan" current="sprints" {onnavigate} />

  {#if isReading && running.length === 0}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if running.length === 0}
    <EmptyState kind="unused" title={t('app.sprints.none_title')} description={t('app.sprints.none_hint')} />
  {:else}
    <section aria-labelledby="hub-sprints">
      <Stack gap="100">
        <h2 class="heading" id="hub-sprints">{t('app.product_home.sprints_running')}</h2>
        <ul class="rows">
          {#each running as each (each.id)}
            {@const where = containers.find(each.collection_id)}
            <li data-running-sprint={each.id}>
              <ListRow href={`/collections/${each.collection_id}/sprint`} onactivate={() => onnavigate(`/collections/${each.collection_id}/sprint`)} label={each.name}>
                {#snippet leading()}<Icon name="repeat" size="sm" />{/snippet}
                <span class="name" title={each.name}>{each.name}</span>
                <span class="purpose">{[where?.name ?? '', t('app.sprints.days_left', { count: String(sprintDaysLeft(each.end_on, today)) })].filter(Boolean).join(' · ')}</span>
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
  .name { min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: var(--fw-medium); }
  .purpose { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
