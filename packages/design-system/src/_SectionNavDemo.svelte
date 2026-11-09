<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import SearchField from './SearchField.svelte';
  import SectionNav from './SectionNav.svelte';
  import { signInSections, workflows } from './_pageFixtures.ts';

  const { mode = 'settings' }: { mode?: 'settings' | 'editor' } = $props();

  let picked = $state<string | undefined>(undefined);
  const current = $derived(picked ?? (mode === 'editor' ? 'standard' : 'passwords'));
</script>

<div class="column">
  {#if mode === 'editor'}
    <SectionNav label="Workflows" sections={workflows} {current} onselect={(id) => (picked = id)}>
      {#snippet search()}<SearchField label="Find a workflow" isLabelHidden placeholder="Find a workflow" clearLabel="Clear the search" size="sm" />{/snippet}
    </SectionNav>
  {:else}
    <SectionNav label="Sections of Sign-in" sections={signInSections} {current} onselect={(id) => (picked = id)} />
  {/if}
</div>

<style>
  .column { max-inline-size: var(--layout-settings-nav); }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .column { max-inline-size: none; }
  }
</style>
