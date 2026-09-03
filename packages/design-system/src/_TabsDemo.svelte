<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Stack from './Stack.svelte';
  import Tabs, { type Tab } from './Tabs.svelte';

  const { mode = 'views' }: { mode?: 'views' | 'gated' | 'long' } = $props();

  const views: Tab[] = [
    { id: 'list', label: 'List' },
    { id: 'board', label: 'Board' },
    { id: 'activity', label: 'History' },
  ];

  const gated: Tab[] = [
    { id: 'list', label: 'List' },
    { id: 'board', label: 'Board' },
    {
      id: 'timeline',
      label: 'Timeline',
      disabledReason: 'The timeline needs a start date on at least one entry.',
    },
  ];

  const long: Tab[] = [
    { id: 'list', label: 'Liste' },
    { id: 'board', label: 'Pinnwandansicht' },
    { id: 'activity', label: 'Änderungsverlauf' },
    { id: 'settings', label: 'Sammlungseinstellungen' },
  ];

  let selected = $state('list');
  const tabs = $derived(mode === 'gated' ? gated : mode === 'long' ? long : views);
</script>

<Stack gap="200">
  <Tabs label="Views of this collection" {tabs} bind:selected>
    <p>The panel for “{tabs.find((tab) => tab.id === selected)?.label}”.</p>
  </Tabs>
</Stack>
