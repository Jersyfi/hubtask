<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The bar renders from lib/axes.ts and knows nothing else. Adding an axis is one entry in that
  // file, and it then applies to every component at once - which is the property that makes the
  // matrix worth having over per-component knobs.
  import { AXES, type AxisId, type AxisState } from '../lib/axes.ts';
  import type { StoryMeta } from '../lib/story.ts';

  interface Props {
    state: AxisState;
    /** The axes this story declares. Highlighted, never enforced — see the note below. */
    declared: StoryMeta['axes'];
    onchange: (id: AxisId, value: string) => void;
  }

  const { state, declared, onchange }: Props = $props();

  const notes = $derived(
    AXES.flatMap((axis) => {
      const value = axis.values.find((candidate) => candidate.value === state[axis.id]);
      return value?.note ? [{ axis: axis.label, note: value.note }] : [];
    }),
  );
</script>

<div class="bar">
  {#each AXES as axis (axis.id)}
    <fieldset class:declared={declared.includes(axis.id)}>
      <legend title={axis.rule}>{axis.label}</legend>
      <div class="values">
        {#each axis.values as value (value.value)}
          <button
            type="button"
            aria-pressed={state[axis.id] === value.value}
            onclick={() => onchange(axis.id, value.value)}
          >
            {value.label}
          </button>
        {/each}
      </div>
    </fieldset>
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
  .bar {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-250);
    padding: var(--sp-150) var(--sp-300);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-surface);
  }

  fieldset {
    margin: 0;
    padding: 0;
    border: 0;
  }

  legend {
    padding: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-subtle);
    cursor: help;
  }

  /* An axis this story declares carries a rule for this component. The marker is a border rather
     than a colour on its own — rule 3 applies to the tool as well as to the product. */
  .declared legend {
    color: var(--text-brand);
    border-bottom: 2px solid var(--accent-primary);
  }

  .values {
    display: flex;
    /* An axis with six values must not push the page sideways when the frame is narrow. The
       workbench holds itself to what it checks: nothing here scrolls the body horizontally. */
    flex-wrap: wrap;
    margin-top: var(--sp-050);
    border: 1px solid var(--border-default);
    border-radius: var(--r-sm);
    overflow: hidden;
  }

  button {
    padding: var(--sp-050) var(--sp-150);
    border: 0;
    border-inline-start: 1px solid var(--border-subtle);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  button:first-child {
    border-inline-start: 0;
  }

  button:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  button[aria-pressed='true'] {
    background: var(--accent-primary);
    color: var(--text-inverse);
  }

  .notes {
    margin: 0;
    padding: var(--sp-100) var(--sp-300);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    list-style: none;
  }
</style>
