<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's or a collection's tab strip under its head (user-experience.md §2.4), drawn by the
  // place's own screen and by every screen one of its tabs opens, so the reader always sees where
  // they are and can switch. A tab with several views draws them as a second, quieter strip.
  // Nothing is drawn until the place's modules have answered, and nothing where only the first
  // tab would stand: a strip of one link is no choice.
  import { untrack } from 'svelte';
  import { PlaceTabs } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { modules, modulesHere } from '../data/modules.svelte.ts';
  import { entryTypesOffered } from '../data/entrytypes.ts';
  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import { actsAsDesk } from '../portal/portal.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { placeTabs } from './placeTabs.ts';

  interface Props {
    placeId: string;
    /** The tab the screen is, and the view inside it where the tab holds several. */
    current: string;
    view?: string;
    onnavigate: (path: string) => void;
  }
  const { placeId, current, view, onnavigate }: Props = $props();

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
  // The types are read only where the portal could be a tab: a collection with the desk on.
  $effect(() => {
    if (place?.type !== 'COLLECTION' || !modulesOn.includes('SERVICE_DESK')) return;
    return untrack(() => entryTypes.open());
  });
  const isDesk = $derived(
    place?.type === 'COLLECTION'
      && actsAsDesk(entryTypes.all, entryTypesOffered(place.structure?.settings, 'TASK'), place.parent_id),
  );
  const entries = $derived(
    place && placeModules !== undefined
      ? placeTabs({ type: place.type === 'HUB' ? 'HUB' : 'COLLECTION', id: place.id, modulesOn, isDesk })
      : [],
  );
  const tabs = $derived(entries.map((entry) => ({ id: entry.id, label: t(entry.code), href: entry.href, icon: entry.icon })));
  const views = $derived(
    (entries.find((entry) => entry.id === current)?.views ?? []).map((each) => ({ id: each.id, label: t(each.code), href: each.href })),
  );
</script>

{#if place && tabs.length > 1}
  <div class="strip">
    <PlaceTabs label={place.name} {tabs} {current} onnavigate={(id) => onnavigate(tabs.find((each) => each.id === id)?.href ?? '/')} />
    {#if views.length > 1}
      <PlaceTabs
        label={tabs.find((each) => each.id === current)?.label ?? ''}
        tabs={views}
        current={view ?? views[0]?.id}
        onnavigate={(id) => onnavigate(views.find((each) => each.id === id)?.href ?? '/')}
      />
    {/if}
  </div>
{/if}

<style>
  .strip {
    display: grid;
    gap: var(--sp-050);
    min-inline-size: 0;
  }
</style>
