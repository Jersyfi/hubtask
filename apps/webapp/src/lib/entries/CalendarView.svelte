<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The calendar layout (work-surface concept §6.2, E-21): a month of the collection's entries on
  // the days of the date the view names - due by default, start, created or done. What the query
  // keeps is what stands; the month is one more condition of the same read, so nothing beyond the
  // reader's own view is asked (Q-5). Undated entries are counted under the month, never hidden.
  import { untrack } from 'svelte';
  import { Button, IconButton } from '@hubtask/design-system/components';
  import type { FilterNode } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { items, type ItemsQuery } from '../data/items.svelte.ts';
  import { bothOf } from '../data/query.ts';
  import { monthGrid } from '../data/viewquery.ts';
  import { formatPlainDay } from '../i18n/datetime.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { firstWeekdayOf } from '../i18n/week.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    collectionId: string;
    query: ItemsQuery;
    /** The timestamp the entries are placed on (`SavedView.grouping.date_field`). */
    dateField: string;
    onopen: (itemId: string) => void;
  }

  let { collectionId, query, dateField, onopen }: Props = $props();

  const today = todayIn(actor.zone);
  let year = $state(Number(today.slice(0, 4)));
  let month = $state(Number(today.slice(5, 7)) - 1);
  const weeks = $derived(monthGrid(year, month, firstWeekdayOf(actor.weekStart, messages.locale) === 0));
  const from = $derived(weeks[0]?.[0] ?? today);
  const to = $derived(weeks.at(-1)?.at(-1) ?? today);

  $effect(() => {
    const window: FilterNode = { op: 'BETWEEN', field: dateField, value: [`${from}T00:00:00Z`, `${to}T23:59:59Z`] } as FilterNode;
    const asked: ItemsQuery = { filter: bothOf(query.filter, window), sort: [{ field: dateField, dir: 'ASC' }] };
    const at = collectionId;
    return untrack(() => items.openCollection(at, asked));
  });
  const entries = $derived(items.inCollection(collectionId));
  const dayOf = (entry: object) => {
    const value = (entry as Record<string, unknown>)[dateField];
    return typeof value === 'string' ? value.slice(0, 10) : undefined;
  };
  const byDay = $derived.by(() => {
    const days = new Map<string, { id: string; title: string }[]>();
    for (const entry of entries) {
      const day = dayOf(entry);
      if (!day) continue;
      days.set(day, [...(days.get(day) ?? []), { id: entry.id, title: entry.title }]);
    }
    return days;
  });
  const monthName = $derived(new Intl.DateTimeFormat(messages.locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(year, month, 1))));
  const weekdays = $derived((weeks[0] ?? []).map((day) => new Intl.DateTimeFormat(messages.locale, { weekday: 'short', timeZone: 'UTC' }).format(new Date(`${day}T00:00:00Z`))));

  function step(by: number) {
    const next = new Date(Date.UTC(year, month + by, 1));
    year = next.getUTCFullYear();
    month = next.getUTCMonth();
  }
</script>

<section class="calendar" aria-label={t('app.calendar.label', { month: monthName })} data-calendar={dateField}>
  <header>
    <IconButton icon="chevron-left" size="sm" label={t('app.calendar.previous')} onclick={() => step(-1)} />
    <h2>{monthName}</h2>
    <IconButton icon="chevron-right" size="sm" label={t('app.calendar.next')} onclick={() => step(1)} />
    <Button size="sm" tone="subtle" onclick={() => { year = Number(today.slice(0, 4)); month = Number(today.slice(5, 7)) - 1; }}>{t('app.calendar.today')}</Button>
  </header>
  <div class="grid" role="table" aria-label={monthName}>
    <div class="row head" role="row">{#each weekdays as name, index (index)}<span role="columnheader">{name}</span>{/each}</div>
    {#each weeks as week (week[0])}
      <div class="row" role="row">
        {#each week as day (day)}
          {@const held = byDay.get(day) ?? []}
          <div class="day" role="cell" data-day={day} data-other={Number(day.slice(5, 7)) - 1 !== month ? '' : undefined} data-today={day === today ? '' : undefined}>
            <span class="number" aria-label={formatPlainDay(day, messages.locale)}>{Number(day.slice(8, 10))}</span>
            {#each held.slice(0, 3) as entry (entry.id)}
              <button type="button" class="entry" data-entry={entry.id} onclick={() => onopen(entry.id)}>{entry.title}</button>
            {/each}
            {#if held.length > 3}<span class="more">{t('app.calendar.more', { count: String(held.length - 3) })}</span>{/if}
          </div>
        {/each}
      </div>
    {/each}
  </div>
</section>

<style>
  .calendar { display: flex; flex-direction: column; gap: var(--sp-150); }
  header { display: flex; align-items: center; gap: var(--sp-100); }
  h2 { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); min-inline-size: var(--sp-1000); text-align: center; }
  .grid { display: flex; flex-direction: column; border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); overflow: hidden; background: var(--bg-surface); }
  .row { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); }
  .row + .row { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .head span { padding: var(--sp-100); font-size: var(--fs-065); color: var(--text-secondary); background: var(--bg-surface-sunken); }
  .day { display: flex; flex-direction: column; gap: var(--sp-025); min-block-size: var(--sp-1000); padding: var(--sp-050); min-inline-size: 0; }
  .day + .day { border-inline-start: var(--bw-hairline) solid var(--border-subtle); }
  .day[data-other] { background: var(--bg-surface-sunken); color: var(--text-subtle); }
  .number { font-size: var(--fs-065); color: var(--text-secondary); }
  .day[data-today] .number { color: var(--text-brand); font-weight: var(--fw-semibold); }
  .entry { display: block; inline-size: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: start; font-size: var(--fs-065); color: var(--text-primary); background: var(--accent-primary-subtle); border: none; border-radius: var(--r-xs); padding-inline: var(--sp-050); cursor: pointer; }
  .more { font-size: var(--fs-065); color: var(--text-secondary); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target beyond its drawn box (P-2). */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    /* The entry clips its words, so its press grows as itself, not by an overhang. */
    .entry { display: flex; align-items: center; min-block-size: var(--layout-target); }
  }
</style>
