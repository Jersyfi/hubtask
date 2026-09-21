<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The column beside a list, from `large` up (ADR-0061 decision 4): an entry opened next to
  // where it was chosen, so the list stays and the choice stays visible.
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
  import { escapeHandler } from './focus.ts';
  import { layers } from './layers.ts';

  interface Props {
    /** What is open. Resolved text (ADR-0011). */
    title: string;
    /** What kind of thing it is, in the manifest's words: "Task", "Work package". */
    kind?: string;
    /** The name of the control that closes it. Required: a pane is always dismissible. */
    dismissLabel: string;
    onClose: () => void;
    /** The name of the control that opens the same thing as a page, and where that is. */
    pageLabel?: string;
    pageHref?: string;
    onOpenPage?: () => void;
    children: Snippet;
  }

  const { title, kind, dismissLabel, onClose, pageLabel, pageHref, onOpenPage, children }: Props = $props();

  const titleId = `pane-${Math.random().toString(36).slice(2, 9)}`;

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
  <header class="head">
    <div class="names">
      {#if kind}<span class="kind">{kind}</span>{/if}
      <h2 id={titleId} class="title">{title}</h2>
    </div>
    <div class="controls">
      {#if pageLabel && (pageHref || onOpenPage)}
        {#if pageHref}
          <!-- A link, because it goes somewhere: a reader may open it in a new tab, and the
               router takes it like any other. The name is the label; the mark is decoration. -->
          <a
            class="page"
            href={pageHref}
            aria-label={pageLabel}
            title={pageLabel}
            onclick={(event) => {
              if (!onOpenPage) return;
              event.preventDefault();
              onOpenPage();
            }}
          >
            <Icon name="external-link" size="sm" />
          </a>
        {:else}
          <IconButton icon="external-link" label={pageLabel} size="sm" onclick={onOpenPage} />
        {/if}
      {/if}
      <IconButton icon="x" label={dismissLabel} size="sm" onclick={onClose} />
    </div>
  </header>
  <div class="body">{@render children()}</div>
</aside>

<style>
  .pane {
    display: flex;
    flex-direction: column;
    inline-size: var(--layout-pane-width);
    max-inline-size: 100%;
    min-width: 0;
    background: var(--bg-surface);
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    /* Rule 1: a pane is a child of the page beside its list, not a standalone element and not an
       overlay, so it takes a hairline and no shadow. */
  }

  .head {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-150);
    padding: var(--sp-200) var(--sp-300);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .names { display: flex; flex-direction: column; gap: var(--sp-025); flex: 1; min-width: 0; }

  .kind {
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-subtle);
  }

  .title {
    margin: 0;
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }

  .controls { display: flex; align-items: center; gap: var(--sp-050); flex: none; }

  /* The same box as the icon button beside it, so the two read as one pair of controls. */
  .page {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-inline-size: var(--density-control-sm-min);
    min-block-size: var(--density-control-sm-min);
    border-radius: var(--r-sm);
    color: var(--text-secondary);
  }

  .page:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  .page:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .body {
    flex: 1;
    min-width: 0;
    padding: var(--sp-300);
  }
</style>
