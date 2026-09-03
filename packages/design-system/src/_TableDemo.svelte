<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import IconButton from './IconButton.svelte';
  import Table, { type Column } from './Table.svelte';

  const { mode = 'entries' }: { mode?: 'entries' | 'wide' | 'long' } = $props();

  const columns: Column[] = [
    { id: 'title', label: 'Title' },
    { id: 'bucket', label: 'Bucket' },
    { id: 'count', label: 'Open', align: 'end' },
    { id: 'actions', label: 'Actions', isLabelHidden: true, align: 'end' },
  ];

  // German headings are what push a table past its container first, which is why the wide case is
  // the German one rather than an invented column count.
  const wide: Column[] = [
    { id: 'title', label: 'Titel' },
    { id: 'bucket', label: 'Arbeitsschritt' },
    { id: 'assignee', label: 'Zuständige Person' },
    { id: 'due', label: 'Fälligkeitsdatum' },
    { id: 'labels', label: 'Bezeichnungen' },
    { id: 'count', label: 'Offene Teilaufgaben', align: 'end' },
  ];

  const rows = [
    { title: 'Move the socket by the window', bucket: 'Electrics', count: 2 },
    { title: 'Order the tiles', bucket: 'Materials', count: 0 },
    { title: 'Book the electrician', bucket: 'Electrics', count: 1 },
  ];
</script>

{#if mode === 'wide' || mode === 'long'}
  <Table label="Einträge dieser Sammlung" columns={wide}>
    {#each rows as row (row.title)}
      <tr>
        <td>{row.title}</td>
        <td>{row.bucket}</td>
        <td>Anna Winkel</td>
        <td>4. September 2026</td>
        <td>Renovierung</td>
        <td data-align="end">{row.count}</td>
      </tr>
    {/each}
  </Table>
{:else}
  <Table label="Entries in this collection" {columns}>
    {#each rows as row (row.title)}
      <tr>
        <td>{row.title}</td>
        <td><Badge>{row.bucket}</Badge></td>
        <td data-align="end">{row.count}</td>
        <td data-align="end">
          <IconButton icon="ellipsis" label={`Actions for ${row.title}`} size="sm" />
        </td>
      </tr>
    {/each}
  </Table>
{/if}
