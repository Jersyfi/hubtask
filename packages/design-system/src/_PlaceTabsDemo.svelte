<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import PlaceTabs, { type PlaceTab } from './PlaceTabs.svelte';

  const { mode = 'hub' }: { mode?: 'hub' | 'home' | 'long' } = $props();

  let current = $state('overview');

  const tabs = $derived<PlaceTab[]>(
    mode === 'home'
      ? [
          { id: 'overview', label: 'Queues', href: '#queues', count: 12 },
          { id: 'desks', label: 'Desks', href: '#desks' },
          { id: 'settings', label: 'Settings', href: '#settings' },
        ]
      : [
          { id: 'overview', label: mode === 'long' ? 'Überblick' : 'Overview', href: '#overview', icon: 'hub' },
          { id: 'plan', label: mode === 'long' ? 'Planung' : 'Plan', href: '#plan', icon: 'calendar' },
          { id: 'pages', label: mode === 'long' ? 'Seiten' : 'Pages', href: '#pages', icon: 'book-open' },
          { id: 'whiteboards', label: 'Whiteboards', href: '#whiteboards', icon: 'layout-template' },
          { id: 'desk', label: mode === 'long' ? 'Servicedesk' : 'Desk', href: '#desk', icon: 'inbox' },
          { id: 'assets', label: 'Assets', href: '#assets', icon: 'shield' },
          { id: 'time', label: mode === 'long' ? 'Zeiterfassung' : 'Time', href: '#time', icon: 'clock' },
          { id: 'goals', label: mode === 'long' ? 'Ziele' : 'Goals', href: '#goals', icon: 'flag' },
        ],
  );
</script>

<PlaceTabs label="Product development" {tabs} {current} onnavigate={(id) => (current = id)} />
<p class="state">On: {current}.</p>

<style>
  .state { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
</style>
