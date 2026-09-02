<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Text a screen reader announces and nobody sees.
  //
  // It is in wave 0 rather than wave 1 because an accessible name that is not on screen is a
  // layout concern, and every icon-only control needs one (design-system.md §4). It is not
  // `display: none` and not `visibility: hidden`: both remove the element from the accessibility
  // tree, which is the one thing this must never do.
  //
  // `forceVisible` is the skip-link case - a control that is hidden until it takes focus, which is
  // the only legitimate reason to unhide one of these.

  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  interface Props extends HTMLAttributes<HTMLElement> {
    /** The element to render. `span` inside a control, `div` where a block is wanted. */
    as?: string;
    /** Become visible when this element, or anything inside it, has focus. */
    isFocusable?: boolean;
    children?: Snippet;
  }

  const { as = 'span', isFocusable = false, children, ...rest }: Props = $props();
</script>

<svelte:element this={as} class="visually-hidden" data-focusable={isFocusable ? '' : undefined} {...rest}>
  {@render children?.()}
</svelte:element>

<style>
  /* The clip pattern, in the form that survives a flex parent - a `height: 0` element in a flex
     row still takes part in `align-items: baseline`, which moves the visible siblings. */
  .visually-hidden {
    position: absolute;
    /* The one pixel of the clip pattern is not a design value: a zero-sized element is dropped
       from the accessibility tree by some screen readers, so the box has to exist and be exactly
       one pixel. There is nothing here for tokens.json to decide. */
    /* design-system-lint-ignore */
    width: 1px;
    /* design-system-lint-ignore */
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    border: 0;
    white-space: nowrap;
  }

  /* `:focus-within` rather than `:focus`, because the focus that reveals a skip link is normally
     on a child - the wrapper itself is rarely the focusable thing. */
  .visually-hidden[data-focusable]:focus-within {
    position: static;
    width: auto;
    height: auto;
    margin: 0;
    overflow: visible;
    clip-path: none;
    white-space: normal;
  }
</style>
