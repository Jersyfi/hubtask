<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Entries on a time axis: a span where both dates exist, a point where only one does.
  //
  // **It is a schedule, not a list with a bar in it.** The axis is ruled by dated gridlines at the
  // scale the caller chose and today is marked across every row, so a bar can be read against a
  // date rather than counted along from an edge (ADR-0063 decision 12).
  //
  // **What it cannot place, it trays.** An entry with no dates is shown beside the axis rather than
  // dropped — a timeline that hid the undated would be a filter nobody chose — and the tray folds,
  // because a plan that has not been scheduled yet is mostly tray and the axis is what the reader
  // came for. Carried onto the axis, an undated entry is asking for its first dates.
  //
  // **A drag names columns, never dates.** This component knows the grid and nothing about
  // calendars: it answers which column a gesture began on and which it ended on, and what that
  // means for a start and a due date is the application's (`lib/data/schedule.ts`). The same split
  // `reorder.ts` makes for a rank — the arithmetic out where `node --test` can reach it, and only
  // the events and the measuring left in here.
  //
  // **The bar redraws where it would land rather than being carried.** Nothing translates, so
  // there is no motion to reduce and no transform over a layout: while a drag is live the span is
  // simply computed at the candidate columns, which is also the clearest possible preview.
  //
  // **The pointer rules are decision 13's, and they are the drag helper's.** A fine pointer begins
  // on movement past `hasLeftTheHandle`, a coarse one on `HOLD_MS` of stillness — a finger that
  // moves is scrolling the axis. The single-pointer alternative SC 2.5.7 asks for is the row
  // itself: it opens the entry, where the dates have a form.
  //
  // **No inline style, so no arithmetic in the markup.** ADR-0028's policy is `style-src 'self'`,
  // which refuses a `style="grid-column: 4 / 9"` — so the track is one cell per column and the
  // cells a span covers are *marked* rather than positioned. The column width is a step of the
  // space scale (rule 15), one per scale, which is what makes the scale a token rather than a
  // number somebody picked.
  //
  // **The axis marks are the caller's.** A gridline is a date and a phrase, already worded: which
  // granularity to mark, where a week starts, and how a month is spelled are all questions this
  // package answers in no language (ADR-0011).

  import { HOLD_MS, columnAt, hasLeftTheHandle } from './reorder.ts';
  import { rovingIndex } from './focus.ts';

  /** One entry on the axis. Both dates absent means it cannot be placed and is trayed instead. */
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

  /** What a drag took hold of, and the columns it went from and to. */
  export interface TimelineCarry {
    readonly id: string;
    /** `bar` moves both dates, an end moves one, `place` gives an undated entry its first. */
    readonly grab: 'bar' | 'start' | 'due' | 'place';
    readonly from: number;
    readonly to: number;
  }

  interface Props {
    /** What the timeline is called, for a reader who arrives by keyboard. */
    label: string;
    /** The window drawn, inclusive at both ends, as `YYYY-MM-DD`. */
    from: string;
    to: string;
    /** How finely the axis is ruled. It decides the column width and nothing else. */
    scale?: 'day' | 'week' | 'month';
    rows: readonly TimelineRow[];
    /** The dated gridlines, already worded. */
    gridlines?: readonly TimelineTick[];
    /** The day to mark across every row, where it falls inside the window. */
    today?: string;
    /** What that mark is called, for a reader who cannot see it. */
    todayLabel?: string;
    /** What the tray of entries that cannot be placed is called. */
    undatedLabel: string;
    /** What a timeline with nothing on it says. */
    emptyLabel: string;
    /** What a reader is told a bar's ends are for. */
    startHandleLabel?: string;
    dueHandleLabel?: string;
    onSelect?: (id: string) => void;
    /** Where a drag ended. Called once, and only when it ended somewhere else. */
    onCarry?: (carry: TimelineCarry) => void;
  }

  const {
    label,
    from,
    to,
    scale = 'week',
    rows,
    gridlines = [],
    today,
    todayLabel,
    undatedLabel,
    emptyLabel,
    startHandleLabel,
    dueHandleLabel,
    onSelect,
    onCarry,
  }: Props = $props();

  const DAY = 86_400_000;

  let list = $state<HTMLElement | null>(null);
  let axis = $state<HTMLElement | null>(null);
  let active = $state(0);

  /** The drag in progress, in columns. `null` between drags, which is most of the time. */
  let carry = $state<TimelineCarry | null>(null);

  /** UTC, deliberately: a day index must not move because the machine is west of Greenwich. */
  const dayOf = (date: string) => Date.parse(`${date}T00:00:00Z`);
  const columnOf = (date: string) => Math.round((dayOf(date) - dayOf(from)) / DAY);

  const days = $derived(Math.max(1, Math.round((dayOf(to) - dayOf(from)) / DAY) + 1));
  const columns = $derived([...Array(days).keys()]);

  /** Where a row sits, in day columns, clamped to the window rather than dropped at its edge. */
  function placement(row: TimelineRow): { first: number; last: number } | undefined {
    const end = row.due ?? row.start;
    const begin = row.start ?? row.due;
    if (!end || !begin) return undefined;
    const first = columnOf(begin);
    const last = columnOf(end);
    if (Number.isNaN(first) || Number.isNaN(last)) return undefined;
    if (last < 0 || first > days - 1) return undefined;
    return { first: Math.max(0, Math.min(first, last)), last: Math.min(days - 1, Math.max(first, last)) };
  }

  /**
   * Where a row would be while it is being dragged.
   *
   * The preview, not a transform: what a reader wants to see is the span they would get, and a bar
   * translated by a pointer's travel is the old span drawn somewhere it does not belong.
   */
  function previewed(row: TimelineRow, at: { first: number; last: number }): { first: number; last: number } {
    if (carry?.id !== row.id || carry.grab === 'place') return at;
    const moved = carry.to - carry.from;
    if (carry.grab === 'bar') return { first: at.first + moved, last: at.last + moved };
    if (carry.grab === 'start') return { first: Math.min(at.first + moved, at.last), last: at.last };
    return { first: at.first, last: Math.max(at.last + moved, at.first) };
  }

  const placed = $derived(
    rows
      .map((row) => ({ row, at: placement(row) }))
      .filter((entry): entry is { row: TimelineRow; at: { first: number; last: number } } => entry.at !== undefined)
      .map((entry) => ({ row: entry.row, at: previewed(entry.row, entry.at) })),
  );
  const undated = $derived(rows.filter((row) => placement(row) === undefined));

  /** The undated entry being carried onto the axis, and the span it would be given. */
  const placing = $derived.by(() => {
    const held = carry;
    if (held?.grab !== 'place') return undefined;
    return {
      row: undated.find((row) => row.id === held.id),
      first: Math.min(held.from, held.to),
      last: Math.max(held.from, held.to),
    };
  });

  const lineColumns = $derived(
    new Map(
      gridlines
        .map((tick) => [columnOf(tick.at), tick.label] as const)
        .filter(([column]) => column >= 0 && column < days),
    ),
  );
  const todayColumn = $derived(today === undefined ? -1 : columnOf(today));

  // On the rows themselves rather than on the list: a `<ul>` is not interactive, and a keyboard
  // listener on one is a control a screen reader never announces as such.
  function onKeydown(event: KeyboardEvent) {
    const next = rovingIndex(event.key, active, placed.length, { orientation: 'vertical' });
    if (next === null) return;
    event.preventDefault();
    active = next;
    list?.querySelector<HTMLElement>(`[data-index="${next}"]`)?.focus();
  }

  /**
   * One drag, from `pointerdown` to `pointerup`.
   *
   * The moves are listened for on the window rather than through `setPointerCapture`, for the
   * reason the board's drag gives: a capture taken on `pointerdown` retargets the `click` that
   * follows, and the row's own button would never receive it.
   */
  /**
   * The one listener a drag needs, delegated from the section.
   *
   * Added in an effect rather than written on the track, for the reason the list's own drag gives:
   * a `<div>` with an `onpointerdown` is a static element with an interaction, and a handler there
   * would be a control a screen reader never announces as one. What can be pressed here is the
   * row's title and a bar's ends, and those are real buttons.
   */
  function attach(node: HTMLElement) {
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Element | null;
      const end = target?.closest?.('[data-end]');
      const surface = end ?? target?.closest?.('[data-track]') ?? target?.closest?.('[data-tray]');
      if (!(surface instanceof HTMLElement)) return;
      const id = surface.closest<HTMLElement>('[data-row]')?.dataset.row;
      if (id === undefined) return;
      const grab = (end instanceof HTMLElement ? end.dataset.end : surface.dataset.track ? 'bar' : 'place') as
        | TimelineCarry['grab']
        | undefined;
      if (grab === undefined) return;
      begin(event, id, grab);
    };
    node.addEventListener('pointerdown', onPointerDown);
    return () => node.removeEventListener('pointerdown', onPointerDown);
  }

  function begin(event: PointerEvent, id: string, grab: TimelineCarry['grab']) {
    if (event.button !== 0) return;
    const box = axis?.getBoundingClientRect();
    if (!box) return;
    const width = box.width / days;
    const startColumn = columnAt(event.clientX, box.left, width, days);
    const origin = { x: event.clientX, y: event.clientY };

    const isCoarse = event.pointerType !== 'mouse';
    let hasBegun = false;
    let mayBegin = !isCoarse;
    let held: ReturnType<typeof setTimeout> | undefined = isCoarse
      ? setTimeout(() => {
          mayBegin = true;
        }, HOLD_MS)
      : undefined;
    let landed = startColumn;

    const onMove = (move: PointerEvent) => {
      if (!hasBegun) {
        if (!hasLeftTheHandle(origin, { x: move.clientX, y: move.clientY })) return;
        // A finger that moved before the hold was up is scrolling the axis. Let the gesture go
        // entirely rather than taking it.
        if (!mayBegin) {
          finish();
          return;
        }
        hasBegun = true;
      }
      landed = columnAt(move.clientX, box.left, box.width / days, days);
      carry = { id, grab, from: startColumn, to: landed };
    };

    const finish = () => {
      if (held !== undefined) {
        clearTimeout(held);
        held = undefined;
      }
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', finish);
      window.removeEventListener('pointercancel', finish);
      const moved = hasBegun && (landed !== startColumn || grab === 'place');
      carry = null;
      // A drag that ends where it began writes nothing: it is the ordinary outcome of thinking
      // better of it. An undated entry is the exception - carried anywhere on the axis it is
      // asking for a date, and one column is a day.
      if (moved) onCarry?.({ id, grab, from: startColumn, to: landed });
    };

    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', finish);
    window.addEventListener('pointercancel', finish);
  }
