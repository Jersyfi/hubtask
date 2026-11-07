<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import ResourceCard from './ResourceCard.svelte';
  import ResourceGrid from './ResourceGrid.svelte';
  import { templates } from './_pageFixtures.ts';

  const { isEmpty = false }: { isEmpty?: boolean } = $props();
</script>

<ResourceGrid label="Templates" {isEmpty}>
  {#each templates as template (template.id)}
    <ResourceCard
      title={template.title}
      description={template.description}
      mark={template.mark}
      chips={template.chips}
      state={'state' in template ? template.state : undefined}
      href={`#template-${template.id}`}
    />
  {/each}
  {#snippet empty()}
    <EmptyState kind="unused" title="No template saved yet" description="Save a hub as a template, and every new hub can start from it." icon="layout-template">
      {#snippet action()}<Button tone="primary" icon="plus">Save from a hub</Button>{/snippet}
    </EmptyState>
  {/snippet}
</ResourceGrid>
