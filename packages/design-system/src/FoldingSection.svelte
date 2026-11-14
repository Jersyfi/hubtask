<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A section of an object's main column that folds itself (ET-3): its heading with a count and
  // its tools, then the content - a text clamped to a number of lines, or a list cut to its first
  // (or newest) rows - and *Show all* under it. A jump mark lands on it, so it takes focus.
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import { foldWindow, type FoldFrom, type FoldLines, type FoldWindow } from './fold.ts';

  type Fold =
    | { readonly kind: 'lines'; readonly lines: FoldLines }
    | { readonly kind: 'rows'; readonly total: number; readonly limit: number; readonly from?: FoldFrom };

  interface Props {
    /** The id a jump mark points at. */
    id: string;
    title: string;
    /** *2 of 8 done*, *9 · 14.1 MB*. */
    count?: string;
    /** The tools and the one act at the end of the heading. */
    actions?: Snippet;
    fold?: Fold;
    /** *Show all 21*, *Show more · Background, Out of scope*. */
    showAllLabel?: string;
    showLessLabel?: string;
    /** The content; for a list, handed the window of rows to draw. */
    children?: Snippet<[FoldWindow]>;
    /**
     * Given, the section is empty and stands as one line (WB-3): its heading, the sentence and its
     * act - no body, no second line.
     */
    emptyLabel?: string;
  }

  const { id, title, count, actions, fold, showAllLabel, showLessLabel, children, emptyLabel }: Props = $props();

  let isOpen = $state(false);
  let body = $state<HTMLElement | null>(null);
  let isOverflowing = $state(false);

  const shown = $derived<FoldWindow>(
    fold?.kind === 'rows' ? foldWindow(fold.total, fold.limit, fold.from, isOpen) : { start: 0, end: Number.MAX_SAFE_INTEGER, hidden: 0 },
  );
  const isClamped = $derived(fold?.kind === 'lines' && !isOpen);
  const canFold = $derived(fold?.kind === 'rows' ? shown.hidden > 0 || isOpen : fold?.kind === 'lines' && (isOverflowing || isOpen));

  // A clamped text measures itself: a short description shows no *Show more*.
  $effect(() => {
    if (!body || fold?.kind !== 'lines' || typeof ResizeObserver === 'undefined') return;
    const node = body;
    const observer = new ResizeObserver(() => (isOverflowing = node.scrollHeight > node.clientHeight + 1));
    observer.observe(node);
    return () => observer.disconnect();
  });

  const bodyId = $derived(`${id}-body`);
</script>

<section class="section" {id} tabindex="-1" aria-labelledby={`${id}-title`} data-empty={emptyLabel !== undefined ? '' : undefined}>
  <header class="head">
    <h2 class="title" id={`${id}-title`}>{title}</h2>
    {#if count}<span class="count">{count}</span>{/if}
    {#if emptyLabel !== undefined}<span class="none">{emptyLabel}</span>{/if}
    {#if actions}<span class="actions">{@render actions()}</span>{/if}
  </header>
  {#if emptyLabel === undefined}
  <!-- A link inside the clamped part is reachable: focus arriving there opens the fold, so it is never hidden (2.4.11). -->
  <div
    class="body"
    id={bodyId}
    bind:this={body}
    data-lines={isClamped && fold?.kind === 'lines' ? fold.lines : undefined}
    onfocusin={() => {
      if (isClamped && isOverflowing) isOpen = true;
    }}
  >
    {@render children?.(shown)}
  </div>
  {/if}
  {#if emptyLabel === undefined && canFold && showAllLabel}
    <button type="button" class="more" aria-expanded={isOpen} aria-controls={bodyId} onclick={() => (isOpen = !isOpen)}>
      <Icon name={isOpen ? 'chevron-up' : 'chevron-down'} size="sm" />{isOpen ? (showLessLabel ?? showAllLabel) : showAllLabel}
    </button>
  {/if}
</section>

<style>
  .section {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    padding-block: var(--sp-300);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    scroll-margin-block-start: calc(var(--page-sticky-top, var(--layout-appbar-height)) + var(--sp-600));
  }

  .section:focus { outline: none; }
  .section:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .head { display: flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }

  .title {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .count { color: var(--text-subtle); font-size: var(--fs-090); }
  .none { min-inline-size: 0; color: var(--text-secondary); font-size: var(--fs-090); }
  .section[data-empty] { padding-block: var(--sp-150); }
  .head { flex-wrap: wrap; }
  .actions { display: inline-flex; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }

  .body { min-inline-size: 0; }

  /* A line count, not a length: the clamp ends after that many whole lines of the text, counted
     across its paragraphs and headings, so the fold never cuts through a line - a height in `lh`
     did, wherever a heading's line was taller than the body's. */
  .body[data-lines] { display: -webkit-box; -webkit-box-orient: vertical; overflow: hidden; }
  .body[data-lines='3'] { -webkit-line-clamp: 3; line-clamp: 3; }
  .body[data-lines='5'] { -webkit-line-clamp: 5; line-clamp: 5; }
  .body[data-lines='8'] { -webkit-line-clamp: 8; line-clamp: 8; }
  .body[data-lines='12'] { -webkit-line-clamp: 12; line-clamp: 12; }

  /* A thumb's target (§7): *Show more* is layout.target tall where the pointer is coarse. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .more { min-block-size: var(--layout-target); }
  }

  .more {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    align-self: flex-start;
    padding: var(--sp-025) 0;
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-brand);
    font: inherit;
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    cursor: pointer;
  }

  .more:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }
</style>
