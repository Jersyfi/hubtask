<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import ProductBand from './ProductBand.svelte';
  import Button from './Button.svelte';
  import SummaryCard from './SummaryCard.svelte';
  import type { ProductId } from './ProductTile.svelte';
  import type { IconName } from './icons/index.ts';

  interface Props {
    mode?: 'desk' | 'all' | 'empty' | 'timer';
  }

  const { mode = 'desk' }: Props = $props();

  const BANDS: [ProductId, IconName, string, string[], string][] = [
    ['plans', 'calendar', 'Plans', ['1 sprint running', '2 releases planned', '1 roadmap'], 'Plan a sprint'],
    ['pages', 'book-open', 'Pages', ['10 pages', '3 spaces', '2 changed today'], 'New page'],
    ['whiteboards', 'layout-template', 'Whiteboards', ['3 whiteboards', '1 changed today'], 'New whiteboard'],
    ['desk', 'inbox', 'Service desk', ['14 open', '1 close to breach', '6 unassigned'], 'New request'],
    ['assets', 'shield', 'Inventory', ['12 items', '2 need attention'], 'New item'],
    ['time', 'clock', 'Time', ['6:00 this week', 'open'], 'Log time'],
    ['goals', 'flag', 'Goals', ['4 goals', '2 need a look', 'until 31 Dec 2026'], 'New goal'],
    ['agents', 'zap', 'Agents', ['2 agents', '0 active', '1 draft'], 'Add an agent'],
  ];
  const menu = { label: 'More for the service desk', items: [{ id: 'settings', label: 'Settings', icon: 'settings' as const }], onselect: () => {} };
  const createMenu = [{ id: 'incident', label: 'Incident' }, { id: 'request', label: 'Service request' }, { id: 'change', label: 'Change' }];
</script>

{#if mode === 'desk'}
  <ProductBand product="desk" icon="inbox" name="Service desk" figures={['14 open', '1 close to breach', '6 unassigned']} primary={{ label: 'New request', icon: 'plus', onclick: () => {}, menu: { label: 'New …', items: createMenu, onselect: () => {} } }} {menu} breadcrumb={{ label: 'Where you are', expandLabel: 'Show the whole path', trail: [{ id: 'products', label: 'Products', href: '#' }, { id: 'desk', label: 'Service desk' }] }} />
  <div class="figures">
    <SummaryCard lead="figure" title="My open" figure="3" line="1 close to breach" />
    <SummaryCard lead="figure" title="Unassigned" figure="6" line="in your groups" />
    <SummaryCard lead="figure" title="First reply kept" figure="50 %" line="last 30 days · 1 breached" />
    <SummaryCard lead="figure" title="Resolution kept" figure="100 %" line="last 30 days" />
  </div>
{:else if mode === 'all'}
  <div class="bands">
    {#each BANDS as [product, icon, name, figures, act] (product)}
      <ProductBand {product} {icon} {name} {figures} primary={{ label: act, icon: 'plus', onclick: () => {} }} />
    {/each}
  </div>
{:else if mode === 'timer'}
  <ProductBand product="time" icon="clock" name="My week" figures={['6:00 of 40:00', '9–15 November']} state={{ label: 'Open', vocabulary: 'timesheet', state: 'open' }} primary={{ label: 'Log time', icon: 'plus', onclick: () => {} }} {menu}>
    {#snippet aside()}<Button tone="subtle" icon="play" onclick={() => {}}>Start timer</Button>{/snippet}
  </ProductBand>
{:else}
  <ProductBand product="goals" icon="flag" name="Goals" figures={['No goals yet']} primary={{ label: 'New goal', icon: 'plus', onclick: () => {} }} />
{/if}

<style>
  .bands { display: flex; flex-direction: column; gap: var(--sp-400); }
  .figures { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--sp-200); margin-block-start: var(--sp-300); }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) { .figures { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
