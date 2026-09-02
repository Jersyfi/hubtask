<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Things one under another, with one gap between them.
  //
  // The gap rather than a margin on each child is the whole point: a margin belongs to the child
  // and therefore travels with it into the next layout, where it is wrong. A gap belongs to the
  // arrangement, which is what a caller is choosing when they reach for this.

  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  import type { Align, Justify, Space } from './space.ts';

  interface Props extends HTMLAttributes<HTMLElement> {
    /** The element to render. `ul`, `ol`, `fieldset` and `section` all stack. */
    as?: string;
    /** The gap between children. */
    gap?: Space;
    /** Across the inline axis. `stretch` is the flex default and the useful one here. */
    align?: Align;
    /** Along the block axis. Only bites when the stack has a height to distribute. */
    justify?: Justify;
    children?: Snippet;
  }

  const { as = 'div', gap, align, justify, children, ...rest }: Props = $props();
</script>

<svelte:element this={as} class="stack" data-gap={gap} data-align={align} data-justify={justify} {...rest}>
  {@render children?.()}
</svelte:element>

<style>
  .stack {
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
  }

  /* Written out for the reason Box.svelte gives: `style="gap: …"` is what ADR-0028's CSP refuses. */
  .stack[data-gap='025'] { gap: var(--sp-025); }
  .stack[data-gap='050'] { gap: var(--sp-050); }
  .stack[data-gap='100'] { gap: var(--sp-100); }
  .stack[data-gap='150'] { gap: var(--sp-150); }
  .stack[data-gap='200'] { gap: var(--sp-200); }
  .stack[data-gap='250'] { gap: var(--sp-250); }
  .stack[data-gap='300'] { gap: var(--sp-300); }
  .stack[data-gap='400'] { gap: var(--sp-400); }
  .stack[data-gap='500'] { gap: var(--sp-500); }
  .stack[data-gap='600'] { gap: var(--sp-600); }
  .stack[data-gap='800'] { gap: var(--sp-800); }
  .stack[data-gap='1000'] { gap: var(--sp-1000); }

  /* `flex-start`/`flex-end` and not `left`/`right`: the values that follow the writing direction
     are the only ones that survive the workbench's RTL axis (§3). */
  .stack[data-align='start'] { align-items: flex-start; }
  .stack[data-align='center'] { align-items: center; }
  .stack[data-align='end'] { align-items: flex-end; }
  .stack[data-align='stretch'] { align-items: stretch; }
  .stack[data-align='baseline'] { align-items: baseline; }

  .stack[data-justify='start'] { justify-content: flex-start; }
  .stack[data-justify='center'] { justify-content: center; }
  .stack[data-justify='end'] { justify-content: flex-end; }
  .stack[data-justify='between'] { justify-content: space-between; }
</style>
