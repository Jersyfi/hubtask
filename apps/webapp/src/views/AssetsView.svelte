<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Inventory* in a hub (M6, UC-AST-01 … UC-AST-13; work-surface §6.10, scene invlist): one head
  // that says how it stands, and five tabs - *Stock* (all objects, devices and licences, software),
  // *Map*, *Health*, *Alerts* with the firing count, *Upkeep* gathering the former sub-pages (IN-4).
  // Every former sub-page keeps its address under `/hubs/:id/inventory` and lands on its tab
  // (`inventoryPlace`). A work page (page-system §7.8); one object is `CiView`.
  //
  // **What the reader may never do is not drawn** (P-05): a reader without WRITE_ITEMS on the hub
  // is offered no create and no import, and *Upkeep* only to whom may run something in it. On a
  // phone the head offers no create or import: the inventory is read there (§6.10, UC-AST-01 check 10).
  import ToolHead from '../lib/workspace/ToolHead.svelte';

  import { untrack } from 'svelte';
  import { Skeleton, ViewBar, WorkPage } from '@hubtask/design-system/components';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import type { CiAlert, CiAlertList, CiClassList, CmdbHealth } from '@hubtask/sync-engine';

  import Alerts from '../lib/assets/Alerts.svelte';
  import Care from '../lib/assets/Care.svelte';
  import Health from '../lib/assets/Health.svelte';
  import MapTab from '../lib/assets/MapTab.svelte';
  import Stock from '../lib/assets/Stock.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, holdsHere, roleOrder } from '../lib/data/capability.svelte.ts';
  import {
    alertsPath, assetsFigure, classesPath, healthPath, inventoryPath, inventoryPlace, sectionOf, type AssetSection, type Words,
  } from '../lib/data/cmdb.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The hub whose inventory is shown. */
    hubId: string;
    section?: string;
    onnavigate: (path: string) => void;
    /** The home's create act asked for the create dialog (user-experience.md §5.2): opened once. */
    isCreateAsked?: boolean;
  }

  const { hubId, section, onnavigate, isCreateAsked = false }: Props = $props();

  const classesRead = resource<CiClassList>(classesPath);
  const classes = $derived(classesRead.state.status === 'ready' ? classesRead.state.data.data : []);
  const classesRefusal = $derived(classesRead.state.status === 'failed' ? renderProblem(classesRead.state.error, messages).message : undefined);

  const words: Words = (code) => (messages.has(code) ? t(code) : undefined);

  $effect(() => {
    const at = hubId;
    if (!at) return;
    return untrack(() => people.open({ hubId: at }));
  });
  const mayWrite = $derived(
    hubId ? holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted' : false,
  );

  // Opened once, when the reader's role here has answered that they may write.
  let isCreateOpened = false;
  $effect(() => {
    if (!isCreateAsked || isCreateOpened || !mayWrite) return;
    isCreateOpened = true;
    untrack(() => ask('ci'));
  });
  const mayShape = $derived(holdsHere('STRUCTURE') === true);

  // The head's figure and the alerts' count, from the same reads the health and the alerts tabs
  // draw; a refusal of either leaves its part out rather than the whole head.
  let health = $state<CmdbHealth | undefined>(undefined);
  let alerts = $state<readonly CiAlert[] | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    if (!at) return;
    return untrack(() => {
      const stops = [
        engine.subscribe<CmdbHealth>({ path: healthPath(at) }, (state) => {
          if (state.status === 'ready') health = state.data;
        }),
        engine.subscribe<CiAlertList>({ path: alertsPath(at) }, (state) => {
          if (state.status === 'ready') alerts = state.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  const standing = $derived(assetsFigure(health, alerts));
  // The objects to mark in the stock: found wanting, or an alert firing on them.
  const wanting = $derived(new Set([
    ...(health?.problems ?? []).map((each) => each.ci_id),
    ...(alerts ?? []).filter((each) => each.status === 'FIRING' && each.ci_id).map((each) => each.ci_id as string),
  ]));

  const asked0 = $derived(sectionOf(section));
  const place = $derived(inventoryPlace(asked0, { write: mayWrite, shape: mayShape }));
  const tabLabel = (id: string) => t(`app.assets.section.${id}`);
  const tabs = $derived(place.tabs.map((id) => ({
    id, label: tabLabel(id), href: inventoryPath(hubId, id), ...(id === 'alerts' && standing?.firing ? { count: standing.firing } : {}),
  })));
  const go = (id: string) => onnavigate(inventoryPath(hubId, id as AssetSection));

  // A create chosen in the head opens its tab's own dialog: the tab takes the ask once it is drawn
  // and gives it back, so returning to it later opens nothing.
  type Ask = 'ci' | 'connector' | 'group' | 'campaign';
  let asked = $state<Ask | undefined>(undefined);
  const ASK_SECTION: Record<Ask, AssetSection> = { ci: 'inventory', connector: 'connectors', group: 'inventory', campaign: 'campaigns' };
  function ask(what: Ask) {
    asked = what;
    const to = ASK_SECTION[what];
    const isThere = to === 'inventory' ? place.tab === 'inventory' && place.stock === 'all' : place.tab === 'care' && (place.care === to || asked0 === 'care');
    if (!isThere) onnavigate(inventoryPath(hubId, to));
  }
  const answered = () => (asked = undefined);

  const creates = $derived([
    ...(mayShape ? [{ id: 'connector', label: t('app.assets.connector_new'), icon: 'link' as const }] : []),
    ...(mayWrite ? [{ id: 'group', label: t('app.ci_groups.new'), icon: 'shapes' as const }] : []),
    ...(mayWrite ? [{ id: 'campaign', label: t('app.campaigns.start'), icon: 'check' as const }] : []),
  ]);

  $effect(() => frame.entitle(t('app.assets.title')));
</script>

{#snippet inventoryFigure()}
  {#if standing}
    <p class="figure" data-assets-figure>
      <span class="total">{t('app.assets.figure_total', { count: standing.total })}</span>
      {#if standing.wanting > 0}<span><span aria-hidden="true">·</span> <a href={inventoryPath(hubId, 'health')} title={t('app.assets.attention_why')}>{t('app.assets.figure_wanting', { count: standing.wanting })}</a></span>{/if}
      {#if standing.overdue > 0}<span><span aria-hidden="true">·</span> {t('app.inventory.figure_overdue', { count: standing.overdue })}</span>{/if}
      {#if standing.completeness !== undefined}<span><span aria-hidden="true">·</span> {t('app.inventory.figure_complete', { percent: String(standing.completeness) })}</span>{/if}
    </p>
  {/if}
{/snippet}
<WorkPage>
  {#snippet top()}
  {#if hubId}
    <ToolHead {hubId} tool="inventory" {onnavigate} figure={inventoryFigure}
      primary={mayWrite && !viewport.isCompact
        ? {
            label: t('app.assets.new'),
            icon: 'plus',
            onclick: () => ask('ci'),
            ...(creates.length > 0 ? { menu: { label: t('app.assets.create_more'), items: creates, onselect: (id: string) => ask(id as Ask) } } : {}),
          }
        : undefined}
      secondary={mayWrite && !viewport.isCompact ? [{ label: t('app.inventory.import'), icon: 'cloud-upload', onclick: () => onnavigate(inventoryPath(hubId, 'import')) }] : []}
    >
      {#snippet bar()}
        {#if !classesRefusal && classesRead.state.status === 'ready'}
          <ViewBar
            label={t('app.assets.sections')}
            views={tabs}
            current={place.tab}
            isCompact={viewport.isCompact}
            sheetTitle={t('app.nav.tool_inventory')}
            closeLabel={t('app.sheet_close')}
            onnavigate={go}
          />
        {/if}
      {/snippet}
    </ToolHead>
  {/if}
  {/snippet}
    {#if classesRefusal}
      <FailureNote problem={classesRefusal} />
    {:else if classesRead.state.status !== 'ready'}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if hubId}
      <div role="region" aria-label={tabLabel(place.tab)} data-inventory-tab={place.tab}>
        {#if place.tab === 'inventory'}
          <Stock {hubId} {classes} {words} {mayWrite} {onnavigate} part={place.stock} {wanting} isSavedShown={asked0 === 'groups'}
            asked={asked === 'ci' || asked === 'group' ? asked : undefined} onasked={answered} onimport={() => onnavigate(inventoryPath(hubId, 'import'))} />
        {:else if place.tab === 'map'}
          <MapTab {hubId} {classes} {words} />
        {:else if place.tab === 'health'}
          <Health {hubId} {classes} {words} />
        {:else if place.tab === 'alerts'}
          <Alerts {hubId} mayConnect={mayShape} onconnect={() => ask('connector')} />
        {:else if place.tab === 'care'}
          <Care {hubId} {classes} {words} {mayWrite} parts={place.careParts} part={place.care}
            asked={asked === 'connector' || asked === 'campaign' ? asked : undefined} onasked={answered} onimported={() => onnavigate(inventoryPath(hubId, 'inventory'))} />
        {/if}
      </div>
    {/if}
</WorkPage>

<style>
  .figure { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .figure .total { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .figure a { color: var(--text-warning); text-decoration-line: underline; text-decoration-style: dotted; text-underline-offset: var(--sp-025); }
  .figure a:hover { color: var(--text-primary); }
</style>
