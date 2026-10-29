<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Assets* in a hub (M6, UC-AST-01 … UC-AST-08): the inventory by class, the reconciliation's runs
  // with the items held, the open duplicates, a file's import, the connectors, the alerts and the
  // inventory's health, one section each at its own address; and one configuration item at `/cis/:id`.
  //
  // **What the reader may never do is not drawn** (P-05): a reader without WRITE_ITEMS on the hub
  // reads every section and is offered no write; the import section is not offered to them at all.

  import { untrack } from 'svelte';
  import { Banner, Spinner, Stack, Tabs } from '@hubtask/design-system/components';
  import type { CiClassList, CiRelationTypeList, ConfigurationItem } from '@hubtask/sync-engine';

  import Alerts from '../lib/assets/Alerts.svelte';
  import CiDetail from '../lib/assets/CiDetail.svelte';
  import Connectors from '../lib/assets/Connectors.svelte';
  import Duplicates from '../lib/assets/Duplicates.svelte';
  import Health from '../lib/assets/Health.svelte';
  import ImportFile from '../lib/assets/ImportFile.svelte';
  import Inventory from '../lib/assets/Inventory.svelte';
  import Runs from '../lib/assets/Runs.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { ASSET_SECTIONS, ciPath, classesPath, relationTypesPath, sectionOf, type Words } from '../lib/data/cmdb.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

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

  const shown = $derived(sectionOf(section));
  const tabs = $derived(
    ASSET_SECTIONS.filter((each) => each !== 'import' || mayWrite).map((each) => ({ id: each, label: t(`app.assets.section.${each}`) })),
  );

  $effect(() => frame.entitle(current?.name ?? t('app.assets.title')));
</script>

<div class="assets">
  {#if ciId}
    {#if hub}<a class="hub" href={`/hubs/${hub.id}/assets`}>{hub.name} · {t('app.assets.title')}</a>{/if}
    {#if openRefusal || !current || classesRead.state.status !== 'ready'}
      <h1 class="title">{t('app.assets.title')}</h1>
    {/if}
    {#if openRefusal}
      <Banner tone="danger">{openRefusal}</Banner>
    {:else if !current || classesRead.state.status !== 'ready'}
      <Spinner label={t('app.assets.loading')} />
    {:else}
      {#key current.id}
        <CiDetail ci={current} {classes} {types} {words} {mayWrite} {onnavigate} />
      {/key}
    {/if}
  {:else}
    <Stack gap="025">
      {#if hub}<a class="hub" href={`/hubs/${hub.id}`}>{hub.name}</a>{/if}
      <h1 class="title">{t('app.assets.title')}</h1>
    </Stack>
    {#if classesRefusal}
      <Banner tone="danger">{classesRefusal}</Banner>
    {:else if classesRead.state.status !== 'ready'}
      <Spinner label={t('app.assets.loading')} />
    {:else if hubId}
      <Tabs label={t('app.assets.sections')} {tabs} selected={shown} onselect={(id) => onnavigate(`/hubs/${hubId}/assets/${id}`)}>
        {#if shown === 'inventory'}
          <Inventory {hubId} {classes} {words} {mayWrite} {onnavigate} />
        {:else if shown === 'runs'}
          <Runs {hubId} {classes} {words} {mayWrite} />
        {:else if shown === 'duplicates'}
          <Duplicates {hubId} {classes} {words} {mayWrite} />
        {:else if shown === 'import' && mayWrite}
          <ImportFile {hubId} {classes} {words} />
        {:else if shown === 'connectors'}
          <Connectors {hubId} {classes} {words} {mayWrite} />
        {:else if shown === 'alerts'}
          <Alerts {hubId} />
        {:else if shown === 'health'}
          <Health {hubId} {classes} {words} />
        {/if}
      </Tabs>
    {/if}
  {/if}
</div>

<style>
  .assets { display: grid; gap: var(--sp-150); min-block-size: 0; }
  .title { margin: 0; font-size: var(--fs-400); font-weight: var(--fw-semibold); }
  .hub { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
