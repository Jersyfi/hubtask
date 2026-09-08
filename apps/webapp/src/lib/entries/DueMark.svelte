<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a row says about when an entry is wanted.
  //
  // **Never colour alone** (`design-system.md` §6, rule 3). Overdue carries an icon *and* a word,
  // and both of them carry the date — a reader who cannot tell red from grey learns the same thing
  // from the same row, and so does one reading it in a screenshot printed in black and white.
  //
  // **The relative phrase is an addition, never a replacement.** "in 3 days" does not say which
  // day, and somebody planning needs the day. So the mark reads the date, and says how far away it
  // is beside it.
  //
  // **The date is drawn in the zone it was set in**, and says which zone when that is not the
  // reader's. An all-day date shows no time at all.

  import { Badge } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { isDueSoon, isOverdue } from '../data/due.ts';
  import { belongsToSeries, occurrenceSourceOf } from '../data/reminders.ts';
  import { formatDue, formatRelative } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  const { item }: { item: WorkItem } = $props();

  /** How near counts as near. A day, which is the horizon a to-do list is read over. */
  const SOON_MS = 24 * 60 * 60 * 1000;

  const zone = $derived(actor.zone);
  // Read once per render rather than held: nothing here animates, and a timer that re-rendered
  // every row every minute would be a client burning a laptop's battery to move one word.
  const now = $derived.by(() => Date.now());

  const dueZone = $derived(item.due_time_zone ?? zone);
  const overdue = $derived(isOverdue(item, zone, now));
  const soon = $derived(isDueSoon(item, zone, now, SOON_MS));

  const date = $derived(
    item.due_at
      ? formatDue(item.due_at, messages.locale, dueZone, {
          allDay: item.due_date_only ?? false,
          showZone: dueZone !== zone,
        })
      : undefined,
  );
  const relative = $derived(item.due_at ? formatRelative(item.due_at, messages.locale, now) : '');
</script>

{#if belongsToSeries(item)}
  <!-- Which end, from the row itself: `recurrence_source_id` is on a copy the materialisation made
       and on nothing else, so a list marks the two differently without a request per row. -->
  <Badge icon="repeat">
    {occurrenceSourceOf(item) ? t('app.recurrence.one_of') : t('app.recurrence.repeats')}
  </Badge>
{/if}

{#if date}
  {#if overdue}
    <!-- An icon and a word and the date. The tone is the third signal, not the only one. -->
    <Badge tone="danger" icon="triangle-alert">
      {t('app.due.overdue_by', { relative })} · {date}
    </Badge>
  {:else if soon}
    <Badge tone="warning" icon="clock">
      {t('app.due.due_in', { relative })} · {date}
    </Badge>
  {:else}
    <Badge icon="calendar">{date}</Badge>
  {/if}
{/if}
