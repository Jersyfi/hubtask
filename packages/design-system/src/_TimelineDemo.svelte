<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A fortnight of a small renovation: two spans, two points, and two entries nobody has dated.
  // The gridlines are the caller's, because where a week starts is the reader's account's business
  // and how a day is spelled is a question this package answers in no language.

  import Timeline, { type TimelineCarry, type TimelineRow, type TimelineTick } from './Timeline.svelte';

  const { mode = 'mixed', scale = 'day' }: { mode?: 'mixed' | 'undatedOnly' | 'empty'; scale?: 'day' | 'week' | 'month' } =
    $props();

  let rows = $state<TimelineRow[]>([
    { id: 'r1', title: 'Kitchen fit-out', start: '2026-09-07', due: '2026-09-14' },
    { id: 'r2', title: 'Electrics first fix', start: '2026-09-09', due: '2026-09-11' },
    { id: 'r3', title: 'Delivery slot', due: '2026-09-17' },
    { id: 'r4', title: 'Sign off with the client', due: '2026-09-20' },
    { id: 'r5', title: 'Choose the tiles' },
    { id: 'r6', title: 'Chase the warranty paperwork' },
  ]);

  const FROM = '2026-09-07';
  const DAY = 86_400_000;
  const dayAt = (column: number) => new Date(Date.parse(`${FROM}T00:00:00Z`) + column * DAY).toISOString().slice(0, 10);

  const gridlines: TimelineTick[] = [
    { at: '2026-09-07', label: 'Mon 7' },
    { at: '2026-09-14', label: 'Mon 14' },
    { at: '2026-09-21', label: 'Mon 21' },
  ];

  // The workbench stands in for the application: a carry names columns, and what they mean for a
  // start and a due date is the caller's — here, the shortest possible version of it.
  function carried(carry: TimelineCarry) {
    const moved = carry.to - carry.from;
    rows = rows.map((row) => {
      if (row.id !== carry.id) return row;
      if (carry.grab === 'place') {
        const [first, last] = [Math.min(carry.from, carry.to), Math.max(carry.from, carry.to)];
        return { ...row, start: dayAt(first), due: dayAt(last) };
      }
      const shift = (date: string | undefined) => (date === undefined ? undefined : dayAt(columnOf(date) + moved));
      if (carry.grab === 'bar') return { ...row, start: shift(row.start), due: shift(row.due) };
      if (carry.grab === 'start') return { ...row, start: shift(row.start) };
      return { ...row, due: shift(row.due) };
    });
  }

  const columnOf = (date: string) => Math.round((Date.parse(`${date}T00:00:00Z`) - Date.parse(`${FROM}T00:00:00Z`)) / DAY);
</script>

<Timeline
  label="The next fortnight"
  from={FROM}
  to="2026-09-21"
  {scale}
  rows={mode === 'empty' ? [] : mode === 'undatedOnly' ? rows.filter((row) => !row.start && !row.due) : rows}
  gridlines={mode === 'empty' ? [] : gridlines}
  today="2026-09-11"
  todayLabel="Today is Friday 11 September"
  undatedLabel="Not on the axis yet"
  emptyLabel="Nothing to place here. Give an entry a due date and it appears."
  startHandleLabel="Move the start"
  dueHandleLabel="Move the due date"
  onCarry={carried}
/>
