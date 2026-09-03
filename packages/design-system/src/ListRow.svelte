<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One row of a list, and the shape every wave 3 row is built on: `TaskRow` and
  // `JumbleInboxItem` are this with a domain inside them.
  //
  // The part worth getting right is what the row *is* to the keyboard, and there are two answers
  // rather than one. A row that navigates is a **link**: it has a destination, `Enter` follows it,
  // and a person can open it in a new tab, which is a thing readers do constantly with lists and
  // which a `div` with an `onclick` takes away from them. A row that only selects is a **button**.
  // A row that does neither is neither, and stays out of the tab order entirely.
  //
  // What it must never be is a `div` with a click handler and a `tabindex`, which is the shape
  // that looks right and is wrong in every way that matters: no role, no `Enter`, no context menu,
  // and nothing announced.

  import type { Snippet } from 'svelte';

  interface Props {
    /** Where the row goes. Present makes it a link. */
    href?: string;
    /** What pressing the row does when it does not navigate. Present makes it a button. */
    onactivate?: () => void;
    /** Whether this row is the chosen one, where a list has a selection. */
    isSelected?: boolean;
    /** The accessible name, where the content alone does not read as one. */
    label?: string;
    /** A checkbox, a drag handle, an icon - what comes before the content. */
    leading?: Snippet;
    /** What comes after it: a badge, a menu, a date. Not part of the row's own activation. */
    trailing?: Snippet;
    children: Snippet;
  }

  const { href, onactivate, isSelected = false, label, leading, trailing, children }: Props = $props();

  const interactive = $derived(href !== undefined || onactivate !== undefined);
</script>

<div class="row" data-selected={isSelected ? '' : undefined} data-interactive={interactive ? '' : undefined}>
  {#if leading}
    <!-- Outside the activation, so a checkbox in a row is a checkbox rather than a way to open the
         entry. A leading control that activated the row would make selecting the fifth item
         navigate to it. -->
    <div class="leading">{@render leading()}</div>
  {/if}

  {#if href !== undefined}
    <a class="content" {href} aria-label={label} aria-current={isSelected ? 'true' : undefined}>
      {@render children()}
    </a>
  {:else if onactivate !== undefined}
    <button
      type="button"
      class="content"
      aria-label={label}
      aria-pressed={isSelected}
      onclick={() => onactivate()}
    >
      {@render children()}
    </button>
  {:else}
    <div class="content">{@render children()}</div>
  {/if}

  {#if trailing}<div class="trailing">{@render trailing()}</div>{/if}
</div>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: var(--density-row-gap);
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-150);
    border-radius: var(--r-md);
    color: var(--text-primary);
    font-size: var(--fs-100);
  }

  .row[data-interactive]:hover { background: var(--bg-surface-raised); }

  /* Rule 3: a selected row is not only tinted. It carries the rail as well, so the selection reads
     in greyscale and to a reader who does not perceive the accent. */
  .row[data-selected] {
    background: var(--bg-surface-raised);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--accent-primary);
  }

  :global([dir='rtl']) .row[data-selected] {
    box-shadow: inset calc(var(--bw-thick) * -1) 0 0 0 var(--accent-primary);
  }

  .leading,
  .trailing { display: flex; flex: none; align-items: center; gap: var(--sp-100); }

  /* The content is what stretches, and it is the whole hit area of the row rather than the text
     inside it: a target the width of a title is a target that misses. */
  .content {
    flex: 1;
    min-width: 0;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: start;
    text-decoration: none;
    /* Rule 4: a long title wraps rather than pushing the trailing controls off the end. */
    overflow-wrap: anywhere;
  }

  a.content,
  button.content { cursor: pointer; }

  .content:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-sm);
  }
</style>
