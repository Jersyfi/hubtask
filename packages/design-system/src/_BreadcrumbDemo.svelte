<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Breadcrumb from './Breadcrumb.svelte';
  import type { Crumb } from './structure.ts';
  import Stack from './Stack.svelte';

  const { mode = 'deep' }: { mode?: 'deep' | 'shallow' | 'long' } = $props();

  // The five levels of domain-model.md §3.4, which is the case §4 sized the collapsing for.
  const five: Crumb[] = [
    { id: 'hub', label: 'Private', href: '/hubs/private' },
    { id: 'collection', label: 'Renovation', href: '/collections/renovation' },
    { id: 'task', label: 'The kitchen', href: '/items/kitchen' },
    { id: 'package', label: 'Electrics', href: '/items/electrics' },
    { id: 'activity', label: 'Move the socket by the window' },
  ];

  const shallow: Crumb[] = [five[0]!, { id: 'collection', label: 'Renovation' }];

  // German, and every label at its longest: rule 4's growth applied to the component whose whole
  // job is to fit a path into one line.
  const long: Crumb[] = [
    { id: 'hub', label: 'Privater Arbeitsbereich', href: '/hubs/private' },
    { id: 'collection', label: 'Wohnungssanierung 2026', href: '/collections/renovation' },
    { id: 'task', label: 'Küchenumbau und Elektroinstallation', href: '/items/kitchen' },
    { id: 'package', label: 'Elektroarbeiten', href: '/items/electrics' },
    { id: 'activity', label: 'Steckdose am Fenster versetzen lassen' },
  ];
</script>

<Stack gap="200">
  {#if mode === 'shallow'}
    <Breadcrumb label="Where you are" trail={shallow} expandLabel="Show the levels in between" />
  {:else if mode === 'long'}
    <Breadcrumb label="Wo Sie sind" trail={long} expandLabel="Zwischenebenen anzeigen" />
  {:else}
    <Breadcrumb label="Where you are" trail={five} expandLabel="Show the levels in between" />
  {/if}
</Stack>
