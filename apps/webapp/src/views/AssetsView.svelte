<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Assets* in a hub (M6, UC-AST-01 … UC-AST-13; user-experience.md §7.3): the inventory first
  // under a head that says how it stands, then its health, the alerts and the groups with the
  // service map; the operator's sections - runs, duplicates, connectors, campaigns, register,
  // software, a file's import - behind *Manage* as a second strip. Each section has its own
  // address; one configuration item is at `/cis/:id`.
  //
  // **What the reader may never do is not drawn** (P-05): a reader without WRITE_ITEMS on the hub
  // reads every section and is offered no write; the import section is not offered to them at all.

  import { untrack } from 'svelte';
  import { Badge, Banner, PageHeader, Skeleton, Spinner, Tabs } from '@hubtask/design-system/components';
  import type { CiAlert, CiAlertList, CiClassList, CiRelationTypeList, CmdbHealth, ConfigurationItem } from '@hubtask/sync-engine';

  import Alerts from '../lib/assets/Alerts.svelte';
  import Campaigns from '../lib/assets/Campaigns.svelte';
  import Groups from '../lib/assets/Groups.svelte';
  import Register from '../lib/assets/Register.svelte';
  import Software from '../lib/assets/Software.svelte';
  import CiDetail from '../lib/assets/CiDetail.svelte';
  import Connectors from '../lib/assets/Connectors.svelte';
  import Duplicates from '../lib/assets/Duplicates.svelte';
  import Health from '../lib/assets/Health.svelte';
  import ImportFile from '../lib/assets/ImportFile.svelte';
  import Inventory from '../lib/assets/Inventory.svelte';
  import Runs from '../lib/assets/Runs.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, holdsHere, roleOrder } from '../lib/data/capability.svelte.ts';
  import {
    alertsPath, assetStrips, assetsFigure, ciPath, classesPath, healthPath, relationTypesPath, sectionOf, type AssetSection, type Words,
  } from '../lib/data/cmdb.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import PlaceStrip from '../lib/workspace/PlaceStrip.svelte';

  interface Props {
    /** The hub whose assets are shown; absent when the address names one CI. */
    hubId?: string;
    section?: string;
    ciId?: string;
    onnavigate: (path: string) => void;
  }

  const { hubId: hubFromRoute, section, ciId, onnavigate }: Props = $props();

  const opened = untrack(() => (ciId ? resource<ConfigurationItem>(ciPath(ciId)) : undefined));
  const current = $derived(opened?.state.status === 'ready' ? opened.state.data : undefined);
  const openRefusal = $derived(opened?.state.status === 'failed' ? renderProblem(opened.state.error, messages).message : undefined);
  const hubId = $derived(hubFromRoute ?? current?.hub_id ?? '');
  const hub = $derived(hubId ? containers.find(hubId) : undefined);

  const classesRead = resource<CiClassList>(classesPath);
  const typesRead = resource<CiRelationTypeList>(relationTypesPath);
  const classes = $derived(classesRead.state.status === 'ready' ? classesRead.state.data.data : []);
  const types = $derived(typesRead.state.status === 'ready' ? typesRead.state.data.data : []);
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
  const mayShape = $derived(holdsHere('STRUCTURE') === true);

  // The head's figure, from the same reads the health and the alerts sections draw; a refusal of
  // either leaves its part out rather than the whole head.
  let health = $state<CmdbHealth | undefined>(undefined);
  let alerts = $state<readonly CiAlert[] | undefined>(undefined);
  $effect(() => {
    const at = hubFromRoute;
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

  const strips = $derived(assetStrips(sectionOf(section), { write: mayWrite, shape: mayShape }));
  const shown = $derived(strips.shown);
  const label = (id: string) => t(`app.assets.section.${id}`);
  const go = (id: string) => onnavigate(`/hubs/${hubId}/assets/${id === 'manage' ? strips.manage[0] : id}`);

  // A create chosen in the head opens its section's own dialog: the section takes the ask once it
  // is drawn and gives it back, so returning to the section later opens nothing.
  type Ask = 'ci' | 'connector' | 'group' | 'campaign';
  let asked = $state<Ask | undefined>(undefined);
  const ASK_SECTION: Record<Ask, AssetSection> = { ci: 'inventory', connector: 'connectors', group: 'groups', campaign: 'campaigns' };
  function ask(what: Ask) {
    asked = what;
    if (shown !== ASK_SECTION[what]) onnavigate(`/hubs/${hubId}/assets/${ASK_SECTION[what]}`);
  }
  const answered = () => (asked = undefined);

  const creates = $derived([
    ...(mayWrite ? [{ id: 'import', label: t('app.assets.create_import'), icon: 'cloud-upload' as const }] : []),
    ...(mayShape ? [{ id: 'connector', label: t('app.assets.connector_new'), icon: 'link' as const }] : []),
    ...(mayWrite ? [{ id: 'group', label: t('app.ci_groups.new'), icon: 'shapes' as const }] : []),
    ...(mayWrite ? [{ id: 'campaign', label: t('app.campaigns.start'), icon: 'check' as const }] : []),
  ]);
  function choose(id: string) {
    if (id === 'import') onnavigate(`/hubs/${hubId}/assets/import`);
    else ask(id as Ask);
  }

  $effect(() => frame.entitle(current?.name ?? t('app.assets.title')));
</script>

<div class="assets">
  {#if ciId}
    {#if openRefusal || !current || classesRead.state.status !== 'ready'}
      <h1 class="title">{t('app.assets.title')}</h1>
    {/if}
    {#if openRefusal}
      <Banner tone="danger">{openRefusal}</Banner>
    {:else if !current || classesRead.state.status !== 'ready'}
      <Spinner label={t('app.assets.loading')} />
    {:else}
      {#key current.id}
        <CiDetail ci={current} {classes} {types} {words} {mayWrite} {onnavigate} hub={hub ? { id: hub.id, name: hub.name } : undefined} />
      {/key}
    {/if}
  {:else}
    <PageHeader
      title={t('app.assets.title')}
      isTitleInBar={viewport.isCompact}
      breadcrumb={hub
        ? {
            trail: [{ id: 'place', label: hub.name, href: `/hubs/${hub.id}` }, { id: 'assets', label: t('app.assets.title') }],
            label: t('app.assets.trail'),
            expandLabel: t('app.admin.expand_trail'),
            onnavigate: () => onnavigate(`/hubs/${hubId}`),
          }
        : undefined}
      primary={mayWrite
        ? {
            label: t('app.assets.new'),
            icon: 'plus',
            onclick: () => ask('ci'),
            menu: { label: t('app.assets.create_more'), items: creates, onselect: choose },
          }
        : undefined}
    >
      {#snippet figure()}
        {#if standing}
          <p class="figure" data-assets-figure>
            <span>{t('app.assets.figure_total', { count: standing.total })}</span>
            {#if standing.wanting > 0}<span><span aria-hidden="true">·</span> <a href={`/hubs/${hubId}/assets/health`} title={t('app.assets.attention_why')}>{t('app.assets.figure_wanting', { count: standing.wanting })}</a></span>{/if}
            {#if standing.firing}<Badge size="sm" tone="danger" icon="bell">{t('app.assets.figure_firing', { count: standing.firing })}</Badge>{/if}
          </p>
        {/if}
      {/snippet}
    </PageHeader>
    {#if hubId}<PlaceStrip placeId={hubId} current="assets" {onnavigate} />{/if}
    {#if classesRefusal}
      <Banner tone="danger">{classesRefusal}</Banner>
    {:else if classesRead.state.status !== 'ready'}
      <div aria-busy="true"><Skeleton lines={4} /></div>
    {:else if hubId}
      {#snippet body()}
        {#if shown === 'inventory'}
          <Inventory {hubId} {classes} {words} {mayWrite} {onnavigate} isAsked={asked === 'ci'} onasked={answered} onimport={() => onnavigate(`/hubs/${hubId}/assets/import`)} />
        {:else if shown === 'runs'}
          <Runs {hubId} {classes} {words} {mayWrite} />
        {:else if shown === 'duplicates'}
          <Duplicates {hubId} {classes} {words} {mayWrite} />
        {:else if shown === 'import' && mayWrite}
          <ImportFile {hubId} {classes} {words} onopen={() => onnavigate(`/hubs/${hubId}/assets`)} />
        {:else if shown === 'connectors'}
          <Connectors {hubId} {classes} {words} {mayWrite} isAsked={asked === 'connector'} onasked={answered} />
        {:else if shown === 'alerts'}
          <Alerts {hubId} mayConnect={mayShape} onconnect={() => ask('connector')} />
        {:else if shown === 'health'}
          <Health {hubId} {classes} {words} />
        {:else if shown === 'campaigns'}
          <Campaigns {hubId} {classes} {words} {mayWrite} isAsked={asked === 'campaign'} onasked={answered} />
        {:else if shown === 'groups'}
          <Groups {hubId} {classes} {words} {mayWrite} isAsked={asked === 'group'} onasked={answered} />
        {:else if shown === 'register'}
          <Register {hubId} {classes} {words} />
        {:else if shown === 'software'}
          <Software {hubId} />
        {/if}
      {/snippet}
      <Tabs label={t('app.assets.sections')} tabs={strips.top.map((id) => ({ id, label: label(id) }))} selected={strips.selected} onselect={go}>
        {#if strips.selected === 'manage'}
          <div class="manage" data-assets-manage>
            <Tabs label={t('app.assets.manage_sections')} tabs={strips.manage.map((id) => ({ id, label: label(id) }))} selected={shown} onselect={go}>
              {@render body()}
            </Tabs>
          </div>
        {:else}
          {@render body()}
        {/if}
      </Tabs>
    {/if}
  {/if}
</div>

<style>
  .assets { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-200); min-block-size: 0; }
  .figure { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .figure a { color: inherit; text-decoration-line: underline; text-decoration-style: dotted; text-underline-offset: var(--sp-025); }
  .figure a:hover { color: var(--text-primary); }
  .manage { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-150); }
  .title { margin: 0; font-size: var(--fs-400); font-weight: var(--fw-semibold); }
</style>
