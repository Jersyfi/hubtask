<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The state a real caller holds, so the workbench shows a control that answers rather than a
  // picture of one. Every string is resolved English here for the same reason it is in the other
  // demos: the component never sees a code (ADR-0011).

  import InContext from './_InContext.svelte';
  import Self from './_AssigneeDemo.svelte';
  import AssigneeControl, { type Candidate } from './AssigneeControl.svelte';

  const {
    mode = 'single',
    reason,
  }: { mode?: 'single' | 'multiple' | 'empty' | 'unavailable' | 'row' | 'table' | 'missing'; reason?: string } = $props();

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
    selected = mode === 'multiple' ? ['p1', 'p3'] : mode === 'missing' ? [] : ['p2'];
  });
</script>

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="Assigning is not available for this type of entry.">
    {#snippet subject({ reason })}
      <Self {reason} />
    {/snippet}
  </InContext>
{:else}
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
  disabledReason={reason ?? (mode === 'unavailable'
    ? 'This installation assigns one person per entry, so a set cannot be chosen here.'
    : undefined)}
  error={mode === 'missing' && selected.length === 0 ? 'Choose who to assign them to.' : undefined}
  onSelect={(ids) => {
    selected = ids;
  }}
/>
{/if}
