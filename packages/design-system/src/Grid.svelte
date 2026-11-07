<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Columns that change with the width, and nothing else (page-system §6, L1).
  //
  // A width that names no count keeps the count of the width below it, so `{ compact: 1, large: 4 }`
  // is one column up to large. The counts travel as `data-` attributes, never as an inline style
  // (ADR-0028), and the widths are the window's, like the five widths of design-system.md §6:
  // a grid in a narrow pane on a wide screen is still a wide screen's grid.

  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  import type { Space } from './space.ts';

  export type GridCount = 1 | 2 | 3 | 4 | 5 | 6;
  export type GridColumns = Partial<Record<'compact' | 'medium' | 'expanded' | 'large' | 'xlarge', GridCount>>;

  interface Props extends HTMLAttributes<HTMLElement> {
    /** The element to render: `ul` for a list of items. */
    as?: string;
    /** Columns per width. */
    columns?: GridColumns;
    /** The gap between cells, both ways. */
    gap?: Space;
    children?: Snippet;
  }

  const { as = 'div', columns = { compact: 1 }, gap = '200', children, ...rest }: Props = $props();
</script>

<svelte:element
  this={as}
  class="grid"
  data-gap={gap}
  data-compact={columns.compact ?? 1}
  data-medium={columns.medium}
  data-expanded={columns.expanded}
  data-large={columns.large}
  data-xlarge={columns.xlarge}
  {...rest}
>
  {@render children?.()}
</svelte:element>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(var(--grid-columns, 1), minmax(0, 1fr));
    margin: 0;
    padding: 0;
    list-style: none;
    box-sizing: border-box;
  }

  .grid[data-gap='050'] { gap: var(--sp-050); }
  .grid[data-gap='100'] { gap: var(--sp-100); }
  .grid[data-gap='150'] { gap: var(--sp-150); }
  .grid[data-gap='200'] { gap: var(--sp-200); }
  .grid[data-gap='250'] { gap: var(--sp-250); }
  .grid[data-gap='300'] { gap: var(--sp-300); }
  .grid[data-gap='400'] { gap: var(--sp-400); }

  .grid[data-compact='1'] { --grid-columns: 1; }
  .grid[data-compact='2'] { --grid-columns: 2; }
  .grid[data-compact='3'] { --grid-columns: 3; }
  .grid[data-compact='4'] { --grid-columns: 4; }
  .grid[data-compact='5'] { --grid-columns: 5; }
  .grid[data-compact='6'] { --grid-columns: 6; }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width >= 600px) {
    .grid[data-medium='1'] { --grid-columns: 1; }
    .grid[data-medium='2'] { --grid-columns: 2; }
    .grid[data-medium='3'] { --grid-columns: 3; }
    .grid[data-medium='4'] { --grid-columns: 4; }
    .grid[data-medium='5'] { --grid-columns: 5; }
    .grid[data-medium='6'] { --grid-columns: 6; }
  }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .grid[data-expanded='1'] { --grid-columns: 1; }
    .grid[data-expanded='2'] { --grid-columns: 2; }
    .grid[data-expanded='3'] { --grid-columns: 3; }
    .grid[data-expanded='4'] { --grid-columns: 4; }
    .grid[data-expanded='5'] { --grid-columns: 5; }
    .grid[data-expanded='6'] { --grid-columns: 6; }
  }

  /* design-system-lint-ignore: `primitive.breakpoint.large` (1240px); a media query cannot read a custom property. */
  @media (width >= 1240px) {
    .grid[data-large='1'] { --grid-columns: 1; }
    .grid[data-large='2'] { --grid-columns: 2; }
    .grid[data-large='3'] { --grid-columns: 3; }
    .grid[data-large='4'] { --grid-columns: 4; }
    .grid[data-large='5'] { --grid-columns: 5; }
    .grid[data-large='6'] { --grid-columns: 6; }
  }

  /* design-system-lint-ignore: `primitive.breakpoint.xlarge` (1600px); a media query cannot read a custom property. */
  @media (width >= 1600px) {
    .grid[data-xlarge='1'] { --grid-columns: 1; }
    .grid[data-xlarge='2'] { --grid-columns: 2; }
    .grid[data-xlarge='3'] { --grid-columns: 3; }
    .grid[data-xlarge='4'] { --grid-columns: 4; }
    .grid[data-xlarge='5'] { --grid-columns: 5; }
    .grid[data-xlarge='6'] { --grid-columns: 6; }
  }
</style>
