<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import DataTable from './DataTable.svelte';
  import EmptyState from './EmptyState.svelte';
  import FilterBar, { type Filter } from './FilterBar.svelte';
  import LogPage from './LogPage.svelte';
  import Refused from './_RefusedDemo.svelte';
  import { audit, auditColumns, type AuditRecord } from './_pageFixtures.ts';

  const { content = 'full' }: { content?: 'full' | 'empty' | 'refused' } = $props();

  const auditFilters: Filter[] = [
    { id: 'range', label: 'Last 7 days', isActive: true },
    { id: 'who', label: 'Who' },
    { id: 'kind', label: 'Kind of thing' },
    { id: 'outcome', label: 'Outcome: refused', isActive: true },
  ];
</script>

<LogPage
  head={{ title: 'Audit log', subtitle: 'Who did what and when — never what anybody wrote', breadcrumb: { trail: [{ id: 'admin', label: 'Administration', href: '#admin' }, { id: 'audit', label: 'Audit log' }], label: 'Where you are', expandLabel: 'Show every level' }, secondary: [{ label: 'Export…', onclick: () => {} }] }}
  refused={content === 'refused' ? refused : undefined}
>
  {#snippet filters()}
    <FilterBar searchLabel="Action, e.g. sign-in" clearSearchLabel="Clear the search" filters={auditFilters} moreLabel="More filters" filtersLabel="Filters (2)" sheetTitle="Filters" sheetDismissLabel="Close" sheetGrabLabel="Pull to full height" showLabel="Show 23 records" clearLabel="Clear" countLabel={content === 'empty' ? 'None found' : '23 found'}>
      {#snippet picker()}<Checkbox label="Any" checked />{/snippet}
    </FilterBar>
  {/snippet}
  <DataTable
    label="Audit log"
    columns={auditColumns}
    rows={content === 'empty' ? [] : audit}
    key={(record: AuditRecord) => record.id}
    onopen={() => {}}
    group={(record: AuditRecord) => record.day}
    shownLabel={content === 'empty' ? undefined : 'Showing 50 · more to read'}
    more={content === 'empty' ? undefined : { label: 'Show 50 more', hasMore: true }}
    perPage={content === 'empty' ? undefined : { label: 'Rows', value: 50, onchange: () => {} }}
  >
    {#snippet cell(record: AuditRecord, column)}
      {#if column.id === 'when'}{record.time} · {record.who}
      {:else if column.id === 'who'}{record.who}
      {:else if column.id === 'what'}{record.what}
      {:else if column.id === 'about'}{record.about}
      {:else}<Badge size="sm" tone="danger">Refused</Badge>{/if}
    {/snippet}
    {#snippet empty()}
      <EmptyState kind="filtered" title="Nothing was refused in the last 7 days" description="Widen the time range or clear the filters.">
        {#snippet action()}<Button>Clear filters</Button>{/snippet}
      </EmptyState>
    {/snippet}
  </DataTable>
</LogPage>

{#snippet refused()}<Refused what="the audit log" />{/snippet}