</script>

<section class="timeline" aria-label={label} data-scale={scale} {@attach attach}>
  {#if placed.length === 0 && undated.length === 0}
    <p class="empty">{emptyLabel}</p>
  {:else}
    <!-- The scroller is what a left or right arrow moves through, and it scrolls in the writing
         direction by itself: `overflow-x` follows `direction`, so a right-to-left timeline starts
         at its own beginning without a second rule. -->
    <div class="scroller">
      <div class="axis" bind:this={axis}>
        {#each columns as column (column)}
          <span
            class="tick"
            data-line={lineColumns.has(column) ? '' : undefined}
            data-today={column === todayColumn ? '' : undefined}
          >
            {lineColumns.get(column) ?? ''}
          </span>
        {/each}
      </div>

      <ul class="rows" bind:this={list}>
        {#each placed as entry, index (entry.row.id)}
          <li class="row" data-row={entry.row.id} data-carrying={carry?.id === entry.row.id ? '' : undefined}>
            <button
              type="button"
              class="title"
              data-index={index}
              tabindex={index === active ? 0 : -1}
              onkeydown={onKeydown}
              onclick={() => {
                active = index;
                onSelect?.(entry.row.id);
              }}
            >
              {entry.row.title}
            </button>
            <!-- The track is the drag surface and not a control: the row's control is its title,
                 which opens the entry and is where a keyboard reaches the dates. -->
            <div class="track" data-track>
              {#each columns as column (column)}
                {@const isFilled = column >= entry.at.first && column <= entry.at.last}
                <!-- A point rather than a one-day span when only the due date exists: the two
                     are different statements, and drawing a bar for a date with no beginning
                     would invent one. -->
                <span
                  class="cell"
                  data-today={column === todayColumn ? '' : undefined}
                  data-filled={isFilled ? '' : undefined}
                  data-point={entry.at.first === entry.at.last && column === entry.at.first && !entry.row.start
                    ? ''
                    : undefined}
                >
                  {#if isFilled && entry.row.start && column === entry.at.first && startHandleLabel}
                    <!-- `tabindex="-1"`: the keyboard path to a date is the entry's own editor, not
                         a handle that would have to invent key bindings for a calendar. -->
                    <button type="button" class="end" data-end="start" aria-label={startHandleLabel} tabindex="-1"
                    ></button>
                  {/if}
                  {#if isFilled && entry.row.due && column === entry.at.last && dueHandleLabel}
                    <button type="button" class="end" data-end="due" aria-label={dueHandleLabel} tabindex="-1"
                    ></button>
                  {/if}
                </span>
              {/each}
            </div>
          </li>
        {/each}

        {#if placing?.row}
          <!-- The undated entry while it is being carried: where it would land, drawn on the axis
               it is being given to. -->
          <li class="row" data-carrying>
            <span class="title">{placing.row.title}</span>
            <div class="track">
              {#each columns as column (column)}
                <span
                  class="cell"
                  data-filled={column >= placing.first && column <= placing.last ? '' : undefined}
                ></span>
              {/each}
            </div>
          </li>
        {/if}
      </ul>
    </div>

    {#if undated.length > 0}
      <details class="undated" open>
        <summary class="name">{undatedLabel}</summary>
        <ul class="plain">
          {#each undated as row (row.id)}
            <li data-row={row.id}>
              <button type="button" class="entry" data-tray onclick={() => onSelect?.(row.id)}>
                {row.title}
              </button>
            </li>
          {/each}
        </ul>
      </details>
    {/if}
  {/if}
  {#if today !== undefined && todayLabel !== undefined && todayColumn >= 0 && todayColumn < days}
    <!-- The mark is a line, and a line is not readable. The day it stands on is said once. -->
    <p class="said">{todayLabel}</p>
  {/if}
</section>

<style>
  .timeline {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    min-width: 0;
    /* The scale, as a step of the space scale rather than a number somebody picked (rule 15): a
       fortnight is read day by day, half a year is not. */
    --column: var(--sp-100);
    --title: var(--sp-1600);
  }

  .timeline[data-scale='day'] { --column: var(--sp-300); }
  .timeline[data-scale='week'] { --column: var(--sp-100); }
  .timeline[data-scale='month'] { --column: var(--sp-050); }

  /* Positioned, so it is the containing block of what it scrolls: an absolutely positioned
     descendant of an unpositioned scroller overflows the page instead (issue 874). */
  .scroller { position: relative; overflow-x: auto; padding-block-end: var(--sp-050); }

  .axis {
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: var(--column);
    margin-inline-start: var(--title);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .tick {
    padding-block: var(--sp-025);
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
    white-space: nowrap;
  }

  .tick[data-line] { border-inline-start: var(--bw-hairline) solid var(--border-subtle); }

  /* Today is the one mark that runs through the whole picture, so that a bar is read against it
     without counting columns. Rule 3: it is drawn on the axis and said in words below. */
  .tick[data-today],
  .cell[data-today] { border-inline-start: var(--bw-ring) solid var(--accent-primary); }

  .rows,
  .plain {
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .row { display: flex; align-items: center; gap: var(--sp-100); }

  .row[data-carrying] { opacity: 0.7; }

  .title {
    flex: none;
    /* The step the scale gained for this: at the widest step it had, every entry truncated to
       three words, which makes the axis readable and the rows useless. A value that does not
       exist goes into tokens.json or is not needed (ADR-0029), so it went in. */
    inline-size: var(--title);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    border: 0;
    border-radius: var(--r-md);
    padding: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
  }

  button.title { cursor: pointer; }

  button.title:hover { background: var(--bg-surface-hover); }

  button.title:focus-visible,
  .entry:focus-visible,
  .end:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .track {
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: var(--column);
    touch-action: pan-y;
  }

  .cell { block-size: var(--sp-300); }

  /* A span is a bar; a point is a mark. Rule 3 keeps the two distinguishable without colour: the
     point is round and inset, the bar runs the width of every day it covers. */
  .cell[data-filled] {
    display: flex;
    align-items: center;
    justify-content: space-between;
    align-self: center;
    block-size: var(--sp-150);
    background: var(--accent-primary);
    cursor: grab;
  }

  .cell[data-point] { border-radius: var(--r-full); }

  /* The ends of a bar, which are what a drag of one date takes hold of. They are as wide as the
     column, which at the month scale is narrow - the reader who cannot hit it has the row itself,
     which opens the entry and its date editor (SC 2.5.7). */
  .end {
    inline-size: var(--sp-050);
    block-size: var(--sp-150);
    border: 0;
    padding: 0;
    background: var(--accent-primary-pressed);
    cursor: ew-resize;
  }

  .undated { display: flex; flex-direction: column; gap: var(--sp-050); }

  .name {
    margin: 0;
    color: var(--text-primary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    cursor: pointer;
  }

  .entry {
    border: 0;
    border-radius: var(--r-md);
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-100);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: grab;
    touch-action: pan-y;
  }

  .entry:hover { background: var(--bg-surface-hover); }

  .said { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .empty { margin: 0; padding: var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }
</style>
