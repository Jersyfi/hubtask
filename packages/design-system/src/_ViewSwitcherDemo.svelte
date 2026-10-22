<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import InContext from './_InContext.svelte';
  import Stack from './Stack.svelte';
  import ViewSwitcher, { type View } from './ViewSwitcher.svelte';

  const { mode = 'built' }: { mode?: 'built' | 'reported' | 'long' | 'row' | 'table' } = $props();

  const withReason = (reason: string | undefined): View[] => [
    { id: 'LIST_COLLAPSED', label: 'List', icon: 'menu' },
    { id: 'TIMELINE', label: 'Timeline', icon: 'calendar', unavailableReason: reason },
    { id: 'KANBAN', label: 'Board', icon: 'bucket' },
  ];

  const built: View[] = [
    { id: 'LIST_COLLAPSED', label: 'List', icon: 'menu' },
    { id: 'LIST_EXPANDED', label: 'Outline', icon: 'chevrons-up-down' },
    { id: 'KANBAN', label: 'Board', icon: 'bucket' },
  ];

  const reported: View[] = [
    ...built,
    {
      id: 'TIMELINE',
      label: 'Timeline',
      icon: 'calendar',
      unavailableReason: 'The timeline needs start dates, which arrive with the time features.',
    },
  ];

  const long: View[] = [
    { id: 'LIST_COLLAPSED', label: 'Liste', icon: 'menu' },
    { id: 'LIST_EXPANDED', label: 'Gliederungsansicht', icon: 'chevrons-up-down' },
    { id: 'KANBAN', label: 'Pinnwandansicht', icon: 'bucket' },
  ];

  let selected = $state('LIST_COLLAPSED');
  const views = $derived(mode === 'reported' ? reported : mode === 'long' ? long : built);
</script>

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="The timeline needs start dates on at least one entry.">
    {#snippet subject({ reason })}
      <ViewSwitcher label="How these entries are shown" views={withReason(reason)} />
    {/snippet}
  </InContext>
{:else}
<Stack gap="200">
  <ViewSwitcher label="How these entries are shown" {views} bind:selected />
  <p>Showing “{views.find((view) => view.id === selected)?.label}”.</p>
</Stack>
{/if}
