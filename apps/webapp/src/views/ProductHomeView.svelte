<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's workspace-wide home (user-experience.md §2.2, §5.1): the one door the products band
  // opens. One anatomy for every product - the trail *Products › <product>*, the title with what
  // the product does, the views as `PlaceTabs` where it has several, then the hubs the reader may
  // read where the product is on, each opening the hub's own screen of it. A hub narrows the
  // workspace's modules (UC-INS-21 check 3), so each hub is asked which are on there.
  import { untrack } from 'svelte';
  import { Badge, Icon, ListRow, PageHeader, PlaceTabs, Skeleton, Stack, type PlaceTab } from '@hubtask/design-system/components';

  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
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
  import { createOf, homeTabs, hubScreenOf, inventorySummary, pagesToReview, recentBoards, recentPages, releasesDue } from '../lib/product/home.ts';
  import { releasesPath } from '../lib/data/releases.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { boardsPath } from '../lib/data/whiteboards.svelte.ts';
  import BoardCard from '../lib/whiteboards/BoardCard.svelte';
  import { engine } from '../lib/data/engine.ts';
  import { formatPlainDay, formatRelative } from '../lib/i18n/datetime.ts';
  import type { CmdbHealth, ConfigTemplate, ConfigTemplateList, PageList, ReleaseList, WhiteboardList } from '@hubtask/sync-engine';
  import { config } from '../lib/data/config.svelte.ts';
  import { productTemplates } from '../lib/product/templates.ts';
  import { pagesPath } from '../lib/data/pages.svelte.ts';
  import { healthPath } from '../lib/data/cmdb.svelte.ts';
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

  // The assets' settings are the administration's screen at their home: offered to whoever
  // reaches the administration, as the door before them was (P-05).
  const tabs = $derived<PlaceTab[]>(
    homeTabs(module)
      .filter((each) => !(module === 'ASSETS' && each.id === 'settings') || reachesAdministrationArea() === true)
      .map((each) => ({ id: each.id, label: t(each.code), href: each.path })),
  );
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

  const creation = $derived(hubCreationHere());
  const mayCreateHub = $derived(creation === 'shared' || creation === 'private');
  let isCreatingHub = $state(false);

  // The first visit's templates (user-experience.md §4): the catalogue's that switch this product
  // on, read only where the reader may make the hub one starts, and only while there is none.
  let catalogue = $state<readonly ConfigTemplate[]>([]);
  // The desk offers them while it has no desk, though hubs exist: a request type of the workspace
  // makes none (UC-SVC-05), and a service template's hub does.
  const isFirstVisit = $derived(mayCreateHub && !isReading && (hubsWith.length === 0 || module === 'SERVICE_DESK'));
  $effect(() => {
    if (!isFirstVisit) return;
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
  // The home's create act (UX-2): the product's own words, leading to the hub's screen of it -
  // the first hub's, the others in its menu. Without a hub the empty state's *Create hub* is it.
  const createAct = $derived(createOf(module));
  const primary = $derived.by(() => {
    if (!createAct || isReading || currentTab !== tabs[0]?.id) return undefined;
    const first = hubsWith[0];
    if (!first) return undefined;
    const go = (id: string) => onnavigate(hubScreenOf(module, id) ?? `/hubs/${id}`);
    return {
      label: t(createAct.code),
      icon: 'plus' as const,
      onclick: () => go(first.id),
      menu: hubsWith.length > 1
        ? { label: t(createAct.inCode), items: hubsWith.map((hub) => ({ id: hub.id, label: hub.name })), onselect: go }
        : undefined,
    };
  });
  /** A hub made from a template opens on its *Products* section with that template's plan. */
  function created(id: string): void {
    const from = startingFrom;
    startingFrom = undefined;
    onnavigate(from ? `/hubs/${id}/settings/modules?template=${encodeURIComponent(from)}` : (hubScreenOf(module, id) ?? `/hubs/${id}`));
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

  {#if tabs.length > 1}
    <PlaceTabs label={name} {tabs} current={currentTab} onnavigate={(id) => onnavigate(tabs.find((each) => each.id === id)?.href ?? '/')} />
  {/if}

  {#if currentTab === 'notes'}
    <NotesView {onnavigate} isEmbedded />
  {:else if currentTab === 'settings' && module === 'ASSETS'}
    {#if reachesAdministrationArea() === true}<CmdbView isEmbedded />{/if}
  {:else}
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
                  <span class="purpose">{[hubName(each.hub_id), t('app.releases.due_on', { day: formatPlainDay(each.release_on, messages.locale) })].filter(Boolean).join(' · ')}</span>
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
                <BoardCard {board} meta={[hubName(board.hub_id), t('app.whiteboards.objects', { count: String(board.element_count) }), formatRelative(board.updated_at, messages.locale)].filter(Boolean).join(' · ')} onopen={onnavigate} />
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
            <ProductEmpty
              kind="unused"
              icon={row?.icon ?? 'hub'}
              title={t('app.product_home.none', { product: name })}
              description={t(mayCreateHub ? 'app.product_home.none_body_create' : 'app.product_home.none_body')}
              create={createHub}
              templates={starts}
            />
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
{/if}

{#if isCreatingHub && mayCreateHub}
  <CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" isPrivateOnly={creation === 'private'} oncreated={created} />
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
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .hubs { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); margin: 0; padding: 0; list-style: none; }
  /* One line with an ellipsis and the whole name as the tooltip (user-experience.md §2.5). */
  .name { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
