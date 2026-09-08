<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The caller's half: the reader's zone comes from their account, and the sentence about a
  // foreign zone is formed here because a component writes none.

  import DueDateControl, { type DueDate } from './DueDateControl.svelte';

  const { mode = 'timed' }: { mode?: 'timed' | 'allDay' | 'foreign' | 'none' } = $props();

  const READER_ZONE = 'Europe/Berlin';

  let value = $state<DueDate | null>(null);

  $effect(() => {
    value =
      mode === 'none'
        ? null
        : mode === 'allDay'
          ? { date: '2026-09-17' }
          : mode === 'foreign'
            ? { date: '2026-09-17', time: '09:00', zone: 'America/Sao_Paulo' }
            : { date: '2026-09-17', time: '14:30', zone: READER_ZONE };
  });
</script>

<DueDateControl
  label="Due"
  {value}
  readerZone={READER_ZONE}
  dateLabel="Date"
  timeLabel="Time"
  allDayLabel="All day"
  clearLabel="Remove the due date"
  noDateLabel="No due date. Add one to see this entry on the timeline."
  zoneNoteLabel={mode === 'foreign' ? '09:00 there is 14:00 where you are' : undefined}
  onChange={(next) => {
    value = next;
  }}
/>
