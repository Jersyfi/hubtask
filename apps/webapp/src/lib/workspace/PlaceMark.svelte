<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script module lang="ts">
  // A trail's place mark: the tile of the hub or collection a crumb names by its id, where the
  // mockup draws it - on the trail, not before the title. A crumb that names no place draws none.
  import { containers } from '../data/containers.svelte.ts';
  import ContainerMark from './ContainerMark.svelte';

  export { placeMark };
</script>

{#snippet placeMark(crumb: { readonly id: string })}
  {@const place = containers.find(crumb.id)}
  {#if place}<ContainerMark look={place} size="xs" />{/if}
{/snippet}
