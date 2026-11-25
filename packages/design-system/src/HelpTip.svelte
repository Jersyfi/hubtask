<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *More about* a label or a heading, on demand (design-system.md §4, "Help text"): an ⓘ button
  // beside it that opens a `Popover` with up to three sentences - the why, the background, an
  // example. What a person needs to act now stays the visible hint; this is only what explains it.
  //
  // **A click or a tap, never a hover.** A hover has no counterpart on touch, and text that opens
  // under a passing pointer covers what the reader was aiming at. The button says whether it is
  // open (`aria-expanded`); `Escape` or a press elsewhere closes it, and focus goes back to the ⓘ.
  //
  // **A popover on every width**, never the sheet a popover becomes on a phone: three sentences do
  // not need a sheet, and a sheet would hide the field they explain. Its tip points at the ⓘ; it
  // stands below it, above where there is no room (design-system.md §4, "Where a bubble stands"),
  // and closes when the page scrolls the ⓘ out of the window.
  //
  // Focus moves onto the words when it opens, so a screen reader reads them at once; they are not a
  // control and take no place in the tab order.

  import IconButton from './IconButton.svelte';
  import Popover from './Popover.svelte';
  import type { Placement } from './anchor.ts';
  import { helpName } from './media.ts';

  interface Props {
    /** What it explains: the label or the heading it stands beside. Resolved text (ADR-0011). */
    subject: string;
    /** Up to three sentences. Resolved text. */
    text: string;
    /**
     * The button's name, where the application has not given one through `setHelpName`: "More
     * about Expires".
     */
    label?: string;
    placement?: Placement;
  }

  const { subject, text, label, placement = { side: 'block-end', align: 'start' } }: Props = $props();

  let isOpen = $state(false);
  let words = $state<HTMLElement | null>(null);

  const name = $derived(label ?? helpName(subject) ?? subject);

  $effect(() => {
    if (isOpen) words?.focus();
  });

  /**
   * The ⓘ takes focus before it opens: the popover returns focus to what had it, and Safari's press
   * does not focus a button, so `Escape` would leave focus on the page.
   */
  const focusing = (props: Record<string, unknown>) => ({
    ...props,
    onclick: (event: MouseEvent) => {
      (event.currentTarget as HTMLElement).focus();
      (props.onclick as () => void)();
    },
  });
</script>

<span class="help-tip">
  <Popover label={subject} bind:isOpen {placement} isAnchored hasTip>
    {#snippet trigger(props)}
      <IconButton icon="info" label={name} size="sm" tone="subtle" {...focusing(props)} />
    {/snippet}
    <p class="words" tabindex="-1" bind:this={words}>{text}</p>
  </Popover>
</span>

<style>
  .help-tip {
    display: inline-flex;
    vertical-align: middle;
    /* Beside its label's words, not on their baseline: a small square at the line's middle. */
    margin-block: calc(var(--sp-050) * -1);
  }

  .words {
    max-inline-size: 36ch;
    margin: 0;
    color: var(--text-primary);
    font-size: var(--fs-100);
    font-weight: var(--fw-regular);
    line-height: var(--lh-snug);
    text-align: start;
    overflow-wrap: break-word;
  }

  /* Focus is placed on the words for a screen reader; they are not a control, so no ring. */
  .words:focus { outline: none; }
</style>
