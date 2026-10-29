<!-- SPDX-License-Identifier: Apache-2.0
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
  // **The pointer rules are design-system.md §11.9's, and they are the drag helper's.** A fine pointer begins
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
  //
  // **A dependency is an arrow from the end of the entry waited for to the start of the waiting
  // one** (UC-PLN-01 check 6), measured off the cells it joins rather than computed, so it lands on
  // the bars at every scale and direction. An arrow is not readable: each is also said in words,
  // and one whose waiting entry starts too early is dashed as well as tinted (rule 3).

  import { tick, type Snippet } from 'svelte';

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
    /**
     * Whether the reader may not move it. A locked row draws no ends and takes no drag, on the axis
     * or in the tray; its title still opens it. The caller decides from the reader's role, because
     * a bar that can be dragged is a promise the server will accept the dates.
     */
    readonly isLocked?: boolean;
  }

  /** One mark on the axis: where it sits, and what it says. */
  export interface TimelineTick {
    readonly at: string;
    readonly label: string;
  }

  /** One entry waiting for another, which the axis draws as an arrow between the two rows. */
  export interface TimelineDependency {
    readonly id: string;
    /** The row waited for: the arrow leaves its end. */
    readonly from: string;
    /** The waiting row: the arrow reaches its start. */
    readonly to: string;
    /** Whether the waiting entry starts before the other is due. */
    readonly isEarly?: boolean;
    /** The dependency in words, already worded by the caller. */
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
    /** The entry open beside the timeline: its row is announced as current, not only tinted. */
    currentId?: string;
    /**
     * What a row carries at the end of its title - the entry's menu. Drawn on the axis and in the
     * tray alike, in a place the title keeps for it, so titles do not shift when it appears.
     */
    rowAction?: Snippet<[string]>;
    /**
     * A coarse pointer held still on a row and lifted without moving (design-system.md §11.9): what
     * a mouse has the right click for. A hold that moves drags instead; the click the lift would
     * otherwise send is not delivered, so the row does not also open.
     */
    onHold?: (id: string) => void;
    /** Which rows wait for which. Drawn where both rows are on the axis. */
    dependencies?: readonly TimelineDependency[];
    /** What the list of dependencies in words is called. */
    dependenciesLabel?: string;
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
    currentId,
    rowAction,
    onHold,
    dependencies = [],
    dependenciesLabel,
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

  /**
   * Whether a bar has two ends worth taking hold of separately.
   *
   * One column wide, it has not: the same cell would be both ends, so the whole bar would answer
   * to whichever handle was drawn on top of it and a drag of the bar could never move both dates.
   * A one-day span is dragged as a bar, which is the only reading it has.
   */
  const hasEnds = (at: { first: number; last: number }) => at.last > at.first;

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

  /** The arrows as drawn, in the scroller's own pixels; re-measured whenever what they join moves. */
  let arrows = $state<{ id: string; d: string; isEarly: boolean }[]>([]);
  let canvas = $state<SVGSVGElement | null>(null);

  const drawn = $derived(
    dependencies.filter(
      (dependency) =>
        placed.some((entry) => entry.row.id === dependency.from) && placed.some((entry) => entry.row.id === dependency.to),
    ),
  );

  function measure() {
    const box = canvas?.getBoundingClientRect();
    if (!box || !list) {
      arrows = [];
      return;
    }
    const isRtl = canvas !== null && getComputedStyle(canvas).direction === 'rtl';
    const bar = (id: string) => {
      const cells = list?.querySelectorAll<HTMLElement>(`[data-row="${CSS.escape(id)}"] .cell[data-filled]`);
      if (!cells || cells.length === 0) return undefined;
      const first = cells[0].getBoundingClientRect();
      const last = cells[cells.length - 1].getBoundingClientRect();
      return { first, last };
    };
    const next: { id: string; d: string; isEarly: boolean }[] = [];
    for (const dependency of drawn) {
      const from = bar(dependency.from);
      const to = bar(dependency.to);
      if (!from || !to) continue;
      const x1 = (isRtl ? from.last.left : from.last.right) - box.left;
      const y1 = from.last.top + from.last.height / 2 - box.top;
      const x2 = (isRtl ? to.first.right : to.first.left) - box.left;
      const y2 = to.first.top + to.first.height / 2 - box.top;
      // The bend is as deep as a bar is tall, so the curve scales with the row and no length is
      // invented here.
      const bend = (isRtl ? -1 : 1) * Math.max(from.last.height, to.first.height) * 2;
      next.push({ id: dependency.id, d: `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`, isEarly: dependency.isEarly === true });
    }
    arrows = next;
  }

  $effect(() => {
    // Read so the effect runs again when the bars or the arrows asked for change.
    void placed;
    void drawn;
    void scale;
    let isLive = true;
    void tick().then(() => {
      if (isLive) measure();
    });
    const observed = list;
    const observer = typeof ResizeObserver === 'undefined' || !observed ? undefined : new ResizeObserver(() => measure());
    if (observed) observer?.observe(observed);
    return () => {
      isLive = false;
      observer?.disconnect();
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
      // The row's action is a control of its own, and a press on it is that control's.
      if (target?.closest?.('[data-action]')) return;
      const end = target?.closest?.('[data-end]');
      const surface =
        end ?? target?.closest?.('[data-track]') ?? target?.closest?.('[data-tray]') ?? target?.closest?.('[data-hold]');
      if (!(surface instanceof HTMLElement)) return;
      const id = surface.closest<HTMLElement>('[data-row]')?.dataset.row;
      if (id === undefined) return;
      // A locked row, and a title, take no drag - but a finger held on either still asks for the
      // entry's menu.
      const isHoldOnly = rows.find((row) => row.id === id)?.isLocked === true || surface.hasAttribute('data-hold');
      // `hasAttribute`, not the dataset value: `data-track` carries no value, and `''` is falsy -
      // read as a truth test it made every bar drag look like a tray entry being placed.
      const grab = (
        end instanceof HTMLElement ? end.dataset.end : surface.hasAttribute('data-track') ? 'bar' : 'place'
      ) as TimelineCarry['grab'] | undefined;
      if (grab === undefined) return;
      begin(event, id, grab, isHoldOnly);
    };
    node.addEventListener('pointerdown', onPointerDown);
    return () => node.removeEventListener('pointerdown', onPointerDown);
  }

  /** Swallows the one click a lifted hold sends, so the row it was on does not also open. */
  function swallowNextClick() {
    const swallow = (click: MouseEvent) => {
      click.preventDefault();
      click.stopPropagation();
    };
    window.addEventListener('click', swallow, { capture: true, once: true });
    // A lift that sends no click (the finger left the row) must not eat the next real one.
    setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), HOLD_MS);
  }

  function begin(event: PointerEvent, id: string, grab: TimelineCarry['grab'], isHoldOnly = false) {
    if (event.button !== 0) return;
    const isCoarse = event.pointerType !== 'mouse';
    // A mouse on a row that takes no drag has nothing to begin: its click is the title's.
    if (isHoldOnly && !isCoarse) return;
    const box = axis?.getBoundingClientRect();
    if (!box) return;
    const width = box.width / days;
    const startColumn = columnAt(event.clientX, box.left, width, days);
    const origin = { x: event.clientX, y: event.clientY };

    let hasBegun = false;
    let mayBegin = !isCoarse && !isHoldOnly;
    /** Whether the finger stayed still for the whole hold: lifted now, it asks for the menu. */
    let isHeldStill = false;
    let held: ReturnType<typeof setTimeout> | undefined = isCoarse
      ? setTimeout(() => {
          held = undefined;
          isHeldStill = true;
          mayBegin = !isHoldOnly;
        }, HOLD_MS)
      : undefined;
    let landed = startColumn;

    const onMove = (move: PointerEvent) => {
      if (!hasBegun) {
        if (!hasLeftTheHandle(origin, { x: move.clientX, y: move.clientY })) return;
        // A finger that moved before the hold was up is scrolling the axis, and one on a row that
        // takes no drag is too. Let the gesture go entirely rather than taking it.
        isHeldStill = false;
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
      // Decided on the lift, not when the timer fired, so the same hold could still become a drag.
      if (isHeldStill && !hasBegun && onHold) {
        swallowNextClick();
        onHold(id);
        return;
      }
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
      {#if drawn.length > 0}
        <!-- Unsized and overflowing: it draws in the scroller's coordinates from its own corner. -->
        <svg class="arrows" bind:this={canvas} aria-hidden="true" focusable="false">
          <defs>
            <marker id="timeline-arrow" class="head" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" />
            </marker>
            <marker id="timeline-arrow-early" class="head" data-early viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" />
            </marker>
          </defs>
          {#each arrows as arrow (arrow.id)}
            <path
              class="arrow"
              data-dependency={arrow.id}
              data-early={arrow.isEarly ? '' : undefined}
              d={arrow.d}
              marker-end={arrow.isEarly ? 'url(#timeline-arrow-early)' : 'url(#timeline-arrow)'}
            />
          {/each}
        </svg>
      {/if}
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
          <li
            class="row"
            data-row={entry.row.id}
            data-carrying={carry?.id === entry.row.id ? '' : undefined}
            data-locked={entry.row.isLocked ? '' : undefined}
          >
            <span class="lead">
              <button
                type="button"
                class="title"
                data-index={index}
                data-hold
                data-current={entry.row.id === currentId ? '' : undefined}
                aria-current={entry.row.id === currentId ? 'true' : undefined}
                tabindex={index === active ? 0 : -1}
                onkeydown={onKeydown}
                onclick={() => {
                  active = index;
                  onSelect?.(entry.row.id);
                }}
              >
                {entry.row.title}
              </button>
              {#if rowAction}<span class="action" data-action>{@render rowAction(entry.row.id)}</span>{/if}
            </span>
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
                  {#if !entry.row.isLocked && hasEnds(entry.at) && isFilled && entry.row.start && column === entry.at.first && startHandleLabel}
                    <!-- `tabindex="-1"`: the keyboard path to a date is the entry's own editor, not
                         a handle that would have to invent key bindings for a calendar. -->
                    <button type="button" class="end" data-end="start" aria-label={startHandleLabel} tabindex="-1"
                    ></button>
                  {/if}
                  {#if !entry.row.isLocked && hasEnds(entry.at) && isFilled && entry.row.due && column === entry.at.last && dueHandleLabel}
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
            <span class="lead"><span class="title">{placing.row.title}</span></span>
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
            <li class="trayed" data-row={row.id} data-locked={row.isLocked ? '' : undefined}>
              <button
                type="button"
                class="entry"
                data-tray
                data-current={row.id === currentId ? '' : undefined}
                aria-current={row.id === currentId ? 'true' : undefined}
                onclick={() => onSelect?.(row.id)}
              >
                {row.title}
              </button>
              {#if rowAction}<span class="action" data-action>{@render rowAction(row.id)}</span>{/if}
            </li>
          {/each}
        </ul>
      </details>
    {/if}
  {/if}
  {#if drawn.length > 0}
    <!-- The arrows in words: the picture is for the eye, this is what it says. -->
    <ul class="dependencies" aria-label={dependenciesLabel}>
      {#each drawn as dependency (dependency.id)}
        <li data-dependency-said={dependency.id} data-early={dependency.isEarly ? '' : undefined}>{dependency.label}</li>
      {/each}
    </ul>
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
     descendant of an unpositioned scroller overflows the page instead. */
  .scroller { position: relative; overflow-x: auto; padding-block-end: var(--sp-050); }

  /* `max-content`, so the box is exactly as wide as the columns in it. A grid that filled its
     container would leave the tracks at their own width inside a wider box, and every measurement
     taken of it - which is how a drag finds its column - would be of the wrong thing.

     The margin clears the titles *and* the gap after them, so a column on the axis and the same
     column in a track are the same column. Without the gap in it the whole axis sits one gap to
     the left of the bars it rules, which at the day scale is a third of a day. */
  .axis {
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: var(--column);
    inline-size: max-content;
    margin-inline-start: calc(var(--title) + var(--sp-100));
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
  .tick[data-today] { border-inline-start: var(--bw-ring) solid var(--accent-primary); }

  /* In a track the mark is drawn over the cell rather than as its border: a border is taken out of
     the column, and on today's column - where bars most often start - a bar end would lose the
     ring's width and fall under the target floor. Drawn at the same inline start as the tick's
     border, so the line still runs straight through axis and rows; it takes no pointer. */
  .cell[data-today] { position: relative; }

  .cell[data-today]::after {
    content: '';
    position: absolute;
    inset-block: 0;
    inset-inline-start: 0;
    inline-size: var(--bw-ring);
    background: var(--accent-primary);
    pointer-events: none;
  }

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

  /* The title and the row's action share the title's width, so the axis rules the same columns
     with or without an action: the action's place is taken from the title, never added to the row. */
  .lead {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--sp-050);
    /* The step the scale gained for this: at the widest step it had, every entry truncated to
       three words, which makes the axis readable and the rows useless. A value that does not
       exist goes into tokens.json or is not needed (ADR-0029), so it went in. */
    inline-size: var(--title);
  }

  .action { display: inline-flex; flex: none; }

  .title {
    flex: 1 1 auto;
    min-inline-size: 0;
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

  /* Rule 3, as a list row marks it: the open entry carries the rail as well as the tint. */
  .title[data-current],
  .entry[data-current] {
    background: var(--bg-surface-pressed);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--accent-primary);
  }

  :global([dir='rtl']) .title[data-current],
  :global([dir='rtl']) .entry[data-current] {
    box-shadow: inset calc(var(--bw-thick) * -1) 0 0 0 var(--accent-primary);
  }

  .trayed { display: flex; align-items: center; gap: var(--sp-050); }

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
    inline-size: max-content;
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

  /* The handle is taller than the bar it sits on, so that the target reaches the floor without the
     bar getting thicker. It overflows into the row's own `--sp-300` and no further. */
  .cell[data-filled]:has(.end) { overflow: visible; }

  .cell[data-point] { border-radius: var(--r-full); }

  /* The ends of a bar, which are what a drag of one date takes hold of.
     **The target is the cell; the mark is what is drawn in it.** A cell is `--sp-300` on the block
     axis at every scale and one column on the inline one, so at the day scale the target is
     24 x 24 - exactly where design-system.md §6 rule 1 puts the floor (WCAG 2.2 SC 2.5.8). At the
     week and month scales the column is narrower and cannot be widened: 24 px on a 4 px column
     would cover six days of the picture it exists to edit. That is SC 2.5.8's **Equivalent**
     exception, and the equivalent is real - the row's title opens the entry, where
     `DueDateControl` sets both dates with fields that do meet the floor (§11, 2.5.7 and 2.5.8).

     Taller than the bar it sits on, deliberately: the target reaches the floor without the bar
     getting thicker, and it overflows into the row's own `--sp-300` and no further. */
  .end {
    display: flex;
    align-items: center;
    inline-size: var(--column);
    block-size: var(--sp-300);
    border: 0;
    padding: 0;
    background: transparent;
    cursor: ew-resize;
  }

  /* Each end's mark sits at the bar's outer edge, so it reads as the end rather than as a notch
     somewhere inside the last day. */
  .end[data-end='start'] { justify-content: start; }
  .end[data-end='due'] { justify-content: end; }

  .end::before {
    content: '';
    inline-size: var(--sp-050);
    block-size: var(--sp-150);
    background: var(--accent-primary-pressed);
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

  /* A row the reader may not move offers no grip: the cursor says what a press does, which is
     open the entry. */
  .row[data-locked] .cell[data-filled] { cursor: default; }
  [data-locked] > .entry { cursor: pointer; }

  .said { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  /* Above the rows and taking no pointer, so a bar under an arrow still drags. */
  .arrows {
    position: absolute;
    inset-block-start: 0;
    inset-inline-start: 0;
    inline-size: 100%;
    block-size: 100%;
    overflow: visible;
    pointer-events: none;
  }

  .arrow {
    fill: none;
    stroke: var(--text-secondary);
    stroke-width: var(--bw-thick);
  }

  .arrow[data-early] {
    stroke: var(--status-danger-accent);
    stroke-dasharray: var(--sp-050) var(--sp-025);
  }

  .head path { fill: var(--text-secondary); }
  .head[data-early] path { fill: var(--status-danger-accent); }

  .dependencies {
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    margin: 0;
    padding: 0;
    list-style: none;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .dependencies [data-early] { color: var(--text-danger); }

  .empty { margin: 0; padding: var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }
</style>
