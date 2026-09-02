<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A container that has padding and nothing else.
  //
  // design-system.md §4, wave 0: a primitive produces no visual style of its own - no colour, no
  // border, no shadow. A primitive that decorates is a component, and belongs in a wave that plans
  // it. What it does have is the one thing §0's rule makes impossible to write at a call site: a
  // spacing value.

  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  import type { Space } from './space.ts';

  interface Props extends HTMLAttributes<HTMLElement> {
    /**
     * The element to render. A primitive that is always a `div` forces a wrapper around every
     * `section`, `li` and `fieldset` that needs padding, and a wrapper is a layout bug waiting for
     * a flex parent.
     */
    as?: string;
    /** Padding on all four sides. */
    padding?: Space;
    /** Padding along the block axis, overriding `padding`. */
    paddingBlock?: Space;
    /** Padding along the inline axis, overriding `padding`. Inline, never left/right (§3). */
    paddingInline?: Space;
    children?: Snippet;
  }

  const { as = 'div', padding, paddingBlock, paddingInline, children, ...rest }: Props = $props();
</script>

<!-- The steps travel as data attributes rather than as an inline `style`. ADR-0028's content
     security policy has no `'unsafe-inline'` in `style-src`, so a `style="padding: …"` written by
     a component is a rule the browser refuses to apply - silently, in production, and nowhere in
     development if the workbench is served without the header. The attribute is what a stylesheet
     can select on. -->
<svelte:element
  this={as}
  class="box"
  data-p={padding}
  data-pb={paddingBlock}
  data-pi={paddingInline}
  {...rest}
>
  {@render children?.()}
</svelte:element>

<style>
  .box {
    /* Nothing else. A primitive whose reset grows is a primitive on its way to being a component. */
    box-sizing: border-box;
  }

  /* One rule per step, three times over, because the alternative is the inline style the CSP
     forbids. They are written out rather than generated: a stylesheet a person can read is worth
     more here than thirty-six lines saved, and the scale changes about once a year. */
  .box[data-p='025'] { padding: var(--sp-025); }
  .box[data-p='050'] { padding: var(--sp-050); }
  .box[data-p='100'] { padding: var(--sp-100); }
  .box[data-p='150'] { padding: var(--sp-150); }
  .box[data-p='200'] { padding: var(--sp-200); }
  .box[data-p='250'] { padding: var(--sp-250); }
  .box[data-p='300'] { padding: var(--sp-300); }
  .box[data-p='400'] { padding: var(--sp-400); }
  .box[data-p='500'] { padding: var(--sp-500); }
  .box[data-p='600'] { padding: var(--sp-600); }
  .box[data-p='800'] { padding: var(--sp-800); }
  .box[data-p='1000'] { padding: var(--sp-1000); }

  .box[data-pb='025'] { padding-block: var(--sp-025); }
  .box[data-pb='050'] { padding-block: var(--sp-050); }
  .box[data-pb='100'] { padding-block: var(--sp-100); }
  .box[data-pb='150'] { padding-block: var(--sp-150); }
  .box[data-pb='200'] { padding-block: var(--sp-200); }
  .box[data-pb='250'] { padding-block: var(--sp-250); }
  .box[data-pb='300'] { padding-block: var(--sp-300); }
  .box[data-pb='400'] { padding-block: var(--sp-400); }
  .box[data-pb='500'] { padding-block: var(--sp-500); }
  .box[data-pb='600'] { padding-block: var(--sp-600); }
  .box[data-pb='800'] { padding-block: var(--sp-800); }
  .box[data-pb='1000'] { padding-block: var(--sp-1000); }

  .box[data-pi='025'] { padding-inline: var(--sp-025); }
  .box[data-pi='050'] { padding-inline: var(--sp-050); }
  .box[data-pi='100'] { padding-inline: var(--sp-100); }
  .box[data-pi='150'] { padding-inline: var(--sp-150); }
  .box[data-pi='200'] { padding-inline: var(--sp-200); }
  .box[data-pi='250'] { padding-inline: var(--sp-250); }
  .box[data-pi='300'] { padding-inline: var(--sp-300); }
  .box[data-pi='400'] { padding-inline: var(--sp-400); }
  .box[data-pi='500'] { padding-inline: var(--sp-500); }
  .box[data-pi='600'] { padding-inline: var(--sp-600); }
  .box[data-pi='800'] { padding-inline: var(--sp-800); }
  .box[data-pi='1000'] { padding-inline: var(--sp-1000); }
</style>
