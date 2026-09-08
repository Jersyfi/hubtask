<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A fortnight of a small renovation: two spans, two points, and two entries nobody has dated.
  // The ticks are the caller's, because where a week starts is the reader's account's business.

  import Timeline, { type TimelineRow, type TimelineTick } from './Timeline.svelte';

  const { mode = 'mixed' }: { mode?: 'mixed' | 'undatedOnly' | 'empty' } = $props();

  const rows: TimelineRow[] = [
    { id: 'r1', title: 'Kitchen fit-out', start: '2026-09-07', due: '2026-09-14' },
    { id: 'r2', title: 'Electrics first fix', start: '2026-09-09', due: '2026-09-11' },
    { id: 'r3', title: 'Delivery slot', due: '2026-09-17' },
    { id: 'r4', title: 'Sign off with the client', due: '2026-09-20' },
    { id: 'r5', title: 'Choose the tiles' },
    { id: 'r6', title: 'Chase the warranty paperwork' },
  ];

  const ticks: TimelineTick[] = [
    { at: '2026-09-07', label: 'Mon 7' },
    { at: '2026-09-14', label: 'Mon 14' },
    { at: '2026-09-21', label: 'Mon 21' },
  ];
</script>

<Timeline
  label="The next fortnight"
  from="2026-09-07"
  to="2026-09-21"
  rows={mode === 'empty' ? [] : mode === 'undatedOnly' ? rows.filter((row) => !row.start && !row.due) : rows}
  ticks={mode === 'empty' ? [] : ticks}
  undatedLabel="Not on the axis yet"
  emptyLabel="Nothing to place here. Give an entry a due date and it appears."
/>
