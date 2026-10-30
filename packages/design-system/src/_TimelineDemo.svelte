<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A fortnight of a small renovation: two spans, two points, and two entries nobody has dated.
  // The gridlines are the caller's, because where a week starts is the reader's account's business
  // and how a day is spelled is a question this package answers in no language.

  import IconButton from './IconButton.svelte';
  import Timeline, {
    type TimelineCarry, type TimelineDependency, type TimelineLink, type TimelineRow, type TimelineTick,
  } from './Timeline.svelte';

  const {
    mode = 'mixed',
    scale = 'day',
  }: { mode?: 'mixed' | 'undatedOnly' | 'empty' | 'locked' | 'actions' | 'dependencies' | 'gantt'; scale?: 'day' | 'week' | 'month' | 'quarter' | 'year' } = $props();

  // The entry open beside the timeline, and what a row's menu or a long press last asked for.
  let current = $state<string | undefined>('r2');
  let asked = $state<string | undefined>(undefined);

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

  // A Gantt chart: a summary over two children, a critical chain, a baseline, a milestone, and the
  // dependencies drawn by dragging from a bar's knob onto another row.
  let gantt = $state<TimelineRow[]>([
    { id: 'g0', title: 'Relaunch', start: '2026-09-07', due: '2026-09-18', kind: 'summary' },
    { id: 'g1', title: 'Approve the design', start: '2026-09-07', due: '2026-09-09', depth: 1, isCritical: true, baseline: { start: '2026-09-07', due: '2026-09-08' } },
    { id: 'g2', title: 'Build the pages', start: '2026-09-10', due: '2026-09-15', depth: 1, isCritical: true, baseline: { start: '2026-09-09', due: '2026-09-11' } },
    { id: 'g3', title: 'Write the copy', start: '2026-09-09', due: '2026-09-11', depth: 1 },
    { id: 'g4', title: 'Go live', start: '2026-09-16', due: '2026-09-16', kind: 'milestone', isCritical: true },
  ]);
  let drawnLinks = $state<TimelineDependency[]>([
    { id: 'l1', from: 'g1', to: 'g2', label: 'Build the pages waits for Approve the design' },
    { id: 'l2', from: 'g2', to: 'g4', label: 'Go live waits for Build the pages' },
  ]);
  function linked(link: TimelineLink) {
    const title = (id: string) => gantt.find((row) => row.id === id)?.title ?? id;
    drawnLinks = [...drawnLinks, { id: `l${drawnLinks.length + 1}`, ...link, label: `${title(link.to)} waits for ${title(link.from)}` }];
  }

  // A reader who may change only what is on them: the fit-out and one undated entry are someone
  // else's, so they show where they are and cannot be carried.
  const LOCKED = new Set(['r1', 'r5']);
  const shown = $derived(
    mode === 'empty'
      ? []
      : mode === 'undatedOnly'
        ? rows.filter((row) => !row.start && !row.due)
        : mode === 'locked'
          ? rows.map((row) => ({ ...row, isLocked: LOCKED.has(row.id) }))
          : mode === 'gantt'
            ? gantt
            : rows,
  );


  // The electrics wait for the fit-out to start nothing - they start before it is due, which is
  // the case drawn dashed; the sign-off waits for the delivery in good time.
  const dependencies: TimelineDependency[] = [
    { id: 'd1', from: 'r1', to: 'r2', isEarly: true, label: 'Electrics first fix waits for Kitchen fit-out - and starts before it is due' },
    { id: 'd2', from: 'r3', to: 'r4', label: 'Sign off with the client waits for Delivery slot' },
  ];

  const columnOf = (date: string) => Math.round((Date.parse(`${date}T00:00:00Z`) - Date.parse(`${FROM}T00:00:00Z`)) / DAY);
</script>

<Timeline
  label="The next fortnight"
  from={FROM}
  to="2026-09-21"
  {scale}
  rows={shown}
  gridlines={mode === 'empty' ? [] : gridlines}
  today="2026-09-11"
  todayLabel="Today is Friday 11 September"
  undatedLabel="Not on the axis yet"
  emptyLabel="Nothing to place here. Give an entry a due date and it appears."
  startHandleLabel="Move the start"
  dueHandleLabel="Move the due date"
  onCarry={carried}
  currentId={mode === 'actions' ? current : undefined}
  onSelect={mode === 'actions' ? (id) => (current = id) : undefined}
  onHold={mode === 'actions' ? (id) => (asked = id) : undefined}
  rowAction={mode === 'actions' ? rowAction : undefined}
  dependencies={mode === 'dependencies' ? dependencies : mode === 'gantt' ? drawnLinks : []}
  dependenciesLabel="Dependencies"
  onLink={mode === 'gantt' ? linked : undefined}
  linkHandleLabel="Draw a dependency from here"
/>
{#if mode === 'actions'}
  <p>Asked for the menu of: {asked ?? 'nothing yet'}</p>
{/if}

{#snippet rowAction(id: string)}
  <IconButton icon="ellipsis" label={`Actions for ${rows.find((row) => row.id === id)?.title ?? id}`} size="sm" onclick={() => (asked = id)} />
{/snippet}
