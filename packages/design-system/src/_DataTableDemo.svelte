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

  import IconButton from './IconButton.svelte';

  const { mode = 'full' }: { mode?: 'full' | 'empty' | 'filtered' | 'selecting' | 'busy' | 'log' | 'tree' } = $props();

  // A tree in the table: each row's fold stands before its title, outside the press that opens it.
  interface Kind { readonly id: string; readonly name: string; readonly depth: 0 | 1 | 2; readonly below: number }
  const kinds: readonly Kind[] = [
    { id: 'ci', name: 'Configuration item', depth: 0, below: 3 },
    { id: 'hardware', name: 'Hardware', depth: 1, below: 2 },
    { id: 'server', name: 'Server', depth: 2, below: 0 },
    { id: 'laptop', name: 'Laptop', depth: 2, below: 0 },
    { id: 'software', name: 'Software', depth: 1, below: 0 },
  ];
  let folded = $state<string[]>([]);
  const shownKinds = $derived.by(() => {
    const out: Kind[] = [];
    let hideBelow = -1;
    for (const kind of kinds) {
      if (hideBelow >= 0 && kind.depth > hideBelow) continue;
      hideBelow = folded.includes(kind.id) ? kind.depth : -1;
      out.push(kind);
    }
    return out;
  });

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
    more={{ label: 'Show 50 more', hasMore: true, arrivedLabel: '' }}
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
{:else if mode === 'tree'}
  <DataTable
    label="Classes"
    columns={[{ id: 'name', label: 'Class', priority: 1 }, { id: 'below', label: 'Classes below', priority: 2 }]}
    rows={shownKinds}
    key={(kind: Kind) => kind.id}
    href={(kind: Kind) => `#${kind.id}`}
  >
    {#snippet lead(kind: Kind)}
      <span class="indent" data-depth={String(kind.depth)}></span>
      {#if kind.below > 0}
        <IconButton icon={folded.includes(kind.id) ? 'chevron-right' : 'chevron-down'} size="sm" tone="subtle"
          label={`${folded.includes(kind.id) ? 'Show' : 'Fold'} the classes below ${kind.name}`} aria-expanded={!folded.includes(kind.id)}
          onclick={() => (folded = folded.includes(kind.id) ? folded.filter((id) => id !== kind.id) : [...folded, kind.id])} />
      {/if}
    {/snippet}
    {#snippet cell(kind: Kind, column)}
      {#if column.id === 'name'}{kind.name}{:else}{kind.below > 0 ? kind.below : '—'}{/if}
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

<style>
  .indent[data-depth='1'] { inline-size: var(--sp-150); }
  .indent[data-depth='2'] { inline-size: var(--sp-300); }
</style>
