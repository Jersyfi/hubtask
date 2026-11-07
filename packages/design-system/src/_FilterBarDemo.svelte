<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Checkbox from './Checkbox.svelte';
  import FilterBar, { type Filter } from './FilterBar.svelte';
  import Radio from './Radio.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'people' }: { mode?: 'people' | 'audit' } = $props();

  let search = $state('');
  let roleOn = $state(true);

  const peopleFilters = $derived<Filter[]>([
    { id: 'role', label: roleOn ? 'Role: Administrator' : 'Role: any', isActive: roleOn },
    { id: 'standing', label: 'Standing: active' },
    { id: 'group', label: 'Group' },
    { id: 'seen', label: 'Last seen' },
  ]);

  const auditFilters: Filter[] = [
    { id: 'range', label: 'Last 7 days', isActive: true },
    { id: 'who', label: 'Who' },
    { id: 'kind', label: 'Kind of thing' },
    { id: 'outcome', label: 'Outcome: refused', isActive: true },
  ];
</script>

<FilterBar
  searchLabel={mode === 'audit' ? 'Action, e.g. sign-in' : 'Search people'}
  clearSearchLabel="Clear the search"
  bind:search
  filters={mode === 'audit' ? auditFilters : peopleFilters}
  moreLabel="More filters"
  filtersLabel={mode === 'audit' ? 'Filters (2)' : roleOn ? 'Filters (1)' : 'Filters'}
  sheetTitle="Filters"
  sheetDismissLabel="Close"
  sheetGrabLabel="Pull to full height"
  showLabel={mode === 'audit' ? 'Show 23 records' : 'Show 9 people'}
  clearLabel="Clear"
  onclear={() => (roleOn = false)}
  countLabel={mode === 'audit' ? '23 found' : roleOn ? '1 of 9' : '9 of 9'}
>
  {#snippet picker(filter)}
    <Stack gap="100">
      {#if filter.id === 'role'}
        <Checkbox label="Administrator" bind:checked={roleOn} />
        <Checkbox label="Member" />
        <Checkbox label="Viewer" />
      {:else if filter.id === 'range'}
        <Radio label="Time range" options={[{ value: 'today', label: 'Today' }, { value: '7', label: 'Last 7 days' }, { value: '30', label: 'Last 30 days' }]} value="7" />
      {:else}
        <Checkbox label="Any" checked />
      {/if}
    </Stack>
  {/snippet}
</FilterBar>
