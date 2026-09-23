<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entries of a collection against a window of days.
  //
  // **A layout over the same query, not a second kind of read.** The window is a filter and the
  // order is `start_at`; the entries with no dates at all come back from the same request, because
  // the axis cannot place them and the tray beside it is where they go.
  //
  // **The window opens where the work is** (ADR-0063 decision 12). The collection walked on
  // 2026-09-22 had two dated entries, one on 25 September and one on 2 October, and the timeline
  // opened on the current month: the first was drawn with no label and the second was outside the
  // window with nothing saying so. So the anchor is today where today's window holds any of the
  // work, and otherwise the dated day nearest to it — and because the window decides which entries
  // are read, it is seeded from a first read around today and then kept.
  //
  // **The window moves by keyboard and by control**, through the same two functions and the same
  // three buttons: a window shifted by its own length is what "earlier" and "later" mean, and a
  // real button is what makes them reachable by Tab rather than by a shortcut nobody is told about.
  //
  // **A drag writes a date, and every drag has a keyboard path.** The component answers in columns
  // and `schedule.ts` turns them into dates; the write is the same one `DuePanel` makes, so a
  // gesture and the form cannot disagree about what a start or a due date is. The single-pointer
  // and keyboard alternative SC 2.5.7 asks for is that form: every row opens its entry.
  //
  // **The week starts on the account's day.** `week_start` has been on the account document since
  // F1 and this is what it has been for; a week that always began on Monday would be a week this
  // client had decided on.
  //
  // Dates are placed in the **reader's** zone. An entry's due date carries its own, and reading a
  // span against a different clock per row would put two entries due at the same moment in two
  // columns.

  import { untrack } from 'svelte';

  import { Button, Select, Skeleton, Timeline, type TimelineCarry, type TimelineRow } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { addDays, dueInputFor, shift, type Window } from '../data/due.ts';
  import { items, type ItemsQuery } from '../data/items.svelte.ts';
  import { bothOf, windowFilter } from '../data/query.ts';
  import { anchorOf, draggedTo, gridOf, placedBetween, scaleOf, windowOf, type Scale, type Span } from '../data/schedule.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { firstWeekdayOf } from '../i18n/week.ts';
  import { dateIn, todayIn } from '../i18n/zone.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    collectionId: string;
    /** What the reader has asked of the entries. The window is asked on top of it, never instead. */
    query?: ItemsQuery;
    onopen: (itemId: string) => void;
  }

  const { collectionId, query, onopen }: Props = $props();

  const zone = $derived(actor.zone);
  const today = $derived(todayIn(zone));
  // The account's day, else the manifest's for the locale, else the locale's own (F5-09).
  const weekStart = $derived(firstWeekdayOf(actor.weekStart, messages.locale, manifest.supportedLocales));

  let scale = $state<Scale>('week');
  /** The window drawn, once the reader has moved it. `shown`, because `window` is the global and a
   * component that shadowed it would be one edit away from a puzzle. */
  let shown = $state<Window | undefined>(undefined);
  /**
   * The day the window opened on, kept once it has been decided.
   *
   * Decided from what the first read answered and then left alone: the window is what the read
   * asks for, so an anchor that kept re-deciding from the rows it had just fetched would chase its
   * own tail - it would move to the work, read a different set, and move again.
   */
  let anchor = $state<string | undefined>(undefined);

  const current = $derived(shown ?? windowOf(anchor ?? today, scale, weekStart));

  /** The window as instants, which is what a `BETWEEN` on a timestamp field compares against. */
  const asInstants = $derived({
    from: `${current.from}T00:00:00.000Z`,
    // The last day is included, so the window ends at the end of it rather than at its start.
    to: `${addDays(current.to, 1)}T00:00:00.000Z`,
  });

  const windowed = $derived<ItemsQuery>({
    ...query,
    filter: bothOf(query?.filter, windowFilter(manifest.value, asInstants.from, asInstants.to)),
    // The order the layout reads in: what starts first is drawn first, and what starts nowhere is
    // last rather than first.
    sort: [{ field: 'start_at', dir: 'ASC' }],
  });

  $effect(() => {
    const wanted = collectionId;
    const asked = windowed;
    // From `untrack`, like the board's: the subscription writes the store this component reads,
    // and an effect that tracked that write would re-open the subscription it had just opened.
    return untrack(() => items.openTimeline(wanted, asked));
  });

  const entries = $derived(items.onTimeline(collectionId));
  const reading = $derived(items.stateOf(`timeline:${collectionId}`));

  const rows = $derived<TimelineRow[]>(
    entries.map((entry: WorkItem) => ({
      id: entry.id,
      title: entry.title,
      start: entry.start_at ? dateIn(entry.start_at, zone) : undefined,
      due: entry.due_at ? dateIn(entry.due_at, zone) : undefined,
    })),
  );

  // The anchor, once: what the collection's first answer says about where its work is. A window
  // the reader has moved is theirs, and neither this nor a later read takes it back.
  $effect(() => {
    if (anchor !== undefined || reading?.status !== 'ready') return;
    const dated = untrack(() => rows.flatMap((row) => [row.start, row.due].filter((day) => day !== undefined)));
    anchor = anchorOf(dated as readonly string[], today, untrack(() => scale), weekStart);
  });

  /**
   * How a gridline is spelled, by the scale: the day alone where every day is ruled, the day and
   * the month where every week is, the month where every month is.
   *
   * The day is a label rather than a moment, so it is read in UTC - a window that moved because
   * the machine is west of Greenwich would be a window nobody asked for.
   */
  const partsFor = (of: Scale): Intl.DateTimeFormatOptions =>
    of === 'day'
      ? { day: 'numeric', timeZone: 'UTC' }
      : of === 'week'
        ? { day: 'numeric', month: 'short', timeZone: 'UTC' }
        : { month: 'short', timeZone: 'UTC' };

  const gridlines = $derived.by(() => {
    const format = new Intl.DateTimeFormat(messages.locale, partsFor(scale));
    return gridOf(current, scale, weekStart).map((at) => ({
      at,
      label: format.format(Date.parse(`${at}T00:00:00Z`)),
    }));
  });

  const todayLabel = $derived(
    t('app.timeline.today_is', {
      date: new Intl.DateTimeFormat(messages.locale, { dateStyle: 'long', timeZone: 'UTC' }).format(
        Date.parse(`${today}T00:00:00Z`),
      ),
    }),
  );

  let failure = $state<string | undefined>(undefined);

  function move(direction: 1 | -1) {
    shown = shift(current, direction);
    announcer.say(t('app.timeline.moved', { from: current.from, to: current.to }));
  }

  function backToToday() {
    anchor = today;
    shown = undefined;
    announcer.say(t('app.timeline.moved', { from: current.from, to: current.to }));
  }

  function chooseScale(next: string) {
    scale = scaleOf(next);
    // The window is re-seeded around the anchor rather than kept: a half-year narrowed to a
    // fortnight has to become *some* fortnight, and the one the work is in is the only one the
    // reader did not have to pick.
    shown = undefined;
  }

  /**
   * What a drag landed on, written as dates.
   *
   * The two writers are `DuePanel`'s: a due date is three fields written together and a start is a
   * plain scalar on the patch (D-01). A drag that moved both sends both, one after the other,
   * because they are two operations in the contract and a client that batched them would be
   * inventing a third.
   */
  async function carried(carry: TimelineCarry) {
    const entry = entries.find((each: WorkItem) => each.id === carry.id);
    if (!entry) return;
    const was: Span = {
      start: entry.start_at ? dateIn(entry.start_at, zone) : undefined,
      due: entry.due_at ? dateIn(entry.due_at, zone) : undefined,
    };
    const next =
      carry.grab === 'place'
        ? placedBetween(addDays(current.from, carry.from), addDays(current.from, carry.to))
        : draggedTo(was, carry.grab, carry.to - carry.from);

    failure = undefined;
    try {
      let version = entry.version;
      if (next.start !== was.start) {
        // Midnight in the reader's own zone: a start carries none of its own, so the only honest
        // reading of a day somebody dropped a bar on is the clock they dropped it with.
        const at = next.start === undefined ? null : new Date(`${next.start}T00:00`).toISOString();
        version = (await items.update(entry.id, { start_at: at }, version)).version;
      }
      if (next.due !== was.due && next.due !== undefined) {
        // All-day, because a bar is drawn in days: a drag says which day, and inventing a time for
        // it would be this client deciding something the reader did not.
        const input = dueInputFor({ date: next.due }, zone);
        if (input) version = (await items.setDue(entry.id, input, version)).version;
      }
      announcer.say(t('app.timeline.placed_announced', { title: entry.title }));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<div class="timeline">
  <!-- The keyboard path is the buttons, and deliberately only the buttons.
       A `keydown` on this wrapper was the first attempt and it was wrong twice over: a `<div>` is
       not in the tab order, so the keys would only ever have fired for somebody who reached it
       some other way — and the a11y lint says so in as many words. Three real controls, each
       reachable by Tab and pressed by Enter or Space, are what "the window moves by keyboard"
       actually asks for. -->
  <div class="controls" role="group" aria-label={t('app.timeline.label')}>
    <Button size="sm" tone="secondary" icon="chevron-left" onclick={() => move(-1)}>
      {t('app.timeline.previous')}
    </Button>
    <Button size="sm" tone="secondary" onclick={backToToday}>{t('app.timeline.today')}</Button>
    <Button size="sm" tone="secondary" icon="chevron-right" onclick={() => move(1)}>
      {t('app.timeline.next')}
    </Button>
    <Select
      label={t('app.timeline.scale')}
      size="sm"
      value={scale}
      options={[
        { value: 'day', label: t('app.timeline.scale_day') },
        { value: 'week', label: t('app.timeline.scale_week') },
        { value: 'month', label: t('app.timeline.scale_month') },
      ]}
      onchange={(event: Event) => chooseScale((event.currentTarget as HTMLSelectElement).value)}
    />
    <span class="range">{t('app.timeline.window', { from: current.from, to: current.to })}</span>
  </div>

  {#if reading === undefined || reading.status === 'loading' || reading.status === 'idle'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else}
    <!-- The window the timeline scrolls in. On a phone it has an edge of its own, so a bar that
         runs past the screen visibly ends at a frame rather than at the glass (ADR-0061). -->
    <div class="window">
      <Timeline
        label={t('app.timeline.label')}
        from={current.from}
        to={current.to}
        {scale}
        {rows}
        {gridlines}
        {today}
        {todayLabel}
        undatedLabel={t('app.timeline.undated')}
        emptyLabel={t('app.timeline.empty')}
        startHandleLabel={t('app.timeline.move_start')}
        dueHandleLabel={t('app.timeline.move_due')}
        onSelect={onopen}
        onCarry={(carry) => void carried(carry)}
      />
    </div>
  {/if}

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
</div>

<style>
  .timeline { display: flex; flex-direction: column; gap: var(--sp-150); }

  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .range { align-self: center; color: var(--text-secondary); font-size: var(--fs-075); }

  .window { min-width: 0; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .window {
      padding: var(--sp-100);
      border: var(--bw-hairline) solid var(--border-strong);
      border-radius: var(--r-md);
      background: var(--bg-surface);
    }
  }
</style>
