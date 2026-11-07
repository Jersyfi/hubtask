<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  /** One step of the stack: the workspace level, a hub, its queues, a page's children. */
  export interface StackLevel {
    readonly id: string;
    /** Resolved text: the step's title, also what *‹* names on the next step. */
    readonly title: string;
  }
</script>

<script lang="ts">
  // The same levels as the sidebar, as a stack on compact (design-system.md §4, wave 5; the
  // navigation concept's §9): one stack per bottom-bar tab, the caller keeping which. Its bar
  // shows the root's avatar and bell, or *‹ the level above*, the title and the step's ☆ and ⋯;
  // an edge swipe, the system's back or *‹* goes one level up, and every change is announced
  // ("IT operations, level 2"). What a step lists is the caller's, as `NavGroup`s of touch rows.
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';

  interface Props {
    /** The stack, root first; the last is shown. */
    levels: readonly StackLevel[];
    /** *‹ Hubs*: the level above's short name is the caller's to choose. */
    backLabel?: string;
    onback?: () => void;
    /** The sentence announced on a change: from the title and the depth (1 at the root). */
    announce: (title: string, depth: number) => string;
    /** On the root: before the title (the avatar). */
    rootStart?: Snippet;
    /** At the bar's end: the bell on a root, ☆ and ⋯ deeper, *Done* in reorder mode. */
    end?: Snippet;
    children: Snippet;
  }

  const { levels, backLabel, onback, announce, rootStart, end, children }: Props = $props();

  const top = $derived(levels[levels.length - 1]);
  const isRoot = $derived(levels.length <= 1);
  let heard = $state('');
  let seen = '';

  $effect(() => {
    const key = levels.map((level) => level.id).join('/');
    if (seen && key !== seen && top) heard = announce(top.title, levels.length);
    seen = key;
  });

  // An edge swipe goes one level up: a press that starts at the start edge and travels inward.
  let swipe: { x: number; y: number; edge: number; rtl: boolean } | null = null;

  function press(event: PointerEvent) {
    if (isRoot || !onback || event.pointerType === 'mouse') return;
    const host = event.currentTarget as HTMLElement;
    const box = host.getBoundingClientRect();
    const rtl = getComputedStyle(host).direction === 'rtl';
    const edge = parseFloat(getComputedStyle(host).getPropertyValue('--sp-200')) || 0;
    const fromEdge = rtl ? box.right - event.clientX : event.clientX - box.left;
    swipe = fromEdge <= edge ? { x: event.clientX, y: event.clientY, edge, rtl } : null;
  }

  function release(event: PointerEvent) {
    if (!swipe) return;
    const travel = (swipe.rtl ? -1 : 1) * (event.clientX - swipe.x);
    const drift = Math.abs(event.clientY - swipe.y);
    const far = (event.currentTarget as HTMLElement).getBoundingClientRect().width / 4;
    if (travel > far && drift < travel) onback?.();
    swipe = null;
  }
</script>

<div class="stack" onpointerdown={press} onpointerup={release} onpointercancel={() => (swipe = null)} role="presentation">
  <header class="bar">
    <div class="start">
      {#if isRoot}
        {#if rootStart}{@render rootStart()}{/if}
      {:else if onback}
        <button type="button" class="back" onclick={onback}>
          <Icon name="chevron-left" size="sm" />
          <span class="back-word">{backLabel}</span>
        </button>
      {/if}
    </div>
    <h1 class="title">{top?.title}</h1>
    <div class="end">{#if end}{@render end()}{/if}</div>
  </header>
  {#key top?.id}
    <div class="body">{@render children()}</div>
  {/key}
  <VisuallyHidden><span aria-live="polite">{heard}</span></VisuallyHidden>
</div>

<style>
  .stack {
    display: grid;
    grid-template-rows: auto 1fr;
    min-block-size: 0;
    block-size: 100%;
    background: var(--bg-canvas);
  }

  .bar {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    gap: calc(var(--sp-050) + var(--sp-025));
    block-size: calc(var(--layout-nav-touchrow) + var(--sp-025));
    padding: 0 calc(var(--sp-100) + var(--sp-025));
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    background: var(--bg-surface);
  }

  .start { display: flex; align-items: center; min-inline-size: 0; }
  .end { display: flex; align-items: center; justify-content: flex-end; gap: var(--sp-025); }

  .title {
    margin: 0;
    overflow: hidden;
    font-family: var(--font-ui);
    font-size: var(--fs-100);
    font-weight: var(--fw-semibold);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .back {
    display: flex;
    align-items: center;
    min-inline-size: 0;
    max-inline-size: 100%;
    min-block-size: var(--layout-nav-touchrow);
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-brand);
    font: inherit;
    font-weight: var(--fw-medium);
    cursor: pointer;
  }

  .back :global(.icon) {
    flex: none;
    inline-size: var(--fs-300);
    block-size: var(--fs-300);
  }

  .back-word { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .back:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .body {
    min-block-size: 0;
    overflow: auto;
    padding: var(--sp-150);
    scrollbar-width: none;
    animation: arrive var(--motion-reveal-duration) var(--motion-reveal-easing);
  }

  @keyframes arrive {
    from { opacity: 0; transform: translateX(var(--sp-200)); }
    to { opacity: 1; transform: none; }
  }


  @media (prefers-reduced-motion: reduce) {
    .body { animation: none; }
  }

  :global([data-motion='reduced']) .body { animation: none; }
</style>
