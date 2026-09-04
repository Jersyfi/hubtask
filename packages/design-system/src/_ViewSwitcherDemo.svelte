<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Stack from './Stack.svelte';
  import ViewSwitcher, { type View } from './ViewSwitcher.svelte';

  const { mode = 'built' }: { mode?: 'built' | 'reported' | 'long' } = $props();

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

<Stack gap="200">
  <ViewSwitcher label="How these entries are shown" {views} bind:selected />
  <p>Showing “{views.find((view) => view.id === selected)?.label}”.</p>
</Stack>
