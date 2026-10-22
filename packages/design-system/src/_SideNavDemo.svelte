<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Box from './Box.svelte';
  import SideNav, { type NavNode } from './SideNav.svelte';

  const { mode = 'tree', isRail = false, opened }: { mode?: 'tree' | 'flat' | 'long' | 'bands' | 'figures' | 'colours'; isRail?: boolean; opened?: string } = $props();

  // Hubs holding collections: the two-level container tree of domain-model.md §3.3, which is the
  // shape the application hands this component for real.
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
    { id: 'trash', label: 'Trash', icon: 'trash', href: '/trash' },
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
    // A row whose label is the explanation of the place, the way the installation's is in the
    // administration: it wraps to a second line rather than losing its end.
    { id: 'instance', label: 'Gilt für jeden Arbeitsbereich, den Sie anlegen', icon: 'gauge', href: '/instance' },
  ];

  // The same list in bands: the places, the workspace's own structure under its caption, and a
  // band with no caption at all - which is what the foot of a navigation is, where the hairline
  // says "a different kind of thing" and there is nothing to call the group.
  const bands: NavNode[] = [
    { id: 'overview', label: 'Overview', icon: 'workspace', href: '/' },
    { id: 'jumble', label: 'Jumble', icon: 'jumble', href: '/jumble' },
    { ...tree[0], band: { caption: 'Hubs' } } as NavNode,
    tree[1] as NavNode,
    { id: 'trash', label: 'Trash', icon: 'trash', href: '/trash', band: {} },
  ];

  // Each collection with the figure that counts for it, the way the navigation draws them: a
  // count, a sum, a share, a week - and one with none, which draws nothing in its place.
  const figures: NavNode[] = [
    {
      id: 'private',
      label: 'Private',
      icon: 'hub',
      children: [
        { id: 'shopping', label: 'Shopping', icon: 'collection', href: '/c/shopping', meta: '5 open' },
        { id: 'renovation', label: 'Renovation of the kitchen and the hallway', icon: 'collection', href: '/c/renovation', meta: '38 %' },
        { id: 'household', label: 'Household', icon: 'collection', href: '/c/household', meta: '3 due' },
        { id: 'notes', label: 'Notes', icon: 'collection', href: '/c/notes' },
      ],
    },
  ];

  // Places in the colour and with the symbol their people chose (UC-WRK-33), beside one nobody
  // chose for, which keeps its type's mark and no tile.
  const colours: NavNode[] = [
    {
      id: 'private',
      label: 'House',
      icon: 'house',
      colorToken: 'magenta',
      children: [
        { id: 'shopping', label: 'Shopping', icon: 'shopping-cart', colorToken: 'green', href: '/c/shopping' },
        { id: 'renovation', label: 'Renovation', icon: 'hammer', colorToken: 'amber', href: '/c/renovation' },
        { id: 'notes', label: 'Notes', icon: 'collection', href: '/c/notes' },
      ],
    },
    { id: 'work', label: 'Agency', icon: 'briefcase', colorToken: 'violet', href: '/h/agency' },
    { id: 'garden', label: 'Garden', icon: 'hub', href: '/h/garden' },
  ];

  let expanded = $state(['private', 'work']);
  let nav = $state<HTMLElement | null>(null);

  // The story opens one branch's flyout so that the shape can be looked at, the way a menu story
  // shows the menu rather than the button. It presses the row, because the flyout's state is the
  // component's own and a prop for it would be state as a prop.
  $effect(() => {
    if (!opened || !nav) return;
    const row = nav.querySelector(`[data-node="${opened}"]`);
    if (row instanceof HTMLElement) row.click();
  });
  const nodes = $derived(
    mode === 'flat' ? flat : mode === 'long' ? long : mode === 'bands' ? bands : mode === 'figures' ? figures : mode === 'colours' ? colours : tree,
  );
</script>

<!-- No inline `style` on the primitive: ADR-0028's `style-src` has no `'unsafe-inline'`, so a
     width written there is a rule the browser refuses — silently, and in production only. The
     demo's own stylesheet is where a demo's layout belongs. -->
<div class="pane" data-rail={isRail ? '' : undefined} bind:this={nav}>
  <Box padding="100">
    <SideNav
      label="Workspace"
      {nodes}
      current="renovation"
      {isRail}
      flyoutLabel={(name) => `Inside ${name}`}
      branchLabel={(name, isOpen) => (isOpen ? `Hide what is in ${name}` : `Show what is in ${name}`)}
      bind:expanded
    />
  </Box>
</div>

<style>
  .pane { max-width: 32ch; }

  /* The column the frame gives the fold, so the story is measured against the real width. */
  .pane[data-rail] { max-width: var(--layout-sidenav-rail); }
</style>
