<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who is on this entry: one person, or a set of them.
  //
  // **One control with two shapes, because the model has two.** An entry carries `assigneeId` or
  // `members[]` depending on what the installation reports, and building two controls for that
  // would be two keyboard implementations, two empty states and two ways of announcing a change —
  // for a difference that is one line: whether choosing replaces the set or adds to it.
  //
  // **It fetches nothing.** The candidates are handed in, and who may be assigned in this
  // container is a question the domain answers (F3-07). A control that went looking would be a
  // control that has to know which container it is in, which server it is talking to, and what to
  // do when the answer is slow — three things a component in this package may not know.
  //
  // **A person is `Avatar` plus their name**, the same way `AvatarGroup` draws one on a card, so
  // that the card and the panel do not disagree about what a member looks like.
  //
  // Filterable by typing, for the reason `LabelPicker` is: a workspace with thirty members is one
  // where arrow keys alone stop being usable.

  import Avatar from './Avatar.svelte';
  import SearchField from './SearchField.svelte';
  import type { Disableable } from './control.ts';
  import { rovingIndex } from './focus.ts';

  /** One candidate, as the control needs it: who they are, and their picture if they have one. */
  export interface Candidate {
    readonly id: string;
    readonly name: string;
    readonly src?: string;
  }

  interface Props extends Disableable {
    /** What the list of people is called, for a reader who arrives by keyboard. */
    label: string;
    /** Who may be chosen. Handed in: the domain answers who, and this draws them. */
    candidates: readonly Candidate[];
    /**
     * Who is chosen. A list in both shapes — `single` simply never holds more than one, which
     * keeps the call site from switching on the shape twice.
     */
    selected?: readonly string[];
    /**
     * Whether the entry carries one assignee or a set, as the installation reports it. The only
     * behavioural difference: choosing replaces, or choosing toggles.
     */
    selection?: 'single' | 'multiple';
    /** The name of the filter field. */
    filterLabel: string;
    /** What nobody-to-choose-from says. Resolved text (ADR-0011). */
    emptyLabel: string;
    /** What a filter that matched nothing says — a different sentence (voice-and-tone.md §4.2). */
    noMatchLabel: string;
    /** What the summary of who is chosen is called, so a screen reader announces a change with a name. */
    chosenLabel: string;
    /** What nobody-chosen-yet says, in the summary. */
    unassignedLabel: string;
    /** The next selection, already computed for the shape this control is in. */
    onSelect?: (ids: readonly string[]) => void;
  }

  const {
    label,
    candidates,
    selected = [],
    selection = 'single',
    filterLabel,
    emptyLabel,
    noMatchLabel,
    chosenLabel,
    unassignedLabel,
    disabledReason,
    onSelect,
  }: Props = $props();

  let term = $state('');
  let list = $state<HTMLElement | null>(null);
  let active = $state(0);

  const unavailable = $derived(disabledReason !== undefined);
  const reasonId = $derived(unavailable ? `reason-${Math.random().toString(36).slice(2, 9)}` : undefined);

  const shown = $derived(
    term.trim() === ''
      ? candidates
      : candidates.filter((person) => person.name.toLowerCase().includes(term.trim().toLowerCase())),
  );

  /** The chosen people in the order the caller holds them, so the summary does not reorder itself. */
  const chosen = $derived(selected.map((id) => candidates.find((person) => person.id === id)).filter((person) => person !== undefined));

  function choose(id: string) {
    if (unavailable) return;
    if (selection === 'single') {
      // Choosing the one already chosen clears it: an entry with no assignee is a state the model
      // has, and a control that could only ever set one would make it unreachable.
      onSelect?.(selected.includes(id) ? [] : [id]);
      return;
    }
    onSelect?.(selected.includes(id) ? selected.filter((held) => held !== id) : [...selected, id]);
  }

  function onKeydown(event: KeyboardEvent) {
    const next = rovingIndex(event.key, active, shown.length, { orientation: 'vertical' });
    if (next === null) return;
    event.preventDefault();
    active = next;
    list?.querySelector<HTMLElement>(`[data-index="${next}"]`)?.focus();
  }
</script>

<div class="control">
  <!-- The summary is a live region, so choosing announces a name rather than leaving a reader to
       go looking for what changed. It is polite: a person choosing is not interrupted by the
       result of their own click. -->
  <p class="chosen" aria-live="polite" aria-label={chosenLabel}>
    {#if chosen.length === 0}
      <span class="empty-inline">{unassignedLabel}</span>
    {:else}
      {#each chosen as person (person.id)}
        <span class="person">
          <Avatar name={person.name} src={person.src} size="sm" />
          <span class="name">{person.name}</span>
        </span>
      {/each}
    {/if}
  </p>

  <SearchField
    label={filterLabel}
    isLabelHidden
    clearLabel={filterLabel}
    size="sm"
    bind:value={term}
  />

  {#if candidates.length === 0}
    <p class="empty">{emptyLabel}</p>
  {:else if shown.length === 0}
    <p class="empty">{noMatchLabel}</p>
  {:else}
    <ul
      class="list"
      role="listbox"
      aria-label={label}
      aria-multiselectable={selection === 'multiple'}
      bind:this={list}
      onkeydown={onKeydown}
    >
      {#each shown as person, index (person.id)}
        <li>
          <button
            type="button"
            class="option"
            role="option"
            data-index={index}
            aria-selected={selected.includes(person.id)}
            aria-describedby={reasonId}
            disabled={unavailable}
            tabindex={index === active ? 0 : -1}
            onclick={() => {
              active = index;
              choose(person.id);
            }}
          >
            <!-- Rule 3: the tick says who is chosen. An avatar is a picture of a person and says
                 nothing about whether they are on this entry. -->
            <span class="tick" aria-hidden="true">{selected.includes(person.id) ? '✓' : ''}</span>
            <Avatar name={person.name} src={person.src} size="sm" />
            <span class="name">{person.name}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}

  {#if unavailable}
    <p class="reason" id={reasonId}>{disabledReason}</p>
  {/if}
</div>

<style>
  .control { display: flex; flex-direction: column; gap: var(--sp-100); min-width: 0; }

  .chosen {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
    margin: 0;
    min-block-size: var(--sp-400);
  }

  .person { display: inline-flex; align-items: center; gap: var(--sp-050); min-width: 0; }

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

  .option:hover:not(:disabled) { background: var(--bg-surface-raised); }

  .option:disabled { background: var(--bg-surface-sunken); color: var(--text-subtle); cursor: not-allowed; }

  .option:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .tick { display: inline-flex; flex: none; inline-size: var(--sp-200); justify-content: center; }

  .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .empty-inline,
  .empty,
  .reason { color: var(--text-subtle); font-size: var(--fs-075); }

  .empty { margin: 0; padding: var(--sp-150); }

  .reason { margin: 0; }
</style>
