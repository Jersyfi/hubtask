<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import BottomBar, { type Destination } from './BottomBar.svelte';
  import Input from './Input.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'four' }: { mode?: 'four' | 'keyboard' | 'long' } = $props();

  let current = $state('workspace');
  let draft = $state('');

  // The primary group of the one navigation list (ADR-0061 decision 1), and "You" for the
  // account group - the same four words on every phone.
  const destinations = $derived<Destination[]>([
    { id: 'workspace', label: mode === 'long' ? 'Arbeitsbereich' : 'Workspace', icon: 'workspace', href: '/' },
    { id: 'search', label: mode === 'long' ? 'Suche' : 'Search', icon: 'search', href: '/search' },
    { id: 'jumble', label: 'Jumble', icon: 'jumble', href: '/jumble', count: 3 },
    { id: 'you', label: mode === 'long' ? 'Du' : 'You', icon: 'user', href: '/profile' },
  ]);
</script>

<!-- The bar is fixed to the bottom of the pane's viewport, so the demo leaves room for it. -->
<div class="room">
  <Stack gap="200">
    <p class="state">On: {current}. The bar is pinned to the bottom of the pane.</p>
    {#if mode === 'keyboard'}
      <Input label="A field to focus" bind:value={draft} placeholder="Focus me: the bar gives way" />
    {/if}
  </Stack>
  <BottomBar label="Destinations" {destinations} {current} onnavigate={(id) => (current = id)} />
</div>

<style>
  .room { min-block-size: calc(var(--layout-bottombar-height) * 3); }
  .state { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
</style>
