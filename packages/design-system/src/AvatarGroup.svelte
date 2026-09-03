<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Several people in the space of one and a bit.
  //
  // The overlap is the only reason this exists rather than an `Inline` of avatars, and it is the
  // only interesting line in it: the first person has to be on top, and painting order is document
  // order, so the row is built backwards and laid out reversed. `z-index` per avatar would be the
  // obvious alternative and it cannot be written - a value per instance is an inline style, which
  // ADR-0028's `style-src 'self'` refuses.
  //
  // The offset is a step of the space scale (rule 15) and `margin-inline-start` follows the
  // writing direction, so a right-to-left row overlaps the other way round without a second rule.
  //
  // The overflow chip's text is the caller's, resolved (ADR-0011). "+3" is a number in every
  // language; "3 more people" is a sentence, and this component does not write sentences.

  import Avatar from './Avatar.svelte';
  import type { ControlSize } from './control.ts';

  interface Props {
    /** Who is here. The order is the order they are shown in, first on top. */
    people: readonly { name: string; src?: string }[];
    /** How many are drawn before the rest become a count. */
    max?: number;
    size?: ControlSize;
    /**
     * What the chip says to a screen reader: "3 more people". Required once there are more people
     * than `max`, for the reason `disabledReason` is - a count with no words is not a name.
     */
    overflowLabel?: string;
  }

  const { people, max = 4, size = 'md', overflowLabel }: Props = $props();

  const shown = $derived(people.slice(0, Math.max(max, 0)));
  const hidden = $derived(people.length - shown.length);
  /** Backwards, so that the one the reader sees first is the one painted last. */
  const stack = $derived([...shown].reverse());
</script>

<span class="group">
  {#if hidden > 0}
    <span class="slot">
      <span class="more" data-size={size} role="img" aria-label={overflowLabel}>+{hidden}</span>
    </span>
  {/if}
  {#each stack as person, index (person.name + index)}
    <span class="slot">
      <Avatar name={person.name} src={person.src} {size} />
    </span>
  {/each}
</span>

<style>
  .group {
    display: inline-flex;
    align-items: center;
    /* Reversed: the last element in the source is the first one seen and the last one painted. */
    flex-direction: row-reverse;
    /* And hugging its content. A reversed row packs towards its main start, which is the *end* of
       the line - so a group stretched by its parent would sit at the far edge with a gap in front
       of it, in both directions. */
    width: fit-content;
  }

  .slot { display: inline-flex; }

  /* Every slot but the one at the edge pulls the next one over itself. The edge slot is left alone
     so that the group still starts where its container does. */
  .slot:not(:last-child) { margin-inline-start: calc(var(--sp-100) * -1); }

  .more {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-weight: var(--fw-medium);
    font-variant-numeric: tabular-nums;
    line-height: 1;
  }

  .more[data-size='md'] { width: var(--sp-400); height: var(--sp-400); font-size: var(--fs-075); }
  .more[data-size='sm'] { width: var(--sp-300); height: var(--sp-300); font-size: var(--fs-050); }
</style>
