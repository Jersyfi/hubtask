<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entry panel, from `large` up (ADR-0061 decision 4, design-system.md §11.8): an entry opened
  // next to where it was chosen, so the list stays and the choice stays visible. It lies **over**
  // the list, below the bar and at the end of the line, so opening it never squeezes the list; its
  // width is the person's to drag (`resize`) and the device's to keep, which is the caller's.
  // `whole` is the switch that makes every entry open whole instead, from right here.
  //
  // A place, not a feature. What it holds is the caller's slot - the same one-column form the
  // entry takes on a phone - and everything one can do inside it one can do on the entry's own
  // page, which "open as a page" leads to. It is an `aside` with its own heading, takes no focus
  // of its own and traps none: the reader who opened it from a row is still in the list, which
  // is the point. `Escape` closes it through the register, as the weakest layer, so a menu open
  // inside it goes first.
  //
  // Below `large` it is not drawn at all - the caller does not mount it and navigates instead.
  // This component knows nothing about the viewport.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import ResizeHandle from './ResizeHandle.svelte';
  import { escapeHandler } from './focus.ts';
  import { layers } from './layers.ts';
  import ViewSwitcher from './ViewSwitcher.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';

  interface Props {
    /** What is open. Resolved text (ADR-0011). */
    title: string;
    /** What kind of thing it is, in the manifest's words: "Task", "Work package". */
    kind?: string;
    /**
     * Where it is - the hub and the collection - drawn small at the head, as the concept does: the
     * title is the entry's own field below, so the head says where rather than repeating what.
     */
    trail?: readonly string[];
    /** The name of the control that closes it. Required: a pane is always dismissible. */
    dismissLabel: string;
    onClose: () => void;
    /**
     * How entries open - here beside the list, or as their own page - as one switch of two marks
     * in the head (design-system.md §11.8): choosing *Full page* opens this one so and every one
     * after it; the page carries the same switch back. The caller keeps the choice.
     */
    view?: {
      readonly label: string;
      readonly panelLabel: string;
      readonly pageLabel: string;
      readonly onpage: () => void;
    };
    /** The panel's edge, where the person may drag its width. The caller keeps the width. */
    resize?: {
      readonly label: string;
      readonly value: number;
      readonly min: number;
      readonly max: number;
      readonly reset: number;
      readonly onresize: (width: number) => void;
    };
    children: Snippet;
  }

  const { title, kind, trail, dismissLabel, onClose, view, resize, children }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const titleId = `pane-${instanceId}`;

  // The weakest dismissible layer: `Escape` reaches it last, after any menu or popover opened
  // inside it, and never while a dialog is up.
  $effect(() => {
    const handle = layers.open('overlay', onClose);
    const onKeydown = escapeHandler();
    document.addEventListener('keydown', onKeydown);
    return () => {
      document.removeEventListener('keydown', onKeydown);
      handle.release();
    };
  });
</script>

<aside class="pane" aria-labelledby={titleId}>
  {#if resize}
    <ResizeHandle label={resize.label} value={resize.value} min={resize.min} max={resize.max} reset={resize.reset} edge="inline-start" onresize={resize.onresize} />
  {/if}
  <header class="head">
    <!-- Where the entry is, small; its name is read rather than drawn, because the entry's own
         title field is the first thing below. -->
    <div class="names">
      {#if trail && trail.length > 0}
        <span class="trail">
          {#each trail as step, index (index)}
            {#if index > 0}<span class="step-mark" aria-hidden="true"><Icon name="chevron-right" size="sm" /></span>{/if}
            <span>{step}</span>
          {/each}
        </span>
      {:else if kind}
        <span class="kind">{kind}</span>
      {/if}
      <VisuallyHidden><h2 id={titleId}>{title}</h2></VisuallyHidden>
    </div>
    <div class="controls">
      {#if view}
        <ViewSwitcher
          label={view.label}
          isIconOnly
          views={[
            { id: 'panel', label: view.panelLabel, icon: 'panel-right' },
            { id: 'page', label: view.pageLabel, icon: 'maximize-2' },
          ]}
          selected="panel"
          onselect={(id) => {
            if (id === 'page') view.onpage();
          }}
        />
      {/if}
      <IconButton icon="x" label={dismissLabel} size="sm" onclick={onClose} />
    </div>
  </header>
  <div class="body">{@render children()}</div>
</aside>

<style>
  /* Over the content, below the bar, at the end of the line: the list keeps its width. Raised as an
     overlay is (rule 1) and opaque, never glass, since the list beside it is being read. Its width
     is the caller's `--panel-width` where the person dragged one, else the token's. */
  .pane {
    box-sizing: border-box;
    position: fixed;
    inset-block: var(--layout-appbar-height) 0;
    inset-inline-end: 0;
    z-index: var(--z-overlay);
    display: flex;
    flex-direction: column;
    inline-size: var(--panel-width, var(--layout-panel-width));
    max-inline-size: 100%;
    min-width: 0;
    overflow: auto;
    background: var(--bg-surface);
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    /* The bar's edge, drawn again where the panel lies over it: the line under the bar runs on
       across the panel as it does across the column. */
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    box-shadow: var(--shadow-overlay);
    /* The shadow falls on the list beside it and never up over the bar it sits under. */
    clip-path: inset(0 0 0 calc(-1 * var(--sp-1000)));
    /* Scrolling to its end stays in it: the page behind neither scrolls on nor bounces. */
    overscroll-behavior: contain;
  }

  :global([dir='rtl']) .pane { clip-path: inset(0 calc(-1 * var(--sp-1000)) 0 0); }

  .head {
    display: flex;
    gap: var(--sp-150);
    align-items: center;
    padding: var(--sp-200) var(--sp-300) 0;
  }

  .names { display: flex; flex-direction: column; gap: var(--sp-025); flex: 1; min-width: 0; }

  .kind {
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-subtle);
  }

  .trail {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .step-mark { display: inline-flex; }

  .controls { display: flex; align-items: center; gap: var(--sp-050); flex: none; }

  /* The same box as the icon button beside it, so the two read as one pair of controls. */



  .body {
    flex: 1;
    min-width: 0;
    padding: var(--sp-200) var(--sp-300) var(--sp-300);
  }

</style>
