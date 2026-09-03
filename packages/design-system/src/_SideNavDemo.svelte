<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Box from './Box.svelte';
  import SideNav, { type NavNode } from './SideNav.svelte';

  const { mode = 'tree' }: { mode?: 'tree' | 'flat' | 'long' } = $props();

  // Hubs holding collections: the two-level container tree of domain-model.md §3.3, which is the
  // shape F2-08 will hand this component for real.
  const tree: NavNode[] = [
    {
      id: 'private',
      label: 'Private',
      icon: 'hub',
      children: [
        { id: 'shopping', label: 'Shopping', icon: 'collection', href: '/c/shopping' },
        { id: 'renovation', label: 'Renovation', icon: 'collection', href: '/c/renovation' },
      ],
    },
    {
      id: 'work',
      label: 'Work',
      icon: 'hub',
      children: [
        { id: 'hubtask', label: 'Hubtask', icon: 'collection', href: '/c/hubtask' },
        { id: 'admin', label: 'Administration', icon: 'collection', href: '/c/admin' },
        { id: 'reading', label: 'Reading list', icon: 'collection', href: '/c/reading' },
      ],
    },
    { id: 'jumble', label: 'Jumble', icon: 'jumble', href: '/jumble' },
  ];

  const flat: NavNode[] = [
    { id: 'jumble', label: 'Jumble', icon: 'jumble', href: '/jumble' },
    { id: 'today', label: 'Today', icon: 'calendar', href: '/today' },
    { id: 'trash', label: 'Trash', icon: 'trash-2', href: '/trash' },
  ];

  const long: NavNode[] = [
    {
      id: 'private',
      label: 'Privater Arbeitsbereich',
      icon: 'hub',
      children: [
        { id: 'renovation', label: 'Wohnungssanierung 2026', icon: 'collection', href: '/c/r' },
        { id: 'shopping', label: 'Wocheneinkaufsliste', icon: 'collection', href: '/c/s' },
      ],
    },
  ];

  let expanded = $state(['private', 'work']);
  const nodes = $derived(mode === 'flat' ? flat : mode === 'long' ? long : tree);
</script>

<!-- No inline `style` on the primitive: ADR-0028's `style-src` has no `'unsafe-inline'`, so a
     width written there is a rule the browser refuses — silently, and in production only. The
     demo's own stylesheet is where a demo's layout belongs. -->
<div class="pane">
  <Box padding="100">
    <SideNav label="Workspace" {nodes} current="renovation" bind:expanded />
  </Box>
</div>

<style>
  .pane { max-width: 32ch; }
</style>
