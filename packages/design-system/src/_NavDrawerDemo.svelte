<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import AppBar from './AppBar.svelte';
  import NavDrawer from './NavDrawer.svelte';
  import SideNav, { type NavNode } from './SideNav.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'tree' }: { mode?: 'tree' | 'long' } = $props();

  let isOpen = $state(false);
  let current = $state('renovation');

  // The tree the product's drawer holds below `expanded`: the hubs, their collections, and the
  // trash as the last node - what ADR-0061 decision 1 puts in the drawer on a phone. The primary
  // destinations are not here, because on a phone they are in the bar below the page.
  const tree: NavNode[] = $derived([
    {
      id: 'private',
      label: mode === 'long' ? 'Privat und Familie' : 'Private',
      icon: 'hub',
      children: [
        { id: 'shopping', label: mode === 'long' ? 'Wocheneinkauf und Vorratshaltung' : 'Shopping', icon: 'collection', href: '/c/shopping' },
        { id: 'renovation', label: mode === 'long' ? 'Renovierung der Küche im Erdgeschoss' : 'Renovation', icon: 'collection', href: '/c/renovation' },
      ],
    },
    {
      id: 'work',
      label: mode === 'long' ? 'Arbeit und Ehrenamt' : 'Work',
      icon: 'hub',
      children: [
        { id: 'hubtask', label: 'Hubtask', icon: 'collection', href: '/c/hubtask' },
        { id: 'reading', label: mode === 'long' ? 'Leseliste für das Wintersemester' : 'Reading list', icon: 'collection', href: '/c/reading' },
      ],
    },
    { id: 'trash', label: mode === 'long' ? 'Papierkorb' : 'Trash', icon: 'trash', href: '/trash' },
  ]);

  let expanded = $state(['private']);
</script>

<Stack gap="200">
  <AppBar
    label="Application"
    title={mode === 'long' ? 'Renovierung der Küche im Erdgeschoss' : 'Renovation'}
    toggle={{ kind: 'drawer', label: 'Open the navigation', isExpanded: isOpen, onToggle: () => (isOpen = !isOpen) }}
  />
  <NavDrawer bind:isOpen title="Workspace" dismissLabel="Close the navigation">
    <SideNav
      label="Hubs and collections"
      nodes={tree}
      {current}
      bind:expanded
      onnavigate={(id) => {
        current = id;
        // The caller closes it: the drawer cannot know a navigation happened, and focus returns
        // to the bar's button, where the thumb still is.
        isOpen = false;
      }}
    />
  </NavDrawer>
  <p class="state">On: {current}. The drawer is {isOpen ? 'open' : 'closed'}.</p>
</Stack>

<style>
  .state { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
</style>
