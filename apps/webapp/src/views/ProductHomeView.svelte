<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's workspace-wide home (user-experience.md §2.2, §5.1): the one door the products band
  // opens. One anatomy for every product - the trail *Products › <product>*, the title with what
  // the product does, the views as `PlaceTabs` where it has several, then the hubs the reader may
  // read where the product is on, each opening the hub's own screen of it. A hub narrows the
  // workspace's modules (UC-INS-21 check 3), so each hub is asked which are on there.
  import { untrack } from 'svelte';
  import { Badge, Button, EmptyState, Icon, ListRow, PageHeader, PlaceTabs, Skeleton, Stack, type PlaceTab } from '@hubtask/design-system/components';

  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';
  import { markOf } from '../lib/workspace/look.ts';
  import { hubCreationHere, reachesAdministrationArea } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { live } from '../lib/data/live.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { modules, modulesHere } from '../lib/data/modules.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { productOf } from '../lib/navigation.ts';
  import { agentRows, homeTabs, hubScreenOf, inventorySummary, pagesToReview, productDoors, recentBoards, recentPages, releasesDue } from '../lib/product/home.ts';
  import { releasesPath } from '../lib/data/releases.svelte.ts';
  import { agentsPath } from '../lib/data/agents.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { boardsPath } from '../lib/data/whiteboards.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { formatRelative } from '../lib/i18n/datetime.ts';
  import type { AgentList, CmdbHealth, PageList, ReleaseList, WhiteboardList } from '@hubtask/sync-engine';
  import { pagesPath } from '../lib/data/pages.svelte.ts';
  import { healthPath } from '../lib/data/cmdb.svelte.ts';
  import NotesView from './NotesView.svelte';
  import DeskQueues from '../lib/servicedesk/DeskQueues.svelte';
  import { desksOf } from '../lib/servicedesk/queues.ts';
  import { entryTypes } from '../lib/data/entrytypes.svelte.ts';

  interface Props {
    module: string;
    /** The view of the home, where it has several: the pages' *My notes*. */
    tab?: string;
    onnavigate: (path: string) => void;
  }
  const { module, tab, onnavigate }: Props = $props();

  const row = $derived(productOf(module));
  const name = $derived(row ? t(row.code) : '');
  $effect(() => page.entitle(name));

  const tabs = $derived<PlaceTab[]>(homeTabs(module).map((each) => ({ id: each.id, label: t(each.code), href: each.path })));
  const currentTab = $derived(tab ?? tabs[0]?.id);

  /** The hubs the tree draws, and for each the modules on there, once it has answered. */
  const hubs = $derived(containers.hubs.filter((hub) => !live.hasLost(hub.id)));
  let onAt = $state<Record<string, readonly string[]>>({});
  const hubKey = $derived(hubs.map((hub) => hub.id).join(','));
  $effect(() => {
    const ids = hubKey === '' ? [] : hubKey.split(',');
    return untrack(() => {
      const stops = ids.map((hubId) =>
        modules.here(hubId, (next) => {
          if (next.status === 'ready') onAt = { ...onAt, [hubId]: next.data.modules_on };
        }),
      );
      return () => {
        for (const stop of stops) stop();
      };
    });
  });
  const isReading = $derived(
    containers.hubsState.status === 'loading' || containers.hubsState.status === 'idle' || hubs.some((hub) => onAt[hub.id] === undefined),
  );
  const hubsWith = $derived(
    hubs.filter((hub) => onAt[hub.id] !== undefined && (modulesHere(manifest.value?.modules_on, onAt[hub.id]) as readonly string[]).includes(module)),
  );

  /**
   * One read per hub where the product is on, each hub's own as its tab reads it; a hub that
   * leaves the list stops its read. Called once per product at the component's start.
   */
  function eachHub<T>(forModule: string, pathOf: (hubId: string) => string, onready: (hubId: string, data: T) => void): void {
    const key = $derived(module === forModule ? hubsWith.map((hub) => hub.id).join(',') : '');
    $effect(() => {
      const ids = key === '' ? [] : key.split(',');
      return untrack(() => {
        const stops = ids.map((hubId) =>
          engine.subscribe<T>({ path: pathOf(hubId) }, (next) => {
            if (next.status === 'ready') onready(hubId, next.data);
          }),
        );
        return () => stops.forEach((stop) => stop());
      });
    });
  }

  /** The whiteboards' home lists the boards themselves (§2.2), recent first across the hubs. */
  const BOARDS_SHOWN = 12;
  let boardsAt = $state<Record<string, WhiteboardList['data']>>({});
  eachHub<WhiteboardList>('WHITEBOARD', boardsPath, (hubId, data) => (boardsAt = { ...boardsAt, [hubId]: data.data }));

  /** The pages' home: the reader's pages to review and the recently changed ones (§2.2). */
  const PAGES_SHOWN = 10;
  let pagesAt = $state<Record<string, PageList['data']>>({});
  eachHub<PageList>('KNOWLEDGE', pagesPath, (hubId, data) => (pagesAt = { ...pagesAt, [hubId]: data.data }));
  const knowledgeHubs = $derived(module === 'KNOWLEDGE' ? hubsWith.map((hub) => hub.id) : []);
  const toReview = $derived(pagesToReview(pagesAt, knowledgeHubs, actor.account?.id));
  const recent = $derived(recentPages(pagesAt, knowledgeHubs, PAGES_SHOWN));

  /** The plans' home lists the releases still to go out (§2.2), the soonest first. */
  const RELEASES_SHOWN = 8;
  let releasesAt = $state<Record<string, ReleaseList['data']>>({});
  eachHub<ReleaseList>('PLANNING', releasesPath, (hubId, data) => (releasesAt = { ...releasesAt, [hubId]: data.data }));
  const upcoming = $derived(module === 'PLANNING' ? releasesDue(releasesAt, hubsWith.map((hub) => hub.id), RELEASES_SHOWN) : []);

  /** The assets' home opens with the inventory's health across hubs (§2.2). */
  let healthAt = $state<Record<string, CmdbHealth>>({});
  eachHub<CmdbHealth>('ASSETS', healthPath, (hubId, data) => (healthAt = { ...healthAt, [hubId]: data }));
  const assetHubs = $derived(module === 'ASSETS' ? hubsWith.map((hub) => hub.id) : []);
  const inventory = $derived(inventorySummary(healthAt, assetHubs));

  const boards = $derived(module === 'WHITEBOARD' ? recentBoards(boardsAt, hubsWith.map((hub) => hub.id), BOARDS_SHOWN) : []);
  const hubName = (id: string) => containers.find(id)?.name ?? '';

  /** The agents' home lists the agents working here that the reader may see (§2.2). */
  let agentList = $state<AgentList['data'] | undefined>(undefined);
  $effect(() => {
    if (module !== 'AGENTS') return;
    return untrack(() => engine.subscribe<AgentList>({ path: agentsPath }, (next) => {
      if (next.status === 'ready') agentList = next.data.data;
    }));
  });
  const workingAgents = $derived(agentList ? agentRows(agentList, actor.account?.id, reachesAdministrationArea() === true) : []);
  const agentTone = (status: string) => (status === 'ACTIVE' ? 'success' : status === 'SUSPENDED' ? 'warning' : 'neutral');

  /**
   * The desk's home: the handler's queues across every desk collection of the hubs where the desk
   * is on (§2.2). Each hub's collections and the types are read to know which collections are desks.
   */
  const deskHubKey = $derived(module === 'SERVICE_DESK' ? hubsWith.map((hub) => hub.id).join(',') : '');
  $effect(() => {
    const ids = deskHubKey === '' ? [] : deskHubKey.split(',');
    return untrack(() => {
      const stops = [...ids.map((hubId) => containers.openLevel(hubId)), ...(ids.length > 0 ? [entryTypes.open()] : [])];
      return () => stops.forEach((stop) => stop());
    });
  });
  const typesReady = $derived(entryTypes.state !== undefined && entryTypes.state.status !== 'idle' && entryTypes.state.status !== 'loading');
  const desksAt = $derived(
    module === 'SERVICE_DESK' && typesReady && hubsWith.every((hub) => !containers.isLevelLoading(hub.id))
      ? Object.fromEntries(hubsWith.map((hub) => [hub.id, desksOf(containers.collectionsOf(hub.id), entryTypes.all)]))
      : undefined,
  );
  const hasDesks = $derived(desksAt !== undefined && Object.values(desksAt).some((ids) => ids.length > 0));

  const doors = $derived(productDoors(module, { isAdministrationReachable: reachesAdministrationArea() === true }));
  const creation = $derived(hubCreationHere());
  const mayCreateHub = $derived(creation === 'shared' || creation === 'private');
  let isCreatingHub = $state(false);
