<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entries of a collection against a window of days.
  //
  // **A layout over the same query, not a second kind of read.** The window is a filter and the
  // order is `start_at`; the entries with no dates at all come back from the same request, because
  // the axis cannot place them and the list beside it is where they go.
  //
  // **The window moves by keyboard and by control**, through the same two functions and the same
  // three buttons: a window shifted by its own length is what "earlier" and "later" mean, and a
  // real button is what makes them reachable by Tab rather than by a shortcut nobody is told about.
  //
  // **The week starts on the account's day.** `week_start` has been on the account document since
  // F1 and this is what it has been for; a week that always began on Monday would be a week this
  // client had decided on.
  //
  // Dates are placed in the **reader's** zone. An entry's due date carries its own, and reading a
  // span against a different clock per row would put two entries due at the same moment in two
  // columns.

  import { Button, Select, Skeleton, Timeline, type TimelineRow } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { addDays, monthOf, shift, weekOf, type Window } from '../data/due.ts';
  import { items, type ItemsQuery } from '../data/items.svelte.ts';
  import { bothOf, windowFilter } from '../data/query.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { dateIn, todayIn } from '../i18n/zone.ts';

  interface Props {
    collectionId: string;
    /** What the reader has asked of the entries. The window is asked on top of it, never instead. */
    query?: ItemsQuery;
    onopen: (itemId: string) => void;
  }

  const { collectionId, query, onopen }: Props = $props();

  const zone = $derived(actor.zone);
  const today = $derived(todayIn(zone));

  let span = $state<'week' | 'month'>('month');
  /** The window drawn. Seeded from today, and moved from there. `shown`, because `window` is
   * the global and a component that shadowed it would be one edit away from a puzzle. */
  let shown = $state<Window | undefined>(undefined);

  const current = $derived(
    shown ?? (span === 'week' ? weekOf(today, actor.weekStart) : monthOf(today)),
  );

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
    return items.openTimeline(wanted, asked);
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

  /** One mark on the axis, and only one: today, where it falls inside the window. */
  const ticks = $derived(
    today >= current.from && today <= current.to
      ? [{ at: today, label: t('app.timeline.today') }]
      : [],
  );

  function move(direction: 1 | -1) {
    shown = shift(current, direction);
    announcer.say(t('app.timeline.moved', { from: current.from, to: current.to }));
  }

  function backToToday() {
    shown = span === 'week' ? weekOf(today, actor.weekStart) : monthOf(today);
    announcer.say(t('app.timeline.moved', { from: current.from, to: current.to }));
  }

  function chooseSpan(next: string) {
    span = next === 'week' ? 'week' : 'month';
    // The window is re-seeded around today rather than kept: a month narrowed to a week has to
    // become *some* week, and the one the reader is in is the only one they did not have to pick.
    shown = undefined;
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
      label={t('app.timeline.span')}
      size="sm"
      value={span}
      options={[
        { value: 'week', label: t('app.timeline.span_week') },
        { value: 'month', label: t('app.timeline.span_month') },
      ]}
      onchange={(event: Event) => chooseSpan((event.currentTarget as HTMLSelectElement).value)}
    />
    <span class="range">{t('app.timeline.window', { from: current.from, to: current.to })}</span>
  </div>

  {#if reading === undefined || reading.status === 'loading' || reading.status === 'idle'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else}
    <Timeline
      label={t('app.timeline.label')}
      from={current.from}
      to={current.to}
      {rows}
      {ticks}
      undatedLabel={t('app.timeline.undated')}
      emptyLabel={t('app.timeline.empty')}
      onSelect={onopen}
    />
  {/if}
</div>

<style>
  .timeline { display: flex; flex-direction: column; gap: var(--sp-150); }

  .controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .range { align-self: center; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
