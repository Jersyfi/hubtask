<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's workspace-wide home (user-experience.md §2.2, §5.1): the one door the products band
  // opens. One anatomy for every product - the trail *Products › <product>*, the title with what
  // the product does, the views as one `ViewBar` where it has several, then the hubs the reader may
  // read where the product is on, each opening the hub's own screen of it. A hub narrows the
  // workspace's modules (UC-INS-21 check 3), so each hub is asked which are on there.
  import { untrack } from 'svelte';
  import { Badge, EmptyState, Icon, ListRow, PageHeader, ViewBar, Skeleton, Stack, type ViewLink } from '@hubtask/design-system/components';

  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import { markOf } from '../lib/workspace/look.ts';
  import { holds, holdsHere, hubCreationHere, reachesAdministrationArea, roleOrder } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { live } from '../lib/data/live.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { modules, modulesHere } from '../lib/data/modules.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { productOf } from '../lib/navigation.ts';
  import { createAt, createOf, homeTabs, placesByUse, writableHubs, isUnused, hubScreenOf, hubsWithAssetsFirst, inventoryByClass, inventorySummary, nextReleaseOf, pagesToReview, recentBoards, recentPages, releasesDue, sprintsRunning } from '../lib/product/home.ts';
  import CollectionFigure from '../lib/entries/CollectionFigure.svelte';
  import { releasesPath } from '../lib/data/releases.svelte.ts';
  import { sprintsPath } from '../lib/data/sprints.svelte.ts';
  import { todayIn } from '../lib/i18n/zone.ts';
  import { sprintDaysLeft } from '../lib/today.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { boardsPath } from '../lib/data/whiteboards.svelte.ts';
  import BoardCard from '../lib/whiteboards/BoardCard.svelte';
  import { engine } from '../lib/data/engine.ts';
  import { formatPlainDay, formatRelative } from '../lib/i18n/datetime.ts';
  import type { CiClassList, CmdbHealth, ConfigTemplate, ConfigTemplateList, PageList, PortfolioList, ReleaseList, SprintList, WhiteboardList } from '@hubtask/sync-engine';
  import { PORTFOLIOS } from '../lib/data/goals.svelte.ts';
  import BoardTemplates, { type BoardStart, type RetroFormat } from '../lib/whiteboards/BoardTemplates.svelte';
  import { config } from '../lib/data/config.svelte.ts';
  import { productTemplates } from '../lib/product/templates.ts';
  import { pagesPath } from '../lib/data/pages.svelte.ts';
  import { classesPath, classWord, healthPath } from '../lib/data/cmdb.svelte.ts';
  import NotesView from './NotesView.svelte';
  import CmdbView from './CmdbView.svelte';
  import DeskHome from '../lib/servicedesk/DeskHome.svelte';

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

  // The assets' classes, sources and settings are the administration's screen at their home:
  // offered to whoever reaches the administration, as the door before them was (P-05). The plans'
  // roadmaps are the portfolios', which are the goals' product.
  const ADMINISTERED = ['classes', 'sources', 'settings'];
  const tabs = $derived<ViewLink[]>(
    homeTabs(module)
      .filter((each) => !(module === 'ASSETS' && ADMINISTERED.includes(each.id)) || reachesAdministrationArea() === true)
      .filter((each) => each.id !== 'roadmaps' || (manifest.value?.modules_on ?? []).includes('GOALS'))
      .map((each) => ({ id: each.id, label: t(each.code), href: each.path })),
  );
  const currentTab = $derived(tabs.some((each) => each.id === tab) ? (tab as string) : tabs[0]?.id);
  /**
   * Where the home lists its hubs: the pages' *Spaces* (a hub is a space), the boards' *All*, else
   * the first tab; a home without a hub says so on any tab.
   */
  const showsHubs = $derived(module === 'KNOWLEDGE' ? currentTab === 'spaces' : module === 'WHITEBOARD' ? currentTab === 'all' : currentTab === tabs[0]?.id);
  /** A tab that lists one kind whole, where *For you* shows its first few. */
  const isWhole = $derived(currentTab === 'sprints' || currentTab === 'releases' || currentTab === 'all');

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
  const upcoming = $derived(module === 'PLANNING' ? releasesDue(releasesAt, hubsWith.map((hub) => hub.id), isWhole ? Infinity : RELEASES_SHOWN) : []);

  /** The plans' home lists the sprints running in the hubs' collections (§2.2), the soonest to end first. */
  const SPRINTS_SHOWN = 8;
  let sprintsAt = $state<Record<string, SprintList['data']>>({});
  const collectionsOfHub = (hubId: string) => containers.collectionsOf(hubId).map((collection) => collection.id);
  const sprintCollections = $derived(module === 'PLANNING' ? hubsWith.flatMap((hub) => collectionsOfHub(hub.id)) : []);
  const sprintKey = $derived(sprintCollections.join(','));
  $effect(() => {
    const ids = sprintKey === '' ? [] : sprintKey.split(',');
    return untrack(() => {
      const stops = ids.map((id) =>
        engine.subscribe<SprintList>({ path: `${sprintsPath(id)}?state=ACTIVE` }, (next) => {
          if (next.status === 'ready') sprintsAt = { ...sprintsAt, [id]: next.data.data };
        }),
      );
      return () => stops.forEach((stop) => stop());
    });
  });
  const running = $derived(sprintsRunning(sprintsAt, sprintCollections, isWhole ? Infinity : SPRINTS_SHOWN));
  const today = $derived(todayIn(actor.zone));
  const sprintLeft = (endOn: string) => t('app.sprints.days_left', { count: String(sprintDaysLeft(endOn, today)) });

  /** The assets' home opens with the inventory's health across hubs (§2.2). */
  let healthAt = $state<Record<string, CmdbHealth>>({});
  eachHub<CmdbHealth>('ASSETS', healthPath, (hubId, data) => (healthAt = { ...healthAt, [hubId]: data }));
  const assetHubs = $derived(module === 'ASSETS' ? hubsWith.map((hub) => hub.id) : []);
  const inventory = $derived(inventorySummary(healthAt, assetHubs));
  // The inventory itself across hubs, by class: a shipped class's word is the catalogue's, an own
  // class's its name, so the classes are read where the home lists them.
  const byClass = $derived(inventoryByClass(healthAt, assetHubs));
  let ownClasses = $state<CiClassList['data']>([]);
  const readsClasses = $derived(module === 'ASSETS' && assetHubs.length > 0);
  $effect(() => {
    if (!readsClasses) return;
    return untrack(() => engine.subscribe<CiClassList>({ path: classesPath }, (next) => {
      if (next.status === 'ready') ownClasses = next.data.data;
    }));
  });
  const classOf = (key: string) => classWord(key, ownClasses, (code) => (messages.has(code) ? t(code) : undefined));
  const heldIn = (held: (typeof byClass)[number]) =>
    held.hubs.map((each) => t('app.product_home.held_in', { count: String(each.total), hub: hubName(each.hubId) })).join(' · ');
  /** The hubs the home lists: on the assets' home those that hold an inventory (§2.2). */
  const hubsListed = $derived(module === 'ASSETS' ? hubsWithAssetsFirst(hubsWith, healthAt) : hubsWith);
  /** What a hub's row says of it beside its name (§2.2): the plan's next release, the hub's inventory. */
  function hubLine(hubId: string): string | undefined {
    if (module === 'PLANNING') {
      const sprint = sprintsRunning(sprintsAt, collectionsOfHub(hubId), 1)[0];
      const next = nextReleaseOf(releasesAt, hubId);
      const said = [
        ...(sprint ? [t('app.product_home.sprint_line', { name: sprint.name, remaining: sprintLeft(sprint.end_on) })] : []),
        ...(next ? [t('app.product_home.next_release', { name: next.name, day: formatPlainDay(next.release_on, messages.locale) })] : []),
      ];
      return said.length > 0 ? said.join(' · ') : undefined;
    }
    if (module === 'ASSETS') {
      const health = healthAt[hubId];
      if (!health) return undefined;
      return [
        t('app.assets.figure_total', { count: health.total }),
        ...(health.problems.length > 0 ? [t('app.assets.figure_wanting', { count: health.problems.length })] : []),
      ].join(' · ');
    }
    return undefined;
  }

  const boards = $derived(module === 'WHITEBOARD' ? recentBoards(boardsAt, hubsWith.map((hub) => hub.id), isWhole ? Infinity : BOARDS_SHOWN) : []);

  /** The plans' *Roadmaps*: every portfolio's roadmap, read once its tab is open. */
  let portfolios = $state<PortfolioList['data'] | undefined>(undefined);
  $effect(() => {
    if (currentTab !== 'roadmaps') return;
    return untrack(() => engine.subscribe<PortfolioList>({ path: PORTFOLIOS }, (next) => {
      if (next.status === 'ready') portfolios = next.data.data;
      else if (next.status === 'failed') portfolios = [];
    }));
  });

  /** The boards' *Templates*: a card starts a board in the hub holding most of them, its gallery open. */
  let boardStart = $state<BoardStart>('BLANK');
  let boardFormat = $state<RetroFormat>('RETRO_START_STOP_CONTINUE');
  function startBoard(choice: BoardStart): void {
    const hub = createPlaces[0];
    if (hub) onnavigate(`/hubs/${encodeURIComponent(hub.id)}/contents?kind=whiteboards&new=1&start=${choice}`);
  }
  const hubName = (id: string) => containers.find(id)?.name ?? '';

  const creation = $derived(hubCreationHere());
  const mayCreateHub = $derived(creation === 'shared' || creation === 'private');
  let isCreatingHub = $state(false);

  // The first visit's templates (user-experience.md §4): the catalogue's that switch this product
  // on, read only where the reader may make the hub one starts, and only while there is none.
  let catalogue = $state<readonly ConfigTemplate[]>([]);
  // The desk offers them while it has no desk, though hubs exist: a request type of the workspace
  // makes none (UC-SVC-05), and a service template's hub does.
  // A product switched on at the workspace is on in every hub at once, so the first visit is
  // decided by whether anything of it exists yet - where the home's reads answer that whole.
  const objectCounts = $derived.by((): Record<string, number> | undefined => {
    const count = <T,>(at: Record<string, T>, of: (data: T) => number) =>
      Object.fromEntries(Object.entries(at).map(([hubId, data]) => [hubId, of(data)]));
    if (module === 'KNOWLEDGE') return count(pagesAt, (data) => data.length);
    if (module === 'WHITEBOARD') return count(boardsAt, (data) => data.length);
    if (module === 'ASSETS') return count(healthAt, (data) => data.total);
    return undefined;
  });
  const isUnusedHere = $derived(
    !isReading && hubsWith.length > 0 && objectCounts !== undefined && isUnused(objectCounts, hubsWith.map((hub) => hub.id)) === true,
  );
  const isFirstVisit = $derived(mayCreateHub && !isReading && (hubsWith.length === 0 || module === 'SERVICE_DESK' || isUnusedHere));
  // The catalogue is read by whoever holds STRUCTURE on the workspace; anybody else starts from the
  // hub dialog's presets, and asks nothing the server would refuse.
  $effect(() => {
    if (!isFirstVisit || holdsHere('STRUCTURE') !== true) return;
    return untrack(() => config.templates((next: { status: string; data?: ConfigTemplateList }) => {
      if (next.status === 'ready' && Array.isArray(next.data?.data)) catalogue = next.data.data;
    }));
  });
  /** The template the new hub is to take, chosen on the empty home. */
  let startingFrom = $state<string | undefined>(undefined);
  const templateName = (template: ConfigTemplate): string =>
    template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;
  const starts = $derived(isFirstVisit ? productTemplates(catalogue, module).map((template) => ({
    label: templateName(template),
    onclick: () => { startingFrom = template.key; isCreatingHub = true; },
  })) : []);
  const createHub = $derived(mayCreateHub ? { label: t('app.workspace.create_hub'), onclick: () => { startingFrom = undefined; isCreatingHub = true; } } : undefined);
  // The home's create act (UX-2): the product's own words, opening the create dialog on the place's
  // screen of it - the place holding most of the product, the others in its menu. Without a hub the empty state's *Create hub* is it.
  const createAct = $derived(createOf(module));
  // A create act at a collection reads the collections of the hubs where the product is on.
  const collectionHubs = $derived(createAct?.level === 'COLLECTION' ? hubsWith.map((hub) => hub.id).join(',') : '');
  $effect(() => {
    const ids = collectionHubs ? collectionHubs.split(',') : [];
    const stops = untrack(() => ids.map((id) => containers.openLevel(id)));
    return () => {
      for (const stop of stops) stop();
    };
  });
  // A create act at a hub reads the reader's role in each, and offers only where they may write
  // (P-05): a viewer is offered no act the place's screen would not open.
  const writeHubs = $derived(createAct?.level === 'HUB' ? hubsWith.map((hub) => hub.id).join(',') : '');
  $effect(() => {
    const ids = writeHubs ? writeHubs.split(',') : [];
    const stops = untrack(() => ids.map((hubId) => people.open({ hubId })));
    return () => {
      for (const stop of stops) stop();
    };
  });
  const mayWriteIn = (hubId: string): boolean | undefined => {
    const role = people.roleOf({ hubId }, actor.account?.id, roleOrder());
    return role === undefined ? undefined : holds(role ?? undefined, 'WRITE_ITEMS').status === 'permitted';
  };
  const createPlaces = $derived(
    createAct?.level === 'COLLECTION'
      ? hubsWith.flatMap((hub) => containers.collectionsOf(hub.id).map((collection) => ({ id: collection.id, label: `${hub.name} › ${collection.name}` })))
      : placesByUse(writableHubs(hubsWith, mayWriteIn).map((hub) => ({ id: hub.id, label: hub.name })), objectCounts),
  );
  const primary = $derived.by(() => {
    // The home's act stays on its setup tabs (§5.4): the head is the home's on every tab of it.
    if (!createAct || isReading || (currentTab !== tabs[0]?.id && !ADMINISTERED.includes(currentTab ?? ''))) return undefined;
    const first = createPlaces[0];
    if (!first) return undefined;
    // The place's own create dialog opens there (§5.2): the act creates, it does not list.
    const go = (id: string) => onnavigate(createAt(module, id) ?? `/hubs/${id}`);
    return {
      label: t(createAct.code),
      icon: 'plus' as const,
      onclick: () => go(first.id),
      menu: createPlaces.length > 1 ? { label: t(createAct.inCode), items: createPlaces, onselect: go } : undefined,
    };
  });
  /** The first use's create act on the empty answer, where the head would otherwise carry it. */
  const firstCreate = $derived(primary ? { label: primary.label, onclick: primary.onclick } : undefined);
  /** A hub made from a template opens where its main product is; any other on the product's screen. */
  function created(id: string, landing?: string): void {
    startingFrom = undefined;
    onnavigate(landing ?? hubScreenOf(module, id) ?? `/hubs/${id}`);
  }
