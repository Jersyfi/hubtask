<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A key, drawn as a key: one look wherever a shortcut is shown - *Help and shortcuts*, the bar's
  // field, a list's footer, a button's Enter - so a reader learns it once.
  //
  // Decoration beside words that already say the act, so the caller hides it from the tree where
  // the act is announced otherwise (`aria-hidden` on a hint, `aria-keyshortcuts` on the control).

  import type { Snippet } from 'svelte';

  interface Props {
    /** `sm` inside a field, a button or a list's footer; `md` in a table of shortcuts. */
    size?: 'sm' | 'md';
    /** Drawn on a filled control - the primary button's Enter: in the control's own ink. */
    isOnFill?: boolean;
    children: Snippet;
  }

  const { size = 'md', isOnFill = false, children }: Props = $props();
</script>

<kbd class="kbd" data-size={size} data-on-fill={isOnFill ? '' : undefined}>{@render children()}</kbd>

<style>
  .kbd {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    box-sizing: border-box;
    min-inline-size: var(--sp-300);
    padding: var(--sp-025) var(--sp-050);
    border: var(--bw-hairline) solid var(--border-default);
    border-block-end-width: var(--bw-thick);
    border-radius: var(--r-sm);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    line-height: 1.3;
    white-space: nowrap;
  }

  .kbd[data-size='sm'] {
    min-inline-size: var(--sp-250);
    padding: 0 var(--sp-050);
    font-size: var(--fs-050);
  }

  /* On a filled button: the button's ink, a quiet edge, no surface of its own. */
  .kbd[data-on-fill] {
    border-color: currentColor;
    background: transparent;
    color: inherit;
    opacity: 0.85;
  }
</style>
