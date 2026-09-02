<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The frame: an index on one side, the axis bar and the stage on the other. Everything it holds
  // is in the address (lib/state.svelte.ts), so "it clips in dark RTL at 200 %" is a link rather
  // than a claim.
  import AxisBar from './chrome/AxisBar.svelte';
  import FocusPanel from './chrome/FocusPanel.svelte';
  import Sidebar from './chrome/Sidebar.svelte';
  import Stage from './chrome/Stage.svelte';
  import { workbench } from './lib/state.svelte.ts';
  import { loadStories, type LoadedStory } from './lib/story.ts';

  const groups = loadStories();
  const all = groups.flatMap((group) => group.stories);

  const selected = $derived<LoadedStory | undefined>(
    all.find((story) => story.id === workbench.story) ?? all[0],
  );

  let hosts = $state<HTMLElement[]>([]);

  $effect(() => {
    const adopt = () => workbench.adopt();
    window.addEventListener('popstate', adopt);
    return () => window.removeEventListener('popstate', adopt);
  });
</script>

<svelte:head>
  <title>{selected ? `${selected.meta.title} — Hubtask Workbench` : 'Hubtask Workbench'}</title>
</svelte:head>

<div class="frame">
  <header class="top">
    <span class="wordmark">Hubtask Workbench</span>
    <span class="subtitle"
      >Every story through every rule — <code>design-system.md</code> §3, §4, §6 (ADR-0037)</span
    >
  </header>

  <div class="side">
    <Sidebar {groups} selected={selected?.id ?? null} onselect={(id) => workbench.select(id)} />
  </div>

  <main>
    {#if selected}
      <AxisBar
        state={workbench.axes}
        declared={selected.meta.axes}
        onchange={(id, value) => workbench.set(id, value)}
      />
      {#if selected.about}
        <p class="about">{selected.about}</p>
      {/if}
      <div class="stage-area">
        <Stage story={selected} axes={workbench.axes} onhosts={(next) => (hosts = next)} />
      </div>
      <FocusPanel {hosts} />
    {:else}
      <div class="nothing">
        <p>
          There is no story to show. That is the correct state today: <code>src/</code> stays empty
          until wave 1 builds it deliberately (ADR-0029), and the workbench's own specimen is a
          fixture rather than a component.
        </p>
      </div>
    {/if}
  </main>
</div>

<style>
  .frame {
    display: grid;
    grid-template-columns: 264px minmax(0, 1fr);
    grid-template-rows: auto minmax(0, 1fr);
    grid-template-areas:
      'top top'
      'side main';
    height: 100vh;
  }

  .top {
    grid-area: top;
    display: flex;
    align-items: baseline;
    gap: var(--sp-200);
    padding: var(--sp-150) var(--sp-300);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-surface);
  }

  .wordmark {
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-bold);
    letter-spacing: -0.01em;
  }

  .subtitle {
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .side {
    grid-area: side;
    display: flex;
    min-height: 0;
  }

  .side > :global(nav) {
    flex: 1;
    min-height: 0;
  }

  /* A column rather than explicit rows: the story blurb is optional, and named rows would shift
     the stage into an `auto` track the moment a story leaves it out. */
  main {
    grid-area: main;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .stage-area {
    flex: 1;
    overflow: auto;
    min-height: 0;
  }

  .about {
    margin: 0;
    padding: var(--sp-150) var(--sp-300);
    border-bottom: 1px solid var(--border-subtle);
    color: var(--text-secondary);
    max-width: 90ch;
  }

  .nothing {
    padding: var(--sp-400) var(--sp-300);
    color: var(--text-secondary);
    max-width: 70ch;
  }

  code {
    font-family: var(--font-mono);
  }
</style>
