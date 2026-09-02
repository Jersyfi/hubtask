<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Things beside one another, wrapping by default.
  //
  // Wrapping is the default and not an option a caller remembers to switch on, because rule 4 says
  // everything grows by 40 %. A row of three buttons that fits in English and not in German is the
  // most common way that rule is broken, and `flex-wrap: nowrap` is how it gets broken.

  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  import type { Align, Justify, Space } from './space.ts';

  interface Props extends HTMLAttributes<HTMLElement> {
    /** The element to render. */
    as?: string;
    /** The gap between children, and between wrapped lines. */
    gap?: Space;
    /** Across the block axis. `center` is what a row of mixed heights usually wants. */
    align?: Align;
    /** Along the inline axis, so it follows the writing direction rather than the screen. */
    justify?: Justify;
    /**
     * Off only where wrapping is provably wrong - a segmented control, a pair that means nothing
     * apart. Rule 4 is the reason this is a decision and not a default.
     */
    wrap?: boolean;
    children?: Snippet;
  }

  const { as = 'div', gap, align, justify, wrap = true, children, ...rest }: Props = $props();
</script>

<svelte:element
  this={as}
  class="inline"
  data-gap={gap}
  data-align={align}
  data-justify={justify}
  data-wrap={wrap ? 'wrap' : 'nowrap'}
  {...rest}
>
  {@render children?.()}
</svelte:element>

<style>
  .inline {
    display: flex;
    flex-direction: row;
    box-sizing: border-box;
  }

  .inline[data-wrap='wrap'] { flex-wrap: wrap; }
  .inline[data-wrap='nowrap'] { flex-wrap: nowrap; }

  .inline[data-gap='025'] { gap: var(--sp-025); }
  .inline[data-gap='050'] { gap: var(--sp-050); }
  .inline[data-gap='100'] { gap: var(--sp-100); }
  .inline[data-gap='150'] { gap: var(--sp-150); }
  .inline[data-gap='200'] { gap: var(--sp-200); }
  .inline[data-gap='250'] { gap: var(--sp-250); }
  .inline[data-gap='300'] { gap: var(--sp-300); }
  .inline[data-gap='400'] { gap: var(--sp-400); }
  .inline[data-gap='500'] { gap: var(--sp-500); }
  .inline[data-gap='600'] { gap: var(--sp-600); }
  .inline[data-gap='800'] { gap: var(--sp-800); }
  .inline[data-gap='1000'] { gap: var(--sp-1000); }

  .inline[data-align='start'] { align-items: flex-start; }
  .inline[data-align='center'] { align-items: center; }
  .inline[data-align='end'] { align-items: flex-end; }
  .inline[data-align='stretch'] { align-items: stretch; }
  .inline[data-align='baseline'] { align-items: baseline; }

  .inline[data-justify='start'] { justify-content: flex-start; }
  .inline[data-justify='center'] { justify-content: center; }
  .inline[data-justify='end'] { justify-content: flex-end; }
  .inline[data-justify='between'] { justify-content: space-between; }
</style>
