<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import EmptyState from './EmptyState.svelte';
  import IconButton from './IconButton.svelte';
  import Pagination from './Pagination.svelte';
  import Table, { type Column } from './Table.svelte';
  import { comparing, pageOf, type Sort } from './table.ts';

  const { mode = 'entries' }: {
    mode?: 'entries' | 'wide' | 'sorted' | 'paged' | 'states' | 'empty';
  } = $props();

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

  /** The sortable set. The actions column has a heading it never draws, and is not sortable. */
  const sortable: Column[] = [
    { id: 'title', label: 'Title', isSortable: true },
    { id: 'bucket', label: 'Bucket', isSortable: true },
    { id: 'due', label: 'Due', isSortable: true },
    { id: 'count', label: 'Open', isSortable: true, align: 'end' },
    { id: 'actions', label: 'Actions', isLabelHidden: true, align: 'end' },
  ];

  interface Row {
    readonly title: string;
    readonly bucket: string;
    readonly count: number;
    readonly due: string | null;
  }

  const rows: Row[] = [
    { title: 'Move the socket by the window', bucket: 'Electrics', count: 2, due: '2026-10-02' },
    { title: 'Order the tiles', bucket: 'Materials', count: 0, due: null },
    { title: 'Book the electrician', bucket: 'Electrics', count: 1, due: '2026-09-29' },
    { title: 'Measure the hallway', bucket: 'Planning', count: 4, due: '2026-09-28' },
    { title: 'Ask about the skirting', bucket: 'Materials', count: 0, due: null },
  ];

  /** A list long enough to have pages, built rather than typed out. */
  const many: Row[] = Array.from({ length: 91 }, (_, index) => {
    const source = rows[index % rows.length] as Row;
    return { ...source, title: `${source.title} (${index + 1})`, count: (index * 3) % 7 };
  });

  let sort = $state<Sort>({ columnId: 'due', direction: 'ascending' });
  let page = $state(1);

  const read = (row: Row): string | number | null => {
    if (sort.columnId === 'count') return row.count;
    if (sort.columnId === 'due') return row.due;
    if (sort.columnId === 'bucket') return row.bucket;
    return row.title;
  };

  // The client holds the whole list, so it sorts and pages it here. A cursor-paged list would
  // hand `onSort` to the server and reach for `LoadMore` instead - see `Pagination`'s own note.
  const sorted = $derived([...rows].sort(comparing(read, sort.direction, 'en')));
  const shown = $derived(pageOf([...many].sort(comparing(read, sort.direction, 'en')), page, 12));
</script>

{#if mode === 'wide'}
  <Table label="Einträge dieser Sammlung" columns={wide}>
    {#each rows.slice(0, 3) as row (row.title)}
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
{:else if mode === 'sorted'}
  <Table
    label="Entries in this collection"
    columns={sortable}
    {sort}
    onSort={(next) => (sort = next)}
  >
    {#each sorted as row (row.title)}
      <tr>
        <td>{row.title}</td>
        <td><Badge>{row.bucket}</Badge></td>
        <td>{row.due ?? '—'}</td>
        <td data-align="end">{row.count}</td>
        <td data-align="end">
          <IconButton icon="ellipsis" label={`Actions for ${row.title}`} size="sm" />
        </td>
      </tr>
    {/each}
  </Table>
{:else if mode === 'paged'}
  <Table
    label="Entries in this collection"
    columns={sortable}
    {sort}
    onSort={(next) => {
      sort = next;
      // Back to the first page: the rows under the reader have all changed, and page 4 of the new
      // order is not the page they were looking at.
      page = 1;
    }}
  >
    {#each shown.rows as row (row.title)}
      <tr>
        <td>{row.title}</td>
        <td><Badge>{row.bucket}</Badge></td>
        <td>{row.due ?? '—'}</td>
        <td data-align="end">{row.count}</td>
        <td data-align="end">
          <IconButton icon="ellipsis" label={`Actions for ${row.title}`} size="sm" />
        </td>
      </tr>
    {/each}
  </Table>
  <Pagination
    total={many.length}
    pageSize={12}
    page={shown.page}
    label="Pages of entries"
    rangeLabel={`${shown.firstRow}–${shown.lastRow} of ${shown.total}`}
    previousLabel="Previous page"
    nextLabel="Next page"
    pageLabel={(n) => `Page ${n}`}
    gapLabel={(from, to) => `Pages ${from} to ${to}`}
    onPage={(n) => (page = n)}
  />
{:else if mode === 'states'}
  <Table label="Entries in this collection" columns={sortable} {sort} onSort={(next) => (sort = next)} isBusy>
    {#each sorted as row, index (row.title)}
      <tr data-selected={index === 1 ? '' : undefined}>
        <td>{row.title}</td>
        <td><Badge>{row.bucket}</Badge></td>
        <td>{row.due ?? '—'}</td>
        <td data-align="end">{row.count}</td>
        <td data-align="end">
          <IconButton icon="ellipsis" label={`Actions for ${row.title}`} size="sm" />
        </td>
      </tr>
    {/each}
  </Table>
{:else if mode === 'empty'}
  <Table label="Entries in this collection" columns={sortable}>
    {#snippet empty()}
      <EmptyState
        kind="filtered"
        title="No entry matches this filter"
        description="Widen the filter, or clear it to see every entry again."
      />
    {/snippet}
    {''}
  </Table>
{:else}
  <Table label="Entries in this collection" {columns}>
    {#each rows.slice(0, 3) as row (row.title)}
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
