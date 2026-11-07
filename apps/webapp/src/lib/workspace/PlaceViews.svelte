<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One view bar per object (the navigation concept §3): a hub tool's own tabs, or a collection's
  // views, under the place's head - never the hub's products, which are rows of the sidebar's hub
  // level, and never a second strip. Nothing is drawn until the place's modules have answered, and
  // nothing where only one view would stand: a bar of one link is no choice.
  import { untrack, type Snippet } from 'svelte';
  import { ViewBar } from '@hubtask/design-system/components';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { views as savedViews } from '../data/views.svelte.ts';
  import { layoutsOffered } from '../entries/layouts.ts';

  import { containers } from '../data/containers.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { modules, modulesHere } from '../data/modules.svelte.ts';
  import { entryTypesOffered } from '../data/entrytypes.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { actsAsDesk } from '../portal/portal.ts';
  import { runsSprints } from '../data/sprints.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { collectionViews, toolViews } from './placeViews.ts';

  interface Props {
    placeId: string;
    /** For a hub: the tool whose tabs are drawn (`plan`, `inventory`, `time`, `goals`). */
    tool?: string;
    /** The view the screen is. */
    current: string;
    onnavigate: (path: string) => void;
    /** ＋ at the bar's end: a new view, where the screen can make one. */
    add?: { readonly label: string; readonly onselect: () => void };
    /** *Filter* and the like, at the bar's end. */
    tools?: Snippet;
  }
  const { placeId, tool, current, onnavigate, add, tools }: Props = $props();

  const place = $derived(containers.find(placeId));
  let placeModules = $state<readonly string[] | undefined>(undefined);
  $effect(() => {
    const at = placeId;
    placeModules = undefined;
    if (!at) return;
    return untrack(() => modules.here(at, (next) => {
      if (next.status === 'ready') placeModules = next.data.modules_on;
    }));
  });

  const modulesOn = $derived(modulesHere(manifest.value?.modules_on, placeModules));
  const isCollection = $derived(place?.type === 'COLLECTION');
  // The types are read only where the portal could be a view: a collection with the desk on.
  $effect(() => {
    if (!isCollection || !modulesOn.includes('SERVICE_DESK')) return;
    return untrack(() => entryTypes.open());
  });
  const isDesk = $derived(
    place?.type === 'COLLECTION' && actsAsDesk(entryTypes.all, entryTypesOffered(place.structure?.settings, 'TASK'), place.parent_id),
  );
  const sprintsAt = $derived(isCollection && modulesOn.includes('PLANNING') ? placeId : undefined);
  let sprintsRun = $state(false);
  $effect(() => {
    const at = sprintsAt;
    sprintsRun = false;
    if (!at) return;
    return untrack(() => runsSprints(at, (runs) => { sprintsRun = runs; }));
  });
  // A collection's saved views are views of its bar (§3).
  $effect(() => {
    if (!isCollection) return;
    const at = placeId;
    return untrack(() => savedViews.open(at));
  });
  const layouts = $derived(place && isCollection ? layoutsOffered(place, manifest.value) : []);
  // The budget is MANAGE_COSTS's (UC-TIM-08 check 1): no tab for anybody else (P-05).
  const mayCost = $derived(holdsHere('MANAGE_COSTS') === true || holdsHere('MANAGE_MEMBERS') === true);
  const entries = $derived(
    !place || placeModules === undefined
      ? []
      : isCollection
        ? collectionViews({
            id: placeId, modulesOn, isDesk, layouts: layouts.length > 0 ? layouts : undefined,
            runsSprints: sprintsRun || current === 'sprint',
            saved: savedViews.of(placeId).map((view) => ({ id: view.id, name: view.name })),
          })
        : toolViews(placeId, tool ?? '', { mayCost }),
  );
  const views = $derived(entries.map((entry) => ({ id: entry.id, label: 'code' in entry && entry.code ? t(entry.code) : ('name' in entry ? (entry.name ?? '') : ''), href: entry.href, icon: entry.icon })));
</script>

{#if place && (views.length > 1 || add || tools)}
  <ViewBar
    label={place.name}
    addLabel={add?.label}
    onadd={add?.onselect}
    {tools}
    {views}
    {current}
    isCompact={viewport.isCompact}
    sheetTitle={place.name}
    closeLabel={t('app.nav.close')}
    onnavigate={(id) => onnavigate(views.find((each) => each.id === id)?.href ?? '/')}
  />
{/if}
