<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A family of templates as a grid of `TemplateCard`s (user-experience.md §5.2): the create
  // dialogs, a product's first use and *Administration → Templates* draw the same gallery. A list
  // with its heading as accessible name; the cards wrap to the width there is, one column on a
  // phone, and never scroll sideways.

  import type { Snippet } from 'svelte';

  import VisuallyHidden from './VisuallyHidden.svelte';

  interface Props {
    /** The family's heading (*Service*, *Agile*, *From Hubtask*), and the list's accessible name. */
    label: string;
    /** Whether the heading is drawn or only announced. */
    isLabelHidden?: boolean;
    /** The cards. */
    children: Snippet;
  }

  const { label, isLabelHidden = false, children }: Props = $props();
  const id = $props.id();
</script>

<div class="gallery">
  {#if isLabelHidden}<VisuallyHidden><h3 {id}>{label}</h3></VisuallyHidden>{:else}<h3 class="label" {id}>{label}</h3>{/if}
  <ul class="cards" aria-labelledby={id}>{@render children()}</ul>
</div>

<style>
  .gallery { display: flex; flex-direction: column; gap: var(--sp-100); }

  .label { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr));
    gap: var(--sp-150);
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
