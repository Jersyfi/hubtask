<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product home's key figures under its band (work-surface concept WB-5, §7): `SummaryCard`s
  // four in one row on a desktop, two rows of two on a tablet, two on a phone - which two, the
  // home says by `phone`.
  import type { Snippet } from 'svelte';

  interface Props {
    /** The region's name, announced: *Your desks in figures*. */
    label: string;
    /** The cards' places (from 1) a phone keeps; the rest stand only from a tablet on. Absent: all. */
    phone?: readonly number[];
    children: Snippet;
  }

  const { label, phone, children }: Props = $props();
</script>

<section class="strip" aria-label={label} data-phone={phone ? phone.join(' ') : undefined}>
  {@render children()}
</section>

<style>
  .strip { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--sp-150); }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) { .strip { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .strip[data-phone] > :global(*) { display: none; }
    .strip[data-phone~='1'] > :global(:nth-child(1)),
    .strip[data-phone~='2'] > :global(:nth-child(2)),
    .strip[data-phone~='3'] > :global(:nth-child(3)),
    .strip[data-phone~='4'] > :global(:nth-child(4)) { display: grid; }
  }
</style>
