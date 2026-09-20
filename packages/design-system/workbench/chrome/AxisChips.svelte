<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The axes as one row of chips (F9-03). Seven groups of buttons filled a phone's screen before
  // the component came; a chip per axis says what is set, and opening one offers its values in a
  // `Popover`. The set still renders from lib/axes.ts and knows nothing else: adding an axis is
  // one entry there, and it appears here as one more chip.
  import Popover from '../../src/Popover.svelte';
  import { AXES, type AxisId, type AxisState } from '../lib/axes.ts';
  import type { StoryMeta } from '../lib/story.ts';

  interface Props {
    axes: AxisState;
    /** The axes this story declares. Highlighted, never enforced — see the note below. */
    declared: StoryMeta['axes'];
    onchange: (id: AxisId, value: string) => void;
  }

  const { axes, declared, onchange }: Props = $props();

  const labelOf = (id: AxisId) => {
    const axis = AXES.find((candidate) => candidate.id === id);
    return axis?.values.find((value) => value.value === axes[id])?.label ?? axes[id];
  };

  const notes = $derived(
    AXES.flatMap((axis) => {
      const value = axis.values.find((candidate) => candidate.value === axes[axis.id]);
      return value?.note ? [{ axis: axis.label, note: value.note }] : [];
    }),
  );

  // One open flag per axis, every one declared: `bind:` refuses an undefined value.
  let open = $state<Record<AxisId, boolean>>(
    Object.fromEntries(AXES.map((axis) => [axis.id, false])) as Record<AxisId, boolean>,
  );
</script>

<div class="chips" role="group" aria-label="Axes">
  {#each AXES as axis (axis.id)}
    <Popover label={axis.label} bind:isOpen={open[axis.id]}>
      {#snippet trigger(props)}
        <!-- An axis this story declares carries a rule for this component. The marker is a
             border rather than a colour on its own — rule 3 applies to the tool as well. -->
        <button
          type="button"
          class="chip"
          class:declared={declared.includes(axis.id)}
          title={axis.rule}
          {...props}
        >
          <span class="chip-axis">{axis.label}</span>
          <span class="chip-value">{labelOf(axis.id)}</span>
        </button>
      {/snippet}
      <fieldset>
        <legend>{axis.label}</legend>
        <p class="rule">{axis.rule}</p>
        <div class="values">
          {#each axis.values as value (value.value)}
            <button
              type="button"
              class="value"
              aria-pressed={axes[axis.id] === value.value}
              onclick={() => {
                onchange(axis.id, value.value);
                open = { ...open, [axis.id]: false };
              }}
            >
              {value.label}
            </button>
          {/each}
        </div>
      </fieldset>
    </Popover>
  {/each}
</div>

{#if notes.length > 0}
  <ul class="notes">
    {#each notes as note (note.axis)}
      <li><strong>{note.axis}:</strong> {note.note}</li>
    {/each}
  </ul>
{/if}

<style>
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
  }

  .chip {
    display: inline-flex;
    align-items: baseline;
    gap: var(--sp-050);
    padding: var(--sp-025) var(--sp-150);
    border: 1px solid var(--border-default);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .chip:hover { background: var(--bg-surface-hover); }

  .chip[aria-expanded='true'] {
    background: var(--accent-primary-subtle);
    border-color: var(--accent-primary);
  }

  .chip-axis {
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-subtle);
  }

  .declared .chip-axis {
    color: var(--text-brand);
    border-bottom: 2px solid var(--accent-primary);
  }

  .chip-value { font-weight: var(--fw-medium); }

  fieldset {
    margin: 0;
    padding: 0;
    border: 0;
    min-width: 16ch;
  }

  legend {
    padding: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-subtle);
  }

  .rule {
    margin: var(--sp-050) 0 var(--sp-100);
    max-width: 36ch;
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }

  .values {
    display: flex;
    flex-wrap: wrap;
    border: 1px solid var(--border-default);
    border-radius: var(--r-sm);
    overflow: hidden;
  }

  .value {
    padding: var(--sp-050) var(--sp-150);
    border: 0;
    border-inline-start: 1px solid var(--border-subtle);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .value:first-child { border-inline-start: 0; }

  .value:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .value[aria-pressed='true'] {
    background: var(--accent-primary);
    color: var(--text-inverse);
  }

  .notes {
    margin: 0;
    padding: var(--sp-100) var(--sp-150);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    list-style: none;
  }
</style>
