<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ProductTile from './ProductTile.svelte';
  import type { IconName } from './icons/index.ts';

  const products: [IconName, string, string][] = [
    ['calendar', 'orange', 'Plans'],
    ['book-open', 'teal', 'Pages'],
    ['layout-template', 'amber', 'Whiteboards'],
    ['inbox', 'magenta', 'Service desk'],
    ['shield', 'slate', 'Assets'],
    ['clock', 'lime', 'Time'],
    ['flag', 'green', 'Goals'],
    ['zap', 'violet', 'Agents'],
    ['hub', 'none', 'A hub without a colour'],
  ];
</script>

<ul class="tiles">
  {#each products as [icon, color, name] (name)}
    <li><ProductTile {icon} {color} /> {name} <ProductTile {icon} {color} size="sm" /></li>
  {/each}
</ul>

<style>
  .tiles { display: grid; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
  li { display: flex; align-items: center; gap: var(--sp-100); font-size: var(--fs-090); }
</style>
