<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Box from './Box.svelte';
  import SideNav, { type NavNode } from './SideNav.svelte';

  const { mode = 'tree', isRail = false, opened }: { mode?: 'tree' | 'flat' | 'long' | 'bands'; isRail?: boolean; opened?: string } = $props();

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
  const nodes = $derived(mode === 'flat' ? flat : mode === 'long' ? long : mode === 'bands' ? bands : tree);
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
