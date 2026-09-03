<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where the story renders, once per pane. A pane is one (theme, direction) combination, so
  // `Both / Both` puts four of them side by side and a difference between two modes is a
  // difference one sees rather than one remembers (ADR-0037).
  import type { AxisState } from '../lib/axes.ts';
  import { panes } from '../lib/axes.ts';
  import { applyPseudo } from '../lib/pseudo.ts';
  import type { LoadedStory } from '../lib/story.ts';

  interface Props {
    story: LoadedStory;
    axes: AxisState;
    /** The stage hands its pane elements up, so the focus walk can step through one of them. */
    onhosts: (hosts: HTMLElement[]) => void;
  }

  const { story, axes, onhosts }: Props = $props();

  const Component = $derived(story.meta.component);
  const list = $derived(panes(axes));

  // What forces a re-render. The pseudo-locale rewrites the rendered tree in place, so the tree
  // has to be new whenever anything that feeds it changes - otherwise a second toggle would
  // pseudo-localise the already pseudo-localised text.
  const renderKey = $derived(`${story.id}|${axes.text}|${axes.motion}`);

  let container = $state<HTMLElement | null>(null);

  // The hosts are read out of the DOM rather than collected through `bind:this`. A bound array is
  // both written by the binding and read by this effect, which makes the effect its own
  // dependency - it then re-runs on every render and cancels anything that was watching the
  // previous one, which is how a focus walk ends a tenth of a second after it starts.
  $effect(() => {
    void renderKey;
    if (!container) return;
    const present = [...container.querySelectorAll<HTMLElement>('.host')];
    if (axes.text === 'long') for (const host of present) applyPseudo(host);
    onhosts(present);
  });

  const widthFor = (width: string) => (width === 'auto' ? '100%' : `var(--bp-${width})`);
</script>

<div class="stage" bind:this={container} style="--pane-count: {list.length}">
  {#each list as pane (pane.key)}
    <section
      class="pane"
      data-theme={pane.theme}
      data-motion={axes.motion === 'reduced' ? 'reduced' : null}
      dir={pane.dir}
    >
      <!-- Chrome, not content: the label reads the same way in every pane, so an RTL pane
           is labelled rather than mirrored. -->
      <header class="pane-label" dir="ltr" data-workbench-verbatim>
        <span>{pane.theme}</span>
        <span aria-hidden="true">·</span>
        <span>{pane.dir}</span>
      </header>
      <div class="pane-scroll">
        <div
          class="pane-body"
          style:zoom={axes.zoom === '100' ? null : Number(axes.zoom) / 100}
          style:width={widthFor(axes.width)}
        >
          {#key renderKey}
            <div class="host">
              <Component {...story.args ?? {}} />
            </div>
          {/key}
        </div>
      </div>
    </section>
  {/each}
</div>

<style>
  .stage {
    display: grid;
    grid-template-columns: repeat(var(--pane-count, 1), minmax(0, 1fr));
    gap: var(--sp-200);
    padding: var(--sp-300);
    align-items: start;
    min-height: 0;
  }

  /* Panes carry data-theme, so their own background has to come from the semantic layer or the
     mode being demonstrated would sit on the chrome's canvas and prove nothing. The colour has to
     come with it: a background without one leaves every bare word in a demo inheriting the
     chrome's light-on-dark text, which is invisible in the light pane and was. */
  .pane {
    background: var(--bg-canvas);
    color: var(--text-primary);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-lg);
    overflow: hidden;
    min-width: 0;
  }

  .pane-label {
    display: flex;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-200);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-surface-sunken);
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  /* The width axis sets a pane body wider than its column on purpose - a breakpoint one cannot
     exceed is a breakpoint one cannot check. */
  .pane-scroll {
    overflow-x: auto;
  }

  .pane-body {
    background: var(--bg-canvas);
    padding: var(--sp-300);
  }

  .host {
    min-width: 0;
  }
</style>
