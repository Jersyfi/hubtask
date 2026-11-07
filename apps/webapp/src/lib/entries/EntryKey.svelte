<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's key as a list row, a board card and the panel draw it (user-experience.md §5.6):
  // one look, one text (`ticketKeyOf`), so "INC-42" is recognised wherever it is read.

  import type { KeyedCollection } from '../data/ticket.ts';
  import { ticketKeyOf } from '../data/ticket.ts';

  interface Props {
    collection: KeyedCollection | null | undefined;
    number: number | null | undefined;
  }

  const { collection, number }: Props = $props();
  const key = $derived(ticketKeyOf(collection, number));
</script>

{#if key}<span class="entry-key" data-entry-key>{key}</span>{/if}

<style>
  .entry-key {
    flex: none;
    white-space: nowrap;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    font-variant-numeric: tabular-nums;
  }
</style>
