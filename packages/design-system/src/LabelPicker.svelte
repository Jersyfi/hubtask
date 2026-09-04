<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Choosing labels for an entry, from the ones its collection has.
  //
  // **A label belongs to a collection** (I-W3), so the list this is handed is that collection's and
  // no other. Carrying an entry into another collection is what resolves or reports its labels
  // (I-W6) — that is the move's business, and a picker that tried to solve it would be guessing at
  // a decision the server makes and reports.
  //
  // The description is in the list, not only in a management screen: it is what makes a colour mean
  // something to somebody who did not choose it, and the moment it matters is the moment somebody
  // is choosing between two labels that look alike.
  //
  // Filterable by typing, because a collection with thirty labels is a collection where arrows
  // alone stop being usable — the same threshold `Menu`'s type-ahead exists for.

  import LabelChip from './LabelChip.svelte';
  import SearchField from './SearchField.svelte';
  import { rovingIndex } from './focus.ts';

  /** One label, as the picker needs it. The shape `Label` has, narrowed to what is drawn. */
  export interface PickerLabel {
    readonly id: string;
    readonly name: string;
    readonly colorToken?: string;
    readonly description?: string | null;
  }

  interface Props {
    /** What the list is called, for a reader who arrives by keyboard. */
    label: string;
    /** The collection's labels. Not the workspace's: a label belongs to a collection (I-W3). */
    labels: readonly PickerLabel[];
    /** Which are on this entry. */
    selected?: readonly string[];
    /** The name of the filter field. */
    filterLabel: string;
    /** What an empty collection's label list says. Resolved text (ADR-0011). */
    emptyLabel: string;
    /** What a filter that matched nothing says — a different sentence (voice-and-tone.md §4.2). */
    noMatchLabel: string;
    onToggle?: (id: string) => void;
  }

  const {
    label,
    labels,
    selected = [],
    filterLabel,
    emptyLabel,
    noMatchLabel,
    onToggle,
  }: Props = $props();

  let term = $state('');
  let list = $state<HTMLElement | null>(null);
  let active = $state(0);

  const shown = $derived(
    term.trim() === ''
      ? labels
      : labels.filter((entry) => entry.name.toLowerCase().includes(term.trim().toLowerCase())),
  );

  function onKeydown(event: KeyboardEvent) {
    const next = rovingIndex(event.key, active, shown.length, { orientation: 'vertical' });
    if (next === null) return;
    event.preventDefault();
    active = next;
    list?.querySelector<HTMLElement>(`[data-index="${next}"]`)?.focus();
  }
</script>

<div class="picker">
  <SearchField
    label={filterLabel}
    isLabelHidden
    clearLabel={filterLabel}
    size="sm"
    bind:value={term}
  />

  {#if labels.length === 0}
    <!-- §4.1 and §4.2 are different sentences, and this component has both because it is the one
         place a reader meets each: a collection with no labels yet, and a filter that excluded
         them all. -->
    <p class="empty">{emptyLabel}</p>
  {:else if shown.length === 0}
    <p class="empty">{noMatchLabel}</p>
  {:else}
    <ul class="list" role="listbox" aria-label={label} aria-multiselectable="true" bind:this={list} onkeydown={onKeydown}>
      {#each shown as entry, index (entry.id)}
        <li>
          <button
            type="button"
            class="option"
            role="option"
            data-index={index}
            aria-selected={selected.includes(entry.id)}
            tabindex={index === active ? 0 : -1}
            onclick={() => {
              active = index;
              onToggle?.(entry.id);
            }}
          >
            <!-- Rule 3: the tick carries the selection, not the colour — every option is coloured,
                 so colour cannot be what says which are on the entry. -->
            <span class="tick" aria-hidden="true">{selected.includes(entry.id) ? '✓' : ''}</span>
            <LabelChip name={entry.name} colorToken={entry.colorToken} />
            {#if entry.description}<span class="description">{entry.description}</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    margin: 0;
    padding: 0;
    list-style: none;
    max-block-size: 40vh;
    overflow-y: auto;
  }

  .option {
    display: flex;
    align-items: center;
    gap: var(--density-row-gap);
    inline-size: 100%;
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-100);
    border: 0;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .option:hover { background: var(--bg-surface-raised); }

  .option:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .tick { display: inline-flex; flex: none; inline-size: var(--sp-200); justify-content: center; }

  .description {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .empty { margin: 0; padding: var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }
</style>
