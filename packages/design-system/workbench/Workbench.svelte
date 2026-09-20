<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The frame: the bar, the index beside or over the stage, and the stage as the document.
  // Everything it holds is in the address (lib/state.svelte.ts), so "it clips in dark RTL at
  // 200 %" is a link rather than a claim.
  //
  // It is the first consumer of the shell wave (ADR-0061 decision 5): `AppBar` and `NavDrawer`
  // carry the tool that tests them before any product screen does. Two things this frame no
  // longer does, because both hid the story on a phone: it is not `100vh` tall, and the stage is
  // not a box that scrolls inside it. The page scrolls, the bar stays, and the story takes the
  // height it needs.
  import AppBar from '../src/AppBar.svelte';
  import NavDrawer from '../src/NavDrawer.svelte';
  import Tabs, { type Tab } from '../src/Tabs.svelte';
  import AxisChips from './chrome/AxisChips.svelte';
  import FocusPanel from './chrome/FocusPanel.svelte';
  import Sidebar from './chrome/Sidebar.svelte';
  import Stage from './chrome/Stage.svelte';
  import { componentOf } from './lib/index.ts';
  import { workbench } from './lib/state.svelte.ts';
  import { loadStories, type LoadedStory } from './lib/story.ts';

  const groups = loadStories();
  const all = groups.flatMap((group) => group.stories);

  const selected = $derived<LoadedStory | undefined>(
    all.find((story) => story.id === workbench.story) ?? all[0],
  );
  const component = $derived(componentOf(groups, selected?.id ?? null));
  const tabs = $derived<Tab[]>(
    (component?.stories ?? []).map((story) => ({ id: story.id, label: story.name })),
  );

  let hosts = $state<HTMLElement[]>([]);
  let isIndexOpen = $state(false);

  // Below `expanded` the index is a drawer over the page; above it, a column beside the stage.
  // The same width the frame's stylesheet switches at (`primitive.breakpoint.expanded`), read
  // here because the bar has to know which control to draw.
  let isNarrow = $state(false);
  $effect(() => {
    // design-system-lint-ignore: `primitive.breakpoint.expanded` less one; a media query cannot read a custom property.
    const query = window.matchMedia('(max-width: 904px)');
    const apply = () => (isNarrow = query.matches);
    apply();
    query.addEventListener('change', apply);
    return () => query.removeEventListener('change', apply);
  });

  $effect(() => {
    const adopt = () => workbench.adopt();
    window.addEventListener('popstate', adopt);
    return () => window.removeEventListener('popstate', adopt);
  });

  function choose(id: string) {
    workbench.select(id);
    isIndexOpen = false;
  }
</script>

<svelte:head>
  <title>{selected ? `${selected.meta.title} — Hubtask Workbench` : 'Hubtask Workbench'}</title>
</svelte:head>

<div class="frame">
  <AppBar
    label="Workbench"
    toggle={isNarrow
      ? { kind: 'drawer', label: 'Open the index', isExpanded: isIndexOpen, onToggle: () => (isIndexOpen = !isIndexOpen) }
      : undefined}
  >
    {#snippet brand()}
      <span class="brand">
        <span class="wordmark">Hubtask Workbench</span>
        <!-- This page is public (ADR-0038), so it says what it is rather than leaving a reader
             to infer it: a development tool showing parts that mostly do not exist yet. The
             stage word is ADR-0035's vocabulary, and the obligation is the one ADR-0035 put on
             the application's maturity banner, applied to the surface that shows the parts. -->
        <span class="stage-word">experimental</span>
      </span>
    {/snippet}
    {#snippet end()}
      <a class="repo" href="https://github.com/Jersyfi/hubtask" rel="noreferrer">Source</a>
    {/snippet}
  </AppBar>

  <p class="subtitle">
    A development tool for building <strong>Hubtask</strong> — every story through every rule
    (<code>design-system.md</code> §3, §4, §6). Components appear, move and disappear without
    notice; nothing here is a promise about the product.
  </p>

  <div class="body">
    {#if isNarrow}
      <NavDrawer bind:isOpen={isIndexOpen} title="Components" dismissLabel="Close the index">
        <Sidebar {groups} selected={selected?.id ?? null} onselect={choose} />
      </NavDrawer>
    {:else}
      <aside class="side">
        <Sidebar {groups} selected={selected?.id ?? null} onselect={choose} />
      </aside>
    {/if}

    <main>
      {#if selected && component}
        <header class="head">
          <p class="crumb">
            <span>{component.group}</span>
            <span aria-hidden="true">/</span>
            <span class="crumb-component">{component.title}</span>
          </p>
          <h1>{selected.name}</h1>
          {#if tabs.length > 1}
            <Tabs label="Stories" {tabs} selected={selected.id} onselect={(id) => workbench.select(id)} />
          {/if}
          <AxisChips
            axes={workbench.axes}
            declared={selected.meta.axes}
            onchange={(id, value) => workbench.set(id, value)}
          />
        </header>

        <Stage story={selected} axes={workbench.axes} onhosts={(next) => (hosts = next)} />

        {#if selected.about}
          <p class="about">{selected.about}</p>
        {/if}

        <FocusPanel {hosts} isOpenByDefault={!isNarrow} />
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
</div>

<style>
  .frame {
    display: flex;
    flex-direction: column;
    min-height: 100%;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    min-width: 0;
  }

  /* On a phone the wordmark gives way before the stage word does: the word is the obligation. */
  .wordmark {
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-bold);
    letter-spacing: -0.01em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .stage-word { flex: none; }

  /* Rule 3: the stage is a word before it is a tint. */
  .stage-word {
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
    color: var(--text-brand);
    font-size: var(--fs-075);
  }

  .subtitle {
    margin: 0;
    padding: var(--sp-100) var(--sp-300);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-surface);
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .body {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    flex: 1;
  }

  .side {
    display: none;
  }

  main {
    display: flex;
    flex-direction: column;
    gap: var(--sp-300);
    min-width: 0;
    padding: var(--sp-300) var(--sp-300) var(--sp-600);
  }

  .head {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
  }

  .crumb {
    display: flex;
    gap: var(--sp-100);
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    letter-spacing: 0.04em;
    color: var(--text-subtle);
  }

  .crumb-component { color: var(--text-secondary); }

  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    text-wrap: balance;
  }

  .about {
    margin: 0;
    max-width: 90ch;
    color: var(--text-secondary);
  }

  .nothing {
    padding: var(--sp-400) 0;
    color: var(--text-secondary);
    max-width: 70ch;
  }

  code { font-family: var(--font-mono); }

  /* From `expanded` up the index is a column beside the stage: the width the sidenav token
     gives it, and the same width the bar switches its control at. The value is written out
     because a media query cannot read a custom property; it is `primitive.breakpoint.expanded`
     and nothing else, and the token remains the source. */
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (min-width: 905px) {
    .body {
      grid-template-columns: var(--layout-sidenav-width) minmax(0, 1fr);
    }

    .side {
      display: block;
      position: sticky;
      inset-block-start: var(--layout-appbar-height);
      align-self: start;
      max-height: calc(100vh - var(--layout-appbar-height));
      overflow-y: auto;
      border-inline-end: 1px solid var(--border-subtle);
      background: var(--bg-surface);
    }
  }
</style>
