<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The state a real caller holds, so the workbench shows a control that answers rather than a
  // picture of one. Every string is resolved English here for the same reason it is in the other
  // demos: the component never sees a code (ADR-0011).

  import AssigneeControl, { type Candidate } from './AssigneeControl.svelte';

  const {
    mode = 'single',
  }: { mode?: 'single' | 'multiple' | 'empty' | 'unavailable' } = $props();

  const people: Candidate[] = [
    { id: 'p1', name: 'Amelie Fischer' },
    { id: 'p2', name: 'Jonas Brandt' },
    { id: 'p3', name: '陈 伟' },
    { id: 'p4', name: 'Sofia Marchetti' },
    { id: 'p5', name: 'Tobias Lang' },
  ];

  // Seeded once, then owned by this demo: the story picks the starting selection, and what
  // happens after that is the caller's state, which is the half a static picture cannot show.
  let selected = $state<readonly string[]>([]);
  $effect(() => {
    selected = mode === 'multiple' ? ['p1', 'p3'] : ['p2'];
  });
</script>

<AssigneeControl
  label="Assignee"
  candidates={mode === 'empty' ? [] : people}
  selection={mode === 'multiple' ? 'multiple' : 'single'}
  {selected}
  filterLabel="Filter people"
  emptyLabel="Nobody here can be assigned yet. Invite someone to this hub first."
  noMatchLabel="No one matches that. Try part of a name."
  chosenLabel="Assigned"
  unassignedLabel="Nobody yet"
  disabledReason={mode === 'unavailable'
    ? 'This installation assigns one person per entry, so a set cannot be chosen here.'
    : undefined}
  onSelect={(ids) => {
    selected = ids;
  }}
/>
