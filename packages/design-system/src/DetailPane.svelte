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

  import Checkbox from './Checkbox.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import ResizeHandle from './ResizeHandle.svelte';
  import { escapeHandler } from './focus.ts';
  import { layers } from './layers.ts';
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
    /** The name of the control that opens the same thing as a page, and where that is. */
    pageLabel?: string;
    /** The word drawn on that control - "Full page" - where `pageLabel` is its whole name. */
    pageWord?: string;
    pageHref?: string;
    onOpenPage?: () => void;
    /** The panel's edge, where the person may drag its width. The caller keeps the width. */
    resize?: {
      readonly label: string;
      readonly value: number;
      readonly min: number;
      readonly max: number;
      readonly reset: number;
      readonly onresize: (width: number) => void;
    };
    /** "Always open entries whole": the person's choice, made where they notice they want it. */
    whole?: {
      readonly label: string;
      readonly isAlways: boolean;
      readonly ontoggle: () => void;
    };
    children: Snippet;
  }

  const { title, kind, trail, dismissLabel, onClose, pageLabel, pageWord, pageHref, onOpenPage, resize, whole, children }: Props = $props();

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
      {#if pageLabel && (pageHref || onOpenPage)}
        {#if pageHref}
          <!-- A link, because it goes somewhere: a reader may open it in a new tab, and the
               router takes it like any other. Where a word is drawn the word is its name (the
               label is in the name, SC 2.5.3) and the full label its tooltip; else the label. -->
          <a
            class="page"
            href={pageHref}
            aria-label={pageWord ? undefined : pageLabel}
            title={pageLabel}
            onclick={(event) => {
              if (!onOpenPage) return;
              event.preventDefault();
              onOpenPage();
            }}
          >
            <Icon name="maximize-2" size="sm" />{#if pageWord}<span>{pageWord}</span>{/if}
          </a>
        {:else}
          <IconButton icon="maximize-2" label={pageLabel} size="sm" onclick={onOpenPage} />
        {/if}
      {/if}
      <IconButton icon="x" label={dismissLabel} size="sm" onclick={onClose} />
    </div>
  </header>
  <div class="body">{@render children()}</div>
  {#if whole}
    <!-- The person's standing choice, at the foot where the concept puts it: a checkbox, so it
         says its state the way every other setting does. -->
    <div class="foot">
      <Checkbox label={whole.label} checked={whole.isAlways} onchange={whole.ontoggle} />
    </div>
  {/if}
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
  .page {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-inline-size: var(--density-control-sm-min);
    min-block-size: var(--density-control-sm-min);
    gap: var(--sp-050);
    padding-inline: var(--sp-050);
    box-sizing: border-box;
    border-radius: var(--r-sm);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    text-decoration: none;
  }

  .page:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  .page:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .body {
    flex: 1;
    min-width: 0;
    padding: var(--sp-200) var(--sp-300) var(--sp-300);
  }

  .foot {
    padding: var(--sp-150) var(--sp-300) var(--sp-300);
    color: var(--text-secondary);
  }
</style>
