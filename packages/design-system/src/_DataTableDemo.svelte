<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import BulkBar from './BulkBar.svelte';
  import Button from './Button.svelte';
  import DataTable from './DataTable.svelte';
  import Stack from './Stack.svelte';
  import PeopleTable from './_PeopleTable.svelte';
  import { audit, auditColumns, type AuditRecord } from './_pageFixtures.ts';

  const { mode = 'full' }: { mode?: 'full' | 'empty' | 'filtered' | 'selecting' | 'busy' | 'log' } = $props();

  let isSelecting = $state(false);
  let selected = $state<string[]>([]);
</script>

{#if mode === 'log'}
  <DataTable
    label="Audit log"
    columns={auditColumns}
    rows={audit}
    key={(record: AuditRecord) => record.id}
    onopen={() => {}}
    group={(record: AuditRecord) => record.day}
    shownLabel="Showing 50 · more to read"
    more={{ label: 'Show 50 more', hasMore: true }}
    perPage={{ label: 'Rows per page', value: 50, onchange: () => {} }}
  >
    {#snippet cell(record: AuditRecord, column)}
      {#if column.id === 'when'}{record.time} · {record.who}
      {:else if column.id === 'who'}{record.who}
      {:else if column.id === 'what'}{record.what}
      {:else if column.id === 'about'}{record.about}
      {:else}<Badge size="sm" tone="danger">Refused</Badge>{/if}
    {/snippet}
  </DataTable>
{:else if mode === 'selecting'}
  <Stack gap="200">
    {#if isSelecting}
      <BulkBar
        label={`${selected.length} selected`}
        actions={[{ id: 'role', label: 'Change role…' }, { id: 'group', label: 'Add to a group…' }]}
        menu={{ label: 'More for the selected', items: [{ id: 'revoke', label: 'Revoke access…', isDestructive: true }] }}
        doneLabel="Done"
        onDone={() => { isSelecting = false; selected = []; }}
      />
    {:else}
      <span><Button size="sm" icon="square-check" onclick={() => (isSelecting = true)}>Select</Button></span>
    {/if}
    <PeopleTable {isSelecting} bind:selected />
  </Stack>
{:else}
  <PeopleTable content={mode === 'empty' || mode === 'filtered' ? mode : 'full'} isBusy={mode === 'busy'} />
{/if}
