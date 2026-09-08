<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Entries on a time axis: a span where both dates exist, a point where only the due date does.
  //
  // **What it cannot place, it lists.** An entry with no dates is shown beside the axis rather than
  // dropped. A timeline that hid what it could not draw would be a filter nobody chose, and the
  // reader would have no way of knowing that a third of their work is missing from the picture.
  //
  // **No drag, in this milestone.** A bar dragged is a date changed, and F2-12 puts the command
  // before the gesture: the command is `DueDateControl`, and a gesture that writes before the
  // command exists is a write with no undo, no confirmation and no keyboard equivalent.
  //
  // **No inline style, so no arithmetic in the markup.** ADR-0028's policy is `style-src 'self'`,
  // which refuses a `style="grid-column: 4 / 9"` — so the track is one cell per day and the cells
  // a span covers are *marked* rather than positioned. The column width is a step of the space
  // scale (rule 15), which is what makes the scale a token rather than a number somebody picked.
  //
  // **The axis marks are the caller's.** A tick is a date and a phrase, already worded: which
  // granularity to mark, where a week starts, and how a month is spelled are all questions this
  // package answers in no language (ADR-0011).

  import { rovingIndex } from './focus.ts';

  /** One entry on the axis. Both dates absent means it cannot be placed and is listed instead. */
  export interface TimelineRow {
    readonly id: string;
    /** What the entry is called. Content, drawn as it is. */
    readonly title: string;
    /** `YYYY-MM-DD`. With a due date it makes a span; without one the entry is undated. */
    readonly start?: string;
    /** `YYYY-MM-DD`. Alone it makes a point. */
    readonly due?: string;
  }

  /** One mark on the axis: where it sits, and what it says. */
  export interface TimelineTick {
    readonly at: string;
    readonly label: string;
  }

  interface Props {
    /** What the timeline is called, for a reader who arrives by keyboard. */
    label: string;
    /** The window drawn, inclusive at both ends, as `YYYY-MM-DD`. */
    from: string;
    to: string;
    rows: readonly TimelineRow[];
    ticks?: readonly TimelineTick[];
    /** What the list of entries that cannot be placed is called. */
    undatedLabel: string;
    /** What a timeline with nothing on it says. */
    emptyLabel: string;
    onSelect?: (id: string) => void;
  }

  const { label, from, to, rows, ticks = [], undatedLabel, emptyLabel, onSelect }: Props = $props();

  const DAY = 86_400_000;

  let list = $state<HTMLElement | null>(null);
  let active = $state(0);

  /** UTC, deliberately: a day index must not move because the machine is west of Greenwich. */
  const dayOf = (date: string) => Date.parse(`${date}T00:00:00Z`);

  const days = $derived(Math.max(1, Math.round((dayOf(to) - dayOf(from)) / DAY) + 1));
  const columns = $derived([...Array(days).keys()]);

  /** Where a row sits, in day columns, clamped to the window rather than dropped at its edge. */
  function placement(row: TimelineRow): { first: number; last: number } | undefined {
    const end = row.due ?? row.start;
    const begin = row.start ?? row.due;
    if (!end || !begin) return undefined;
    const first = Math.round((dayOf(begin) - dayOf(from)) / DAY);
    const last = Math.round((dayOf(end) - dayOf(from)) / DAY);
    if (Number.isNaN(first) || Number.isNaN(last)) return undefined;
    if (last < 0 || first > days - 1) return undefined;
    return { first: Math.max(0, Math.min(first, last)), last: Math.min(days - 1, Math.max(first, last)) };
  }

  const placed = $derived(
    rows
      .map((row) => ({ row, at: placement(row) }))
      .filter((entry): entry is { row: TimelineRow; at: { first: number; last: number } } => entry.at !== undefined),
  );
  const undated = $derived(rows.filter((row) => placement(row) === undefined));

  const tickColumns = $derived(
    new Map(
      ticks
        .map((tick) => [Math.round((dayOf(tick.at) - dayOf(from)) / DAY), tick.label] as const)
        .filter(([column]) => column >= 0 && column < days),
    ),
  );

  // On the rows themselves rather than on the list: a `<ul>` is not interactive, and a keyboard
  // listener on one is a control a screen reader never announces as such.
  function onKeydown(event: KeyboardEvent) {
    const next = rovingIndex(event.key, active, placed.length, { orientation: 'vertical' });
    if (next === null) return;
    event.preventDefault();
    active = next;
    list?.querySelector<HTMLElement>(`[data-index="${next}"]`)?.focus();
  }
