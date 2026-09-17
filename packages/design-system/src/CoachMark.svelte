<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One step of the tour (design-system.md §8, F6-14): a caption and one body line in
  // voice-and-tone.md's register, the step count, and three verbs - back, next (or done on the
  // last step) and skip. Resolved text throughout (ADR-0011).
  //
  // A dialog for focus purposes and not a modal one: the element it points at stays interactive,
  // which a `<dialog>` in `showModal` would forbid. So the mark is `role="dialog"` on a surface,
  // focus moves into it when it opens, `Escape` skips through the layer register, and the tab
  // order is the mark's three controls and the element it points at - handed in by the caller,
  // because the mark is anchored to the cut-out and does not know the element. Focus visible
  // everywhere (rule 5) is the buttons' own.

  import { onMount } from 'svelte';

  import Button from './Button.svelte';
  import Inline from './Inline.svelte';
  import Stack from './Stack.svelte';
  import { focusables } from './focus.ts';

  interface Props {
    caption: string;
    body: string;
    /** "Step 2 of 6", resolved by the caller. */
    countLabel: string;
    nextLabel: string;
    backLabel: string;
    skipLabel: string;
    /** Whether a step comes before this one. Back is offered only where there is one. */
    hasBack?: boolean;
    /** The element the step points at, kept in the tab order between skip and back. */
    target?: HTMLElement | null;
    onNext: () => void;
    onBack?: () => void;
    onSkip: () => void;
    /** The surface, for the caller to anchor. */
    surface?: HTMLElement | null;
  }

  let {
    caption,
    body,
    countLabel,
    nextLabel,
    backLabel,
    skipLabel,
    hasBack = false,
    target = null,
    onNext,
    onBack,
    onSkip,
    surface = $bindable(null),
  }: Props = $props();

  const id = `coach-${Math.random().toString(36).slice(2, 9)}`;

  /**
   * The tab order is a ring of the mark's controls and the element it points at, where that one
   * can take focus. `Tab` past the last goes to the first and `Shift+Tab` the other way, so the
   * page behind the mark is never reached by the keyboard while the mark stands - the same claim
   * a modal makes, kept by hand because the target is outside the mark.
   */
  function onKeydown(event: KeyboardEvent) {
    if (event.key !== 'Tab' || !surface) return;
    const ring = [...focusables(surface), ...(target && isFocusable(target) ? [target] : [])];
    if (ring.length === 0) return;
    const index = ring.indexOf(document.activeElement as HTMLElement);
    let next: number;
    if (index === -1) next = event.shiftKey ? ring.length - 1 : 0;
    else next = (index + (event.shiftKey ? -1 : 1) + ring.length) % ring.length;
    event.preventDefault();
    ring[next]?.focus();
  }

  function isFocusable(element: HTMLElement): boolean {
    if (element.matches('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')) return true;
    return element.tabIndex >= 0;
  }

  onMount(() => {
    // Focus moves to the mark's first control - Next - so a reader is where the step is.
    // Capture rather than bubble: the target's own keydown must not see the Tab first.
    document.addEventListener('keydown', onKeydown, true);
    focusables(surface as HTMLElement)[0]?.focus();
    return () => document.removeEventListener('keydown', onKeydown, true);
  });
</script>

<div class="mark" role="dialog" aria-labelledby={`${id}-caption`} aria-describedby={`${id}-body`} bind:this={surface}>
  <Stack gap="100">
    <span class="count">{countLabel}</span>
    <h2 class="caption" id={`${id}-caption`}>{caption}</h2>
    <p class="body" id={`${id}-body`}>{body}</p>
    <Inline gap="100" align="center">
      <Button size="sm" onclick={onNext}>{nextLabel}</Button>
      {#if hasBack}
        <Button size="sm" tone="secondary" onclick={() => onBack?.()}>{backLabel}</Button>
      {/if}
      <Button size="sm" tone="subtle" onclick={onSkip}>{skipLabel}</Button>
    </Inline>
  </Stack>
</div>

<style>
  .mark {
    position: fixed;
    z-index: var(--z-dialog);
    /* Rule 4: as wide as its words up to a limit, never a fixed width. */
    max-width: 36ch;
    margin: var(--sp-100);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-primary);
    /* Rule 1: raised, a standalone surface over the page. */
    box-shadow: var(--shadow-overlay);
  }

  .count { color: var(--text-subtle); font-size: var(--fs-075); }

  .caption {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .body { margin: 0; color: var(--text-secondary); }
</style>
