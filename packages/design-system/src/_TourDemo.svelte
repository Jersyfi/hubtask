<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper: a small piece of real interface for the spotlight to sit on, three
  // steps written here in the register voice-and-tone.md asks for, and the tour over them.

  import { untrack } from 'svelte';

  import Button from './Button.svelte';
  import Stack from './Stack.svelte';
  import TaskRow from './TaskRow.svelte';
  import Tour from './Tour.svelte';

  const { step = 0 }: { step?: number } = $props();

  const STEPS = [
    { caption: 'Where work lives', body: 'A hub holds collections; a collection holds the entries. Everything you make sits somewhere in this tree.' },
    { caption: 'Two ways to see the same entries', body: 'The list and the board show one collection. Switch between them here; nothing moves but your view.' },
    { caption: 'Now make one', body: 'Add an entry and tick it off. That first completion is the moment this tour has been leading to.' },
  ];

  // The story's step is where the walk starts; the walk itself is this demo's state.
  let index = $state(untrack(() => step));
  let isOpen = $state(true);
  let hub = $state<HTMLElement | null>(null);
  let toggle = $state<HTMLElement | null>(null);
  let add = $state<HTMLElement | null>(null);

  const targets = $derived([hub, toggle, add]);
  const current = $derived(STEPS[index]);
</script>

<Stack gap="200">
  <nav aria-label="Workspace" bind:this={hub}>
    <ul class="tree">
      <li><a href="#errands">Errands</a></li>
      <li><a href="#launch">The launch</a></li>
    </ul>
  </nav>
  <div class="row">
    <span bind:this={toggle}><Button size="sm" tone="secondary">Board</Button></span>
    <span bind:this={add}><Button size="sm" tone="secondary">Add an entry</Button></span>
  </div>
  <TaskRow type="TASK" title="Book the venue" completeLabel="Complete" />
  {#if !isOpen}
    <div><Button size="sm" tone="secondary" onclick={() => { index = 0; isOpen = true; }}>Take the tour again</Button></div>
  {/if}
</Stack>

{#if isOpen && current}
  <Tour
    target={targets[index] ?? null}
    caption={current.caption}
    body={current.body}
    countLabel={`Step ${index + 1} of ${STEPS.length}`}
    nextLabel={index === STEPS.length - 1 ? 'Done' : 'Next'}
    backLabel="Back"
    skipLabel="Skip the tour"
    hasBack={index > 0}
    onNext={() => { if (index === STEPS.length - 1) isOpen = false; else index += 1; }}
    onBack={() => (index = Math.max(0, index - 1))}
    onSkip={() => (isOpen = false)}
  />
{/if}

<style>
  .tree { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); max-width: 24ch; }
  .tree a { color: var(--text-primary); }
  .row { display: flex; gap: var(--sp-100); }
</style>
