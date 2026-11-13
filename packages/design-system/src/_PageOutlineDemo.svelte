<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import PageOutline from './PageOutline.svelte';

  interface Props {
    mode?: 'page';
  }

  const { mode = 'page' }: Props = $props();
  const headings = [
    { id: 'outline-blocks', label: 'Building blocks', level: 2 },
    { id: 'outline-decisions', label: 'Decisions', level: 2 },
    { id: 'outline-passkeys', label: 'Passkeys before passwords', level: 3 },
    { id: 'outline-open', label: 'Open points', level: 2 },
    { id: 'outline-glossary', label: 'Glossary', level: 2 },
  ];
</script>

<div class="page" data-mode={mode}>
  <div class="text">
    {#each headings as heading (heading.id)}
      <h2 id={heading.id}>{heading.label}</h2>
      <p>The outline follows the reading: the heading whose top has passed the upper third is marked.</p>
    {/each}
  </div>
  <PageOutline label="On this page" {headings} />
</div>

<style>
  .page { display: grid; grid-template-columns: minmax(0, 1fr) var(--layout-rail); gap: var(--sp-300); align-items: start; }
  .text { display: grid; gap: var(--sp-600); }
</style>