</script>

<section class="timeline" aria-label={label}>
  {#if placed.length === 0 && undated.length === 0}
    <p class="empty">{emptyLabel}</p>
  {:else}
    <!-- The scroller is what a left or right arrow moves through, and it scrolls in the writing
         direction by itself: `overflow-x` follows `direction`, so a right-to-left timeline starts
         at its own beginning without a second rule. -->
    <div class="scroller">
      <div class="axis">
        {#each columns as column (column)}
          <span class="tick" data-marked={tickColumns.has(column) ? '' : undefined}>
            {tickColumns.get(column) ?? ''}
          </span>
        {/each}
      </div>

      <ul class="rows" bind:this={list}>
        {#each placed as entry, index (entry.row.id)}
          <li>
            <button
              type="button"
              class="row"
              data-index={index}
              tabindex={index === active ? 0 : -1}
              onkeydown={onKeydown}
              onclick={() => {
                active = index;
                onSelect?.(entry.row.id);
              }}
            >
              <span class="title">{entry.row.title}</span>
              <span class="track">
                {#each columns as column (column)}
                  <!-- A point rather than a one-day span when only the due date exists: the two
                       are different statements, and drawing a bar for a date with no beginning
                       would invent one. -->
                  <span
                    class="cell"
                    data-filled={column >= entry.at.first && column <= entry.at.last ? '' : undefined}
                    data-point={entry.at.first === entry.at.last && column === entry.at.first && !entry.row.start
                      ? ''
                      : undefined}
                  ></span>
                {/each}
              </span>
            </button>
          </li>
        {/each}
      </ul>
    </div>

    {#if undated.length > 0}
      <div class="undated">
        <p class="name">{undatedLabel}</p>
        <ul class="plain">
          {#each undated as row (row.id)}
            <li>
              <button type="button" class="entry" onclick={() => onSelect?.(row.id)}>{row.title}</button>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  {/if}
</section>

<style>
  .timeline { display: flex; flex-direction: column; gap: var(--sp-150); min-width: 0; }

  .scroller { overflow-x: auto; padding-block-end: var(--sp-050); }

  .axis {
    display: grid;
    grid-auto-flow: column;
    /* The scale, as a step of the space scale rather than a number somebody picked (rule 15). */
    grid-auto-columns: var(--sp-300);
    margin-inline-start: var(--sp-800);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .tick {
    padding-block: var(--sp-025);
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
    white-space: nowrap;
  }

  .tick[data-marked] { border-inline-start: var(--bw-hairline) solid var(--border-subtle); }

  .rows,
  .plain {
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    border: 0;
    border-radius: var(--r-md);
    padding: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .row:hover { background: var(--bg-surface-raised); }

  .row:focus-visible,
  .entry:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .title {
    flex: none;
    inline-size: var(--sp-800);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .track { display: grid; grid-auto-flow: column; grid-auto-columns: var(--sp-300); }

  .cell { block-size: var(--sp-300); }

  /* A span is a bar; a point is a mark. Rule 3 keeps the two distinguishable without colour: the
     point is round and inset, the bar runs the width of every day it covers. */
  .cell[data-filled] {
    align-self: center;
    block-size: var(--sp-150);
    background: var(--accent-primary);
  }

  .cell[data-point] { border-radius: var(--r-full); }

  .undated { display: flex; flex-direction: column; gap: var(--sp-050); }

  .name { margin: 0; color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .entry {
    border: 0;
    border-radius: var(--r-md);
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-100);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .entry:hover { background: var(--bg-surface-raised); }

  .empty { margin: 0; padding: var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }
</style>