</script>

{#if module === 'SERVICE_DESK'}
  <DeskHome hubs={hubsWith} {isReading} {tab} {onnavigate} {createHub} templates={starts} />
{:else}
<Stack gap="300">
  <PageHeader
    title={name}
    subtitle={t(`app.modules.hint_${module.toLowerCase()}`)}
    isTitleInBar={viewport.isCompact}
    primary={isUnusedHere ? undefined : primary}
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
    <ViewBar label={name} views={tabs} isCompact={viewport.isCompact} closeLabel={t('app.sheet_close')} current={currentTab} onnavigate={(id) => onnavigate(tabs.find((each) => each.id === id)?.href ?? '/')} />
  {/if}

  {#if currentTab === 'notes'}
    <NotesView {onnavigate} isEmbedded />
  {:else if module === 'ASSETS' && currentTab !== 'inventory'}
    {#if reachesAdministrationArea() === true}<CmdbView isEmbedded parts={currentTab === 'settings' ? ['types', 'aliases'] : [currentTab ?? 'classes']} />{/if}
  {:else if currentTab === 'roadmaps'}
    <section aria-labelledby="product-roadmaps" aria-busy={portfolios === undefined}>
      <Stack gap="100">
        <h2 class="heading" id="product-roadmaps">{t('app.product_home.tab_roadmaps')}</h2>
        {#if portfolios === undefined}
          <Skeleton lines={3} />
        {:else if portfolios.length === 0}
          <EmptyState kind="unused" icon="calendar" title={t('app.product_home.roadmaps_none_title')} description={t('app.product_home.roadmaps_none')} />
        {:else}
          <ul class="hubs">
            {#each portfolios as each (each.id)}
              {@const at = `/portfolios/${each.id}/roadmap`}
              <li>
                <ListRow href={at} onactivate={() => onnavigate(at)} label={each.name}>
                  {#snippet leading()}<Icon name="calendar" size="sm" />{/snippet}
                  <span class="name" title={each.name}>{each.name}</span>
                </ListRow>
              </li>
            {/each}
          </ul>
        {/if}
      </Stack>
    </section>
  {:else if currentTab === 'templates'}
    {#if createPlaces.length > 0}
      <BoardTemplates bind:chosen={boardStart} bind:format={boardFormat} onpick={startBoard} />
    {:else if !isReading}
      <EmptyState kind="unused" icon="layout-template" title={t('app.product_home.templates_none_title')} description={t('app.product_home.templates_none')} />
    {/if}
  {:else}
    {#if isUnusedHere && currentTab === tabs[0]?.id}
      <ProductEmpty product={module} place={{ type: 'TENANT' }}
        kind="unused"
        icon={row?.icon ?? 'hub'}
        title={t('app.product_home.unused', { product: name })}
        description={t(starts.length > 0 ? 'app.product_home.unused_body_templates' : 'app.product_home.unused_body')}
        create={firstCreate}
        templates={starts}
      />
    {/if}
    {#if assetHubs.length > 0 && Object.keys(healthAt).length > 0 && !isUnusedHere}
      <section aria-labelledby="product-inventory" aria-busy={!inventory.isWhole}>
        <Stack gap="100">
          <h2 class="heading" id="product-inventory">{t('app.product_home.inventory')}</h2>
          <dl class="figures">
            <div><dt>{t('app.assets.health_total')}</dt><dd>{inventory.total}</dd></div>
            <div><dt>{t('app.assets.problems')}</dt><dd>{inventory.wanting}</dd></div>
            <div><dt>{t('app.assets.review_overdue')}</dt><dd>{inventory.reviewOverdue}</dd></div>
          </dl>
          {#if inventory.wanting > 0}<p class="why">{t('app.assets.attention_why')}</p>{/if}
        </Stack>
      </section>
      {#if byClass.length > 0}
        <section aria-labelledby="product-classes">
          <Stack gap="100">
            <h2 class="heading" id="product-classes">{t('app.product_home.inventory_by_class')}</h2>
            <ul class="hubs">
              {#each byClass as held (held.class)}
                {@const at = `/hubs/${held.hubs[0]?.hubId ?? ''}/assets`}
                <li data-class-held={held.class}>
                  <ListRow href={at} onactivate={() => onnavigate(at)} label={classOf(held.class)}>
                    {#snippet leading()}<Icon name="shield" size="sm" />{/snippet}
                    <span class="name" title={classOf(held.class)}>{classOf(held.class)} · {held.total}</span>
                    <span class="purpose">{heldIn(held)}</span>
                  </ListRow>
                </li>
              {/each}
            </ul>
          </Stack>
        </section>
      {/if}
    {/if}
    {#if running.length > 0 && (currentTab === 'foryou' || currentTab === 'sprints')}
      <section aria-labelledby="product-sprints">
        <Stack gap="100">
          <h2 class="heading" id="product-sprints">{t('app.product_home.sprints_running')}</h2>
          <ul class="hubs">
            {#each running as each (each.id)}
              {@const where = containers.find(each.collection_id)}
              <li data-running-sprint={each.id}>
                <ListRow href={`/collections/${each.collection_id}/sprints`} onactivate={() => onnavigate(`/collections/${each.collection_id}/sprints`)} label={each.name}>
                  {#snippet leading()}<Icon name="timer" size="sm" />{/snippet}
                  <span class="name" title={each.name}>{each.name}</span>
                  <span class="purpose">{[where ? `${hubName(where.parent_id ?? '')} › ${where.name}` : '', sprintLeft(each.end_on)].filter(Boolean).join(' · ')}</span>
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if upcoming.length > 0 && (currentTab === 'foryou' || currentTab === 'releases')}
      <section aria-labelledby="product-releases">
        <Stack gap="100">
          <h2 class="heading" id="product-releases">{t('app.product_home.releases_due')}</h2>
          <ul class="hubs">
            {#each upcoming as each (each.id)}
              <li>
                <ListRow href={`/hubs/${each.hub_id}/releases`} onactivate={() => onnavigate(`/hubs/${each.hub_id}/releases`)} label={each.name}>
                  {#snippet leading()}<Icon name="flag" size="sm" />{/snippet}
                  <span class="name" title={each.name}>{each.name}</span>
                  <span class="purpose">{[hubName(each.hub_id), t('app.releases.due_on', { day: formatPlainDay(each.release_on, messages.locale) })].filter(Boolean).join(' · ')}</span>
                </ListRow>
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if toReview.length > 0 && currentTab === 'foryou'}
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
    {#if recent.length > 0 && currentTab === 'foryou'}
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
    {#if boards.length > 0 && (currentTab === 'foryou' || currentTab === 'all')}
      <section aria-labelledby="product-boards">
        <Stack gap="100">
          <h2 class="heading" id="product-boards">{t('app.product_home.boards_recent')}</h2>
          <ul class="cards">
            {#each boards as board (board.id)}
              <li>
                <BoardCard {board} meta={[hubName(board.hub_id), t('app.whiteboards.objects', { count: String(board.element_count) }), formatRelative(board.updated_at, messages.locale)].filter(Boolean).join(' · ')} onopen={onnavigate} />
              </li>
            {/each}
          </ul>
        </Stack>
      </section>
    {/if}
    {#if hubScreenOf(module, '') !== undefined && (showsHubs || (!isReading && hubsWith.length === 0))}
      <section aria-labelledby="product-hubs">
        <Stack gap="100">
          <h2 class="heading" id="product-hubs">{t('app.product_home.hubs')}</h2>
          {#if isReading && hubsWith.length === 0}
            <div aria-busy="true"><Skeleton lines={3} /></div>
          {:else if hubsWith.length === 0}
            <ProductEmpty product={module} place={{ type: 'TENANT' }}
              kind="unused"
              icon={row?.icon ?? 'hub'}
              title={t('app.product_home.none', { product: name })}
              description={t(mayCreateHub ? 'app.product_home.none_body_create' : 'app.product_home.none_body')}
              create={createHub}
              templates={starts}
            />
          {:else}
            <ul class="hubs">
              {#each hubsListed as hub (hub.id)}
                {@const path = hubScreenOf(module, hub.id) ?? `/hubs/${hub.id}`}
                {@const line = hubLine(hub.id)}
                <li>
                  <ListRow href={path} onactivate={() => onnavigate(path)} label={hub.name}>
                    {#snippet leading()}<Icon name={markOf(hub)} size="sm" />{/snippet}
                    <span class="name" title={hub.name}>{hub.name}</span>
                    {#if line}<span class="purpose" data-hub-line>{line}</span>{/if}
                    {#snippet trailing()}
                      {#if hub.figure && !hub.figure.unavailable}<CollectionFigure figure={hub.figure} name={hub.name} />{/if}
                    {/snippet}
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
{/if}

{#if isCreatingHub && mayCreateHub}
  <CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" isPrivateOnly={creation === 'private'} template={startingFrom} oncreated={created} />
{/if}

<style>
  /* Boards are visual, so they are cards (§5.1), as many to a row as fit. */
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr)); gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }
  .purpose { display: block; overflow: hidden; color: var(--text-subtle); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  /* The same figures as a hub's health section draws (lib/assets/Health.svelte). */
  .figures { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--sp-1600)), 1fr)); gap: var(--sp-100); margin: 0; }
  .figures div {
    display: grid; gap: var(--sp-025); padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface);
  }
  .figures dt { color: var(--text-secondary); font-size: var(--fs-075); }
  .figures dd { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); font-variant-numeric: tabular-nums; }
  .why { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .hubs { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  /* One line with an ellipsis and the whole name as the tooltip (user-experience.md §2.5). */
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
