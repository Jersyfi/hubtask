<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A priority as one shape wherever it stands - a queue's cell, a head's facts, the rail (the
  // queue's mockup): a glyph and the word, in the level's tone. Up for the two high levels, a dash
  // for the middle, down for the low one; the glyph says it as well as the colour (UX-7).
  import Icon from './Icon.svelte';

  interface Props {
    /** 1 is the most urgent. */
    level: 1 | 2 | 3 | 4;
    /** The resolved words: *P1 Critical*. */
    label: string;
  }

  const { level, label }: Props = $props();
  const glyph = $derived(level <= 2 ? 'arrow-up' : level === 3 ? 'minus' : 'arrow-down');
</script>

<span class="priority" data-level={level}><Icon name={glyph} size="sm" />{label}</span>

<style>
  .priority {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    white-space: nowrap;
  }

  .priority[data-level='1'] { color: var(--status-danger-text); font-weight: var(--fw-medium); }
  .priority[data-level='2'] { color: var(--status-warning-text); font-weight: var(--fw-medium); }
  .priority[data-level='4'] { color: var(--text-subtle); }
</style>
