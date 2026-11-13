<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One property in the rail (ET-1): its name, its value - a control the caller hands in, or its
  // words - in two columns, the name a step quieter. An empty value is said, never a blank cell.
  import type { Snippet } from 'svelte';

  interface Props {
    label: string;
    /** The value as words, where it is not a control. */
    value?: string;
    /** Said when there is no value: *Nobody - please name someone*, *None*. */
    emptyLabel?: string;
    /** The form marks it required: a star, and the word for a screen reader. */
    requiredLabel?: string;
    /** A line under the value: *from impact × urgency*, *since 21:14 · before 1st level*. */
    note?: string;
    children?: Snippet;
  }

  const { label, value, emptyLabel, requiredLabel, note, children }: Props = $props();
  const isEmpty = $derived(!children && (value === undefined || value === ''));
</script>

<div class="property">
  <span class="name">
    {label}{#if requiredLabel}<span class="required" aria-hidden="true">*</span><span class="hidden">{requiredLabel}</span>{/if}
  </span>
  <span class="value" data-empty={isEmpty ? '' : undefined}>
    {#if children}{@render children()}{:else if isEmpty}{emptyLabel ?? ''}{:else}{value}{/if}
    {#if note}<span class="note">{note}</span>{/if}
  </span>
</div>

<style>
  .property {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
    gap: var(--sp-100);
    align-items: center;
    min-block-size: var(--sp-400);
    font-size: var(--fs-090);
  }

  .name { color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .required { color: var(--text-danger); margin-inline-start: var(--sp-025); }

  .hidden {
    position: absolute;
    inline-size: var(--bw-hairline);
    block-size: var(--bw-hairline);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .value { display: flex; flex-direction: column; gap: var(--bw-hairline); min-inline-size: 0; overflow-wrap: anywhere; }
  .value[data-empty] { color: var(--text-subtle); }
  .note { color: var(--text-subtle); font-size: var(--fs-075); }
</style>
