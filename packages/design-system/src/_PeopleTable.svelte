<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The People table of the mockup, shared by the DataTable story and the page kinds' stories.
  import Badge from './Badge.svelte';
  import Button from './Button.svelte';
  import DataTable from './DataTable.svelte';
  import EmptyState from './EmptyState.svelte';
  import { people, peopleColumns, personMenu, standingTone, type Person } from './_pageFixtures.ts';
  import type { Sort } from './table.ts';

  interface Props {
    content?: 'full' | 'empty' | 'filtered';
    isSelecting?: boolean;
    isBusy?: boolean;
    selected?: string[];
  }

  let { content = 'full', isSelecting = false, isBusy = false, selected = $bindable([]) }: Props = $props();

  let sort = $state<Sort>({ columnId: 'name', direction: 'ascending' });
  let perPage = $state(50);
  let last = $state('');

  const rows = $derived.by(() => {
    if (content !== 'full') return [];
    const sorted = [...people];
    if (sort.columnId === 'name' && sort.direction !== 'none') {
      sorted.sort((a, b) => a.name.localeCompare(b.name) * (sort.direction === 'ascending' ? 1 : -1));
    }
    return sorted;
  });
</script>

<DataTable
  label="People"
  columns={peopleColumns}
  {rows}
  key={(person: Person) => person.id}
  href={(person: Person) => `#person-${person.id}`}
  rowLabel={(person: Person) => person.name}
  menu={personMenu}
  onmenu={(person: Person, id: string) => (last = `${id} · ${person.name}`)}
  actionsLabel="Actions"
  sheetDismissLabel="Close"
  sheetGrabLabel="Pull to full height"
  {sort}
  onSort={(next) => (sort = next)}
  {isSelecting}
  bind:selected
  {isBusy}
  shownLabel={content === 'full' ? `Showing ${rows.length} of ${rows.length}` : undefined}
  perPage={content === 'full' ? { label: 'Rows per page', value: perPage, onchange: (value) => (perPage = value) } : undefined}
>
  {#snippet cell(person: Person, column)}
    {#if column.id === 'name'}{person.name}
    {:else if column.id === 'email'}{person.email}
    {:else if column.id === 'role'}<Badge size="sm">{person.role}</Badge>
    {:else if column.id === 'groups'}{person.groups}
    {:else if column.id === 'standing'}<Badge size="sm" tone={standingTone[person.standing]}>{person.standing}</Badge>
    {:else}{person.seen}{/if}
  {/snippet}
  {#snippet empty()}
    {#if content === 'filtered'}
      <EmptyState kind="filtered" title="No one matches these filters" description="Nobody with the role Auditor is active.">
        {#snippet action()}<Button>Clear filters</Button>{/snippet}
      </EmptyState>
    {:else}
      <EmptyState kind="unused" title="Nobody is here yet" description="Add the people who work in this workspace; they get an invitation by mail." icon="users">
        {#snippet action()}<Button tone="primary" icon="plus">Add people</Button>{/snippet}
      </EmptyState>
    {/if}
  {/snippet}
</DataTable>
{#if last}<p class="last">Chosen: {last}</p>{/if}

<style>
  .last { margin: var(--sp-150) 0 0; color: var(--text-secondary); font-size: var(--fs-100); }
</style>