</script>

<Stack gap="300">
  <PageHeader
    title={name}
    subtitle={t(`app.modules.hint_${module.toLowerCase()}`)}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: name },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  {#if tabs.length > 1}
    <PlaceTabs label={name} {tabs} current={currentTab} onnavigate={(id) => onnavigate(tabs.find((each) => each.id === id)?.href ?? '/')} />
  {/if}

  {#if currentTab === 'notes'}
    <NotesView {onnavigate} isEmbedded />
  {:else}
    {#if doors.length > 0}
      <ul class="doors">
        {#each doors as door (door.id)}
          <li>
            <ListRow href={door.path} onactivate={() => onnavigate(door.path)}>
              {#snippet leading()}<Icon name={door.icon} size="sm" />{/snippet}
              {t(door.code)}
            </ListRow>
          </li>
        {/each}
      </ul>
    {/if}

    {#if hasDesks && desksAt}
      <DeskQueues desks={desksAt} withHub {onnavigate} />
    {/if}
    {#if workingAgents.length > 0}
      <section aria-labelledby="product-agents">
        <Stack gap="100">
          <h2 class="heading" id="product-agents">{t('app.product_home.agents_working')}</h2>
          <ul class="hubs">
            {#each workingAgents as row (row.agent.id)}
              <li>
                <ListRow href={row.path} onactivate={row.path ? () => onnavigate(row.path ?? '/') : undefined} label={row.agent.name}>
                  {#snippet leading()}<Icon name="zap" size="sm" />{/snippet}
                  <span class="name" title={row.agent.name}>{row.agent.name}</span>
                  {#if row.agent.purpose}<span class="purpose">{row.agent.purpose}</span>{/if}
                  {#snippet trailing()}<Badge tone={agentTone(row.agent.status)} size="sm">{t(`app.agents.status.${row.agent.status}`)}</Badge>{/snippet}
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if assetHubs.length > 0 && Object.keys(healthAt).length > 0}
      <section aria-labelledby="product-inventory" aria-busy={!inventory.isWhole}>
        <Stack gap="100">
          <h2 class="heading" id="product-inventory">{t('app.product_home.inventory')}</h2>
          <dl class="figures">
            <div><dt>{t('app.assets.health_total')}</dt><dd>{inventory.total}</dd></div>
            <div><dt>{t('app.assets.problems')}</dt><dd>{inventory.wanting}</dd></div>
            <div><dt>{t('app.assets.review_overdue')}</dt><dd>{inventory.reviewOverdue}</dd></div>
          </dl>
        </Stack>
      </section>
    {/if}
    {#if upcoming.length > 0}
      <section aria-labelledby="product-releases">
        <Stack gap="100">
          <h2 class="heading" id="product-releases">{t('app.product_home.releases_due')}</h2>
          <ul class="hubs">
            {#each upcoming as each (each.id)}
              <li>
                <ListRow href={`/hubs/${each.hub_id}/releases`} onactivate={() => onnavigate(`/hubs/${each.hub_id}/releases`)} label={each.name}>
                  {#snippet leading()}<Icon name="flag" size="sm" />{/snippet}
                  <span class="name" title={each.name}>{each.name}</span>
                  <span class="purpose">{[hubName(each.hub_id), t('app.releases.due_on', { day: each.release_on })].filter(Boolean).join(' · ')}</span>
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if toReview.length > 0}
      <section aria-labelledby="product-review">
        <Stack gap="100">
          <h2 class="heading" id="product-review">{t('app.product_home.pages_review')}</h2>
          <ul class="hubs">
            {#each toReview as each (each.id)}
              <li>
                <ListRow href={`/pages/${each.id}`} onactivate={() => onnavigate(`/pages/${each.id}`)} label={each.title}>
                  {#snippet leading()}<Icon name="book-open" size="sm" />{/snippet}
                  <span class="name" title={each.title}>{each.title}</span>
                  <span class="purpose">{hubName(each.hub_id)}</span>
                  {#snippet trailing()}<Badge tone="warning" size="sm">{t('app.pages.due_for_review')}</Badge>{/snippet}
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if recent.length > 0}
      <section aria-labelledby="product-pages">
        <Stack gap="100">
          <h2 class="heading" id="product-pages">{t('app.product_home.pages_recent')}</h2>
          <ul class="hubs">
            {#each recent as each (each.id)}
              <li>
                <ListRow href={`/pages/${each.id}`} onactivate={() => onnavigate(`/pages/${each.id}`)} label={each.title}>
                  {#snippet leading()}<Icon name="book-open" size="sm" />{/snippet}
                  <span class="name" title={each.title}>{each.title}</span>
                  <span class="purpose">{[hubName(each.hub_id), formatRelative(each.updated_at, messages.locale)].filter(Boolean).join(' · ')}</span>
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if boards.length > 0}
      <section aria-labelledby="product-boards">
        <Stack gap="100">
          <h2 class="heading" id="product-boards">{t('app.product_home.boards_recent')}</h2>
          <ul class="cards">
            {#each boards as board (board.id)}
              <li>
                <a class="card" href={`/whiteboards/${board.id}`}>
                  <span class="card-mark" aria-hidden="true"><Icon name="layout-template" size="md" /></span>
                  <span class="card-title" title={board.title}>{board.title}</span>
                  <span class="card-meta">{[hubName(board.hub_id), t('app.whiteboards.objects', { count: String(board.element_count) }), formatRelative(board.updated_at, messages.locale)].filter(Boolean).join(' · ')}</span>
                </a>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if hubScreenOf(module, '') !== undefined}
      <section aria-labelledby="product-hubs">
        <Stack gap="100">
          <h2 class="heading" id="product-hubs">{t('app.product_home.hubs')}</h2>
          {#if isReading && hubsWith.length === 0}
            <div aria-busy="true"><Skeleton lines={3} /></div>
          {:else if hubsWith.length === 0}
            <EmptyState
              kind="unused"
              icon={row?.icon ?? 'hub'}
              title={t('app.product_home.none', { product: name })}
              description={t(mayCreateHub ? 'app.product_home.none_body_create' : 'app.product_home.none_body')}
            >
              {#snippet action()}
                {#if mayCreateHub}
                  <Button tone="primary" icon="plus" onclick={() => (isCreatingHub = true)}>{t('app.workspace.create_hub')}</Button>
                {/if}
              {/snippet}
            </EmptyState>
          {:else}
            <ul class="hubs">
              {#each hubsWith as hub (hub.id)}
                {@const path = hubScreenOf(module, hub.id) ?? `/hubs/${hub.id}`}
                <li>
                  <ListRow href={path} onactivate={() => onnavigate(path)} label={hub.name}>
                    {#snippet leading()}<Icon name={markOf(hub)} size="sm" />{/snippet}
                    <span class="name" title={hub.name}>{hub.name}</span>
                  </ListRow>
                </li>
              {/each}
            </ul>
          {/if}
        </Stack>
      </section>
    {/if}
  {/if}
</Stack>

{#if isCreatingHub && mayCreateHub}
  <CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" isPrivateOnly={creation === 'private'} oncreated={(id) => onnavigate(hubScreenOf(module, id) ?? `/hubs/${id}`)} />
{/if}

<style>
  /* Boards are visual, so they are cards (§5.1), as many to a row as fit. */
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 24ch), 1fr)); gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  .card {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--sp-050);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    text-decoration: none;
  }
  .card:hover { background: var(--bg-surface-hover); }
  .card:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .card-mark { display: inline-flex; color: var(--text-subtle); }
  .card-title { overflow: hidden; font-weight: var(--fw-medium); text-overflow: ellipsis; white-space: nowrap; }
  .card-meta { overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  .purpose { display: block; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  /* The same figures as a hub's health section draws (lib/assets/Health.svelte). */
  .figures { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--sp-1600)), 1fr)); gap: var(--sp-100); margin: 0; }
  .figures div {
    display: grid; gap: var(--sp-025); padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface);
  }
  .figures dt { color: var(--text-secondary); font-size: var(--fs-075); }
  .figures dd { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); font-variant-numeric: tabular-nums; }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .doors, .hubs { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  /* One line with an ellipsis and the whole name as the tooltip (user-experience.md §2.5). */
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
