<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's *Timeline* (work-surface concept §6.3, E-16): the one address of its schedule,
  // no layout of the collection's list. Where the planning module is on it is the plan's schedule,
  // timeline or Gantt; without it the same light timeline, drawn from the entries' own dates, since
  // only the Gantt needs the module.
  import { untrack } from 'svelte';
  import { Skeleton } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { modules, modulesHere } from '../lib/data/modules.svelte.ts';
  import PlanView from './PlanView.svelte';

  interface Props {
    collectionId: string;
    openItemId?: string;
    /** A saved view whose layout is the timeline (`?view=`). */
    viewAsked?: string;
    onnavigate: (path: string) => void;
  }

  const { collectionId, openItemId, viewAsked, onnavigate }: Props = $props();

  let placeModules = $state<readonly string[] | undefined>(undefined);
  let isAnswered = $state(false);
  $effect(() => {
    const at = collectionId;
    placeModules = undefined;
    isAnswered = false;
    return untrack(() => modules.here(at, (next) => {
      if (next.status === 'ready') placeModules = next.data.modules_on;
      if (next.status === 'ready' || next.status === 'failed') isAnswered = true;
    }));
  });
  // The place's own answer decides: a hub narrows the workspace's modules (UC-INS-21 check 3).
  const isKnown = $derived(isAnswered && (placeModules !== undefined || manifest.value !== undefined));
  const isPlanned = $derived(modulesHere(manifest.value?.modules_on, placeModules).includes('PLANNING'));
</script>

{#if !isKnown}
  <div aria-busy="true"><Skeleton lines={6} /></div>
{:else}
  <PlanView planId={collectionId} part="schedule" isEntriesOnly={!isPlanned} {openItemId} {viewAsked} {onnavigate} />
{/if}
