<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { untrack } from 'svelte';
  // The mockup's phone screens (§7): Browse, a hub, its queues, contents with a separate ›,
  // reorder mode - one stack, walked by its rows.
  import Avatar from './Avatar.svelte';
  import BrowseStack, { type StackLevel } from './BrowseStack.svelte';
  import IconButton from './IconButton.svelte';
  import NavGroup from './NavGroup.svelte';
  import NavItem from './NavItem.svelte';

  const { mode = 'browse' }: { mode?: 'browse' | 'hub' | 'queues' | 'contents' | 'reorder' } = $props();

  const start: Record<string, StackLevel[]> = {
    browse: [{ id: 'browse', title: 'Browse' }],
    hub: [{ id: 'browse', title: 'Hubs' }, { id: 'it', title: 'IT operations' }],
    queues: [{ id: 'browse', title: 'Hubs' }, { id: 'it', title: 'IT operations' }, { id: 'queues', title: 'Queues' }],
    contents: [{ id: 'browse', title: 'Hubs' }, { id: 'it', title: 'IT operations' }, { id: 'contents', title: 'Contents' }],
    reorder: [{ id: 'browse', title: 'Hubs' }, { id: 'it', title: 'IT operations' }, { id: 'contents', title: 'Contents' }],
  };
  let levels = $state<StackLevel[]>(untrack(() => start[mode]!));
  let isReordering = $state(untrack(() => mode === 'reorder'));
  const top = $derived(levels[levels.length - 1]!.id);
  const push = (id: string, title: string) => (levels = [...levels, { id, title }]);
</script>

<div class="phone">
  <BrowseStack
    {levels}
    backLabel={levels[levels.length - 2]?.title}
    onback={() => (levels = levels.slice(0, -1))}
    announce={(title, depth) => `${title}, level ${depth}`}
  >
    {#snippet rootStart()}<Avatar name="Jérôme Winkel" size="sm" />{/snippet}
    {#snippet end()}
      {#if levels.length === 1}
        <IconButton icon="bell" label="Notices" size="sm" tone="subtle" />
      {:else if isReordering}
        <button type="button" class="done" onclick={() => (isReordering = false)}>Done</button>
      {:else}
        <IconButton icon="star" label="Star" size="sm" tone="subtle" />
        <IconButton icon="ellipsis" label="More" size="sm" tone="subtle" />
      {/if}
    {/snippet}
    {#if top === 'browse'}
      <NavGroup label="Starred" size="touch">
        <NavItem label="Web app" icon="collection" color="blue" href="#web" figure="Sprint 42" size="touch" />
        <NavItem label="About to breach · IT" icon="columns-3" href="#q" figure="1" isFigureUrgent size="touch" />
      </NavGroup>
      <NavGroup label="Products" size="touch">
        <NavItem label="Plans" icon="calendar" color="orange" href="#plans" size="touch" isDrillDown />
        <NavItem label="Pages" icon="book-open" color="teal" href="#pages" size="touch" isDrillDown />
        <NavItem label="Service desk" icon="inbox" color="magenta" href="#desk" figure="2" isFigureUrgent size="touch" isDrillDown />
        <NavItem label="5 more" icon="ellipsis" href="#more" size="touch" isDrillDown />
      </NavGroup>
      <NavGroup label="Hubs" size="touch">
        <NavItem label="IT operations" icon="hub" color="teal" href="#it" size="touch" isDrillDown onnavigate={() => push('it', 'IT operations')} />
        <NavItem label="Product development" icon="hub" color="orange" href="#pe" size="touch" isDrillDown />
      </NavGroup>
    {:else if top === 'it'}
      <NavGroup label="Tools" size="touch">
        <NavItem label="Overview" icon="hub" href="#ov" size="touch" isDrillDown />
        <NavItem label="Queues" icon="inbox" href="#queues" figure="1" isFigureUrgent size="touch" isDrillDown onnavigate={() => push('queues', 'Queues')} />
        <NavItem label="Inventory" icon="shield" href="#inv" figure="6 !" isFigureUrgent size="touch" isDrillDown />
        <NavItem label="Goals" icon="flag" href="#goals" size="touch" isDrillDown />
      </NavGroup>
      <NavGroup label="Collections" size="touch">
        <NavItem label="Service desk" icon="inbox" color="magenta" href="#sd" figure="1 SLA" isFigureUrgent size="touch" />
        <NavItem label="Changes" icon="collection" color="blue" href="#ch" figure="4" size="touch" />
        <NavItem label="Problems" icon="collection" color="blue" href="#pr" figure="2" size="touch" />
      </NavGroup>
      <NavGroup label="Contents" size="touch" headLink={{ label: 'All', onselect: () => push('contents', 'Contents') }}>
        <NavItem label="Knowledge base" icon="file-text" href="#kb" size="touch" descendLabel="Open the pages under Knowledge base" ondescend={() => push('contents', 'Knowledge base')} />
        <NavItem label="Runbooks" icon="file-text" href="#rb" size="touch" descendLabel="Open the pages under Runbooks" ondescend={() => push('contents', 'Runbooks')} />
        <NavItem label="Network map" icon="layout-template" href="#map" size="touch" />
      </NavGroup>
    {:else if top === 'queues'}
      <NavGroup label="Queues" size="touch">
        <NavItem label="About to breach" icon="star" href="#q1" figure="1" isFigureUrgent size="touch" isDrillDown />
        <NavItem label="Mine" icon="columns-3" href="#q2" figure="3" size="touch" isDrillDown />
        <NavItem label="Unassigned" icon="columns-3" href="#q3" figure="2" size="touch" isDrillDown />
        <NavItem label="Unhappy" icon="columns-3" href="#q4" figure="0" size="touch" isDrillDown />
        <NavItem label="All open" icon="columns-3" href="#q5" figure="14" size="touch" isDrillDown />
      </NavGroup>
    {:else}
      <NavGroup label="Contents" size="touch">
        {#each ['Knowledge base', 'Runbooks', 'How we work', 'Suppliers'] as name (name)}
          <NavItem label={name} icon="file-text" href="#p" size="touch" {isReordering} />
        {/each}
        <NavItem label="Network map" icon="layout-template" href="#map" size="touch" {isReordering} />
      </NavGroup>
    {/if}
  </BrowseStack>
</div>

<style>
  .phone {
    display: grid;
    grid-template-rows: 1fr;
    inline-size: calc(var(--layout-board-column) + var(--sp-250));
    block-size: calc(var(--layout-panel-width) - var(--sp-250));
    overflow: hidden;
    border: var(--sp-100) solid var(--text-primary);
    border-radius: calc(var(--r-2xl) + var(--sp-150));
    background: var(--bg-canvas);
  }

  .done {
    border: 0;
    background: none;
    color: var(--text-brand);
    font: inherit;
    font-weight: var(--fw-semibold);
    cursor: pointer;
  }

  .done:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
