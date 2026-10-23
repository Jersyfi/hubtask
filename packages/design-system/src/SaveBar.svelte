<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The changes a form holds that are not saved yet, and the two ways out (design-system.md §4,
  // UC-WRK-32 check 3): "2 changes · Discard · Save".
  //
  // **It replaces "nothing has changed yet".** A form's save is never switched off for want of a
  // change; the bar simply is not there until there is one, and then it says how many.
  //
  // **It moves nothing.** It is placed at the end of the form as an anchor that adds space only there, kept at
  // the bottom of the scroll area by `sticky`, and the bar is drawn over the content from there -
  // arriving in opacity, at once under reduced motion. While it shows, the page, or the caller's
  // scroll area marked `data-scroll-area`, keeps a padding of its height at the end, so a field
  // focused below it scrolls clear of it rather than under it; and the field that has focus when
  // the bar opens is moved clear of it then.
  //
  // **The count is announced, once per change**, through a status region that exists before it is
  // filled (known-traps.md: a live region created already filled is not announced).

  import Button from './Button.svelte';

  interface Props {
    /** What the bar is, for a reader who arrives at it: "Unsaved changes". */
    label: string;
    /**
     * How many changes, resolved with its plural form by the caller ("2 changes"). Present means
     * there are changes and the bar shows; absent means there are none.
     */
    changesLabel?: string;
    discardLabel: string;
    saveLabel: string;
    /** Whether the save is under way: the button keeps its place and its width. */
    isSaving?: boolean;
    /** What the save button says to a reader while it works (voice-and-tone.md §2.4). */
    savingLabel?: string;
    onDiscard?: () => void;
    onSave?: () => void;
  }

  const { label, changesLabel, discardLabel, saveLabel, isSaving = false, savingLabel, onDiscard, onSave }: Props = $props();

  const isOpen = $derived(changesLabel !== undefined);

  // Filled only after the region is in the document, so the first count is announced too.
  let announced = $state('');
  $effect(() => {
    announced = changesLabel ?? '';
  });

  let bar = $state<HTMLElement | null>(null);

  /** The nearest ancestor that scrolls the field, or the page. */
  function scrollerOf(node: HTMLElement): Element {
    for (let at = node.parentElement; at; at = at.parentElement) {
      const overflow = getComputedStyle(at).overflowY;
      if ((overflow === 'auto' || overflow === 'scroll') && at.scrollHeight > at.clientHeight) return at;
    }
    return document.scrollingElement ?? document.documentElement;
  }

  /** Moves a field above the bar by exactly what the bar covers of it, where it covers any. */
  function clear(field: EventTarget | null): void {
    if (!(field instanceof HTMLElement) || !bar || bar.contains(field) || field === document.body) return;
    const covered = field.getBoundingClientRect().bottom - bar.getBoundingClientRect().top;
    if (covered > 0) scrollerOf(field).scrollBy({ top: covered });
  }

  // A field that takes focus under the bar is brought above it. The scroll padding below does
  // this where focus honours it; WebKit scrolls a focused field into view without it, and the
  // bar may be taller than the padding where a font's metrics make it so - so the field is moved
  // by exactly what the bar still covers.
  $effect(() => {
    if (!isOpen) return;
    const onFocus = (event: FocusEvent) => clear(event.target);
    document.addEventListener('focusin', onFocus);
    return () => document.removeEventListener('focusin', onFocus);
  });

  // The field being typed into when the first change brings the bar up already has focus, so no
  // focus scroll happens and the bar would arrive over it - on a phone, over the very value being
  // typed. It is moved clear as the bar opens, once the bar has its height.
  $effect(() => {
    if (!isOpen || !bar) return;
    clear(document.activeElement);
  });
</script>

<div class="anchor" data-save-bar="" data-open={isOpen ? '' : undefined}>
  <span class="status" role="status">{announced}</span>
  {#if isOpen}
    <div class="bar" role="region" aria-label={label} bind:this={bar}>
      <span class="count" aria-hidden="true">{changesLabel}</span>
      <span class="actions">
        <Button tone="subtle" onclick={() => onDiscard?.()}>{discardLabel}</Button>
        <Button tone="primary" isBusy={isSaving} busyLabel={savingLabel} onclick={() => onSave?.()}>{saveLabel}</Button>
      </span>
    </div>
  {/if}
</div>

<style>
  /* Closed, no height of its own: the bar hangs from it, over whatever is above. */
  .anchor {
    position: sticky;
    inset-block-end: 0;
    z-index: var(--z-sticky);
    height: 0;
  }

  /* Open, it is as tall as the bar it holds, after everything else - at the very end, where it moves nothing -
     so the last field can scroll clear of the bar. */
  .anchor[data-open] {
    height: auto;
  }

  /* The announcement, not drawn: the bar already shows the count. */
  .status {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100) var(--sp-200);
    box-sizing: border-box;
    min-block-size: var(--layout-bottombar-height);
    padding: var(--sp-100) var(--sp-200);
    border-block-start: var(--bw-hairline) solid var(--border-default);
    background: var(--bg-surface);
    animation: arrive var(--motion-entrance-duration) var(--motion-entrance-easing) both;
  }

  .count {
    color: var(--text-primary);
    font-size: var(--fs-100);
    font-weight: var(--fw-medium);
  }

  .actions { display: inline-flex; flex-wrap: wrap; gap: var(--sp-100); }

  /* While the bar shows, a field focused at the end scrolls clear of it. */
  :global(html:has([data-save-bar][data-open])),
  :global([data-scroll-area]:has([data-save-bar][data-open])) {
    scroll-padding-block-end: var(--layout-bottombar-height);
  }

  @keyframes arrive {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .bar { animation: none; }
  }

  :global([data-motion='reduced']) .bar { animation: none; }
</style>
