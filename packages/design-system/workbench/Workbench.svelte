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
    <!-- This page is public (ADR-0038), so it says what it is rather than leaving a reader to
         infer it: a development tool showing parts that mostly do not exist yet. The stage word
         is ADR-0035's vocabulary, and the obligation is the one ADR-0035 put on the
         application's maturity banner, applied to the surface that shows the parts. -->
    <span class="stage">experimental</span>
    <span class="subtitle">
      A development tool for building <strong>Hubtask</strong> — every story through every rule
      (<code>design-system.md</code> §3, §4, §6). Components appear, move and disappear without
      notice; nothing here is a promise about the product.
    </span>
    <a class="repo" href="https://github.com/Jersyfi/hubtask" rel="noreferrer">Source</a>
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
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--sp-150);
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
    max-width: 88ch;
  }

  /* Rule 3: the stage is a word before it is a tint. */
  .stage {
    padding: 0 var(--sp-050);
    border-radius: var(--r-sm);
    background: var(--label-amber-bg);
    color: var(--label-amber-fg);
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .repo {
    margin-inline-start: auto;
    color: var(--text-brand);
    font-size: var(--fs-075);
    white-space: nowrap;
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

  /* Below `--bp-medium` the index sits above the stage instead of beside it: a 264 px column and
     a stage do not both fit, and the result is a page that scrolls sideways - the one thing the
     workbench exists to catch. The width is written out because a media query cannot read a
     custom property; the value is `primitive.breakpoint.medium` minus one, and nothing else here
     is allowed to name a length twice. */
  @media (max-width: 599px) {
    .frame {
      grid-template-columns: minmax(0, 1fr);
      grid-template-areas:
        'top'
        'side'
        'main';
      grid-template-rows: auto auto minmax(0, 1fr);
    }

    .side {
      max-height: 30vh;
      border-bottom: 1px solid var(--border-subtle);
    }
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
