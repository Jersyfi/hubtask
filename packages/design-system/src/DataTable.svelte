<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script module lang="ts">
  import type { Column } from './Table.svelte';
  import type { MenuItem } from './overlay.ts';

  export interface DataColumn extends Column {
    /** 1 stays everywhere and is a list row's title; 2 is its line; 3 goes below `expanded`. */
    readonly priority?: 1 | 2 | 3;
    /** Whether a person may hide it; the caller offers the choice and keeps it. */
    readonly isHideable?: boolean;
    /** The state a list row shows at its end on a phone. */
    readonly isStatus?: boolean;
  }

  export interface RowMenu {
    readonly label: string;
    readonly items: readonly MenuItem[];
  }
</script>

<script lang="ts" generics="T">
  // Objects of one kind as one region on the canvas (page-system §6, PS-4, PS-6, PS-8): a `Table`
  // whose columns give way by priority, whose row opens its record, with one quiet verb and a ⋯
  // menu at its end, a selection mode, and a foot that says what is shown and continues the list.
  //
  // **The page scrolls, not the table** (§4.3): the head sticks under the app bar, and a column that
  // does not fit gives way rather than pushing the table sideways - priority 3 below `expanded`,
  // everything but the title, one line and the state on a phone, where each row is a list row:
  // priority 1 as its title, priority 2 under it, the column marked `isStatus` at the end, and ⋯
  // opening a `Sheet` of the row's verbs instead of an anchored menu.
  //
  // **A row is a link** where it has an address (`href`) - middle-click and *Open in new tab* work
  // - or a button that opens its dialog (`onopen`); the title cell carries it, stretched over the
  // row, so the ⋯ and the checkbox above it stay their own targets.
  //
  // It sorts, pages and filters nothing: it reports and draws, like `Table`. Which columns a person
  // hides, how many rows a page holds and what is selected are the caller's to keep.

  import type { Snippet } from 'svelte';

  import Checkbox from './Checkbox.svelte';
  import IconButton from './IconButton.svelte';
  import LoadMore from './LoadMore.svelte';
  import Menu from './Menu.svelte';
  import Select from './Select.svelte';
  import Sheet from './Sheet.svelte';
  import Table from './Table.svelte';
  import type { Sort } from './table.ts';

  interface Props {
    /** The table's name, announced. The heading on the canvas is the page's. */
    label: string;
    columns: readonly DataColumn[];
    rows: readonly T[];
    /** A row's stable key. */
    key: (row: T) => string;
    /** One cell's content. */
    cell: Snippet<[T, DataColumn]>;
    /**
     * What stands before a row's title, outside the press that opens the row: a tree's fold. A
     * control inside the press would be a button within a link and lie under its stretched box.
     */
    lead?: Snippet<[T]>;
    /** The columns the person hid. */
    hiddenColumns?: readonly string[];
    /** The row's address: its record page. */
    href?: (row: T) => string | undefined;
    /** Pressing a row that has no address: its edit dialog. */
    onopen?: (row: T) => void;
    /** The name of a row, for its menu and its checkbox: *Lukas Brandt*. */
    rowLabel?: (row: T) => string;
    /** The one quiet verb of a row, before its menu (PS-6). */
    verb?: Snippet<[T]>;
    /** A row's ⋯ menu, destructive items last. */
    menu?: (row: T) => RowMenu | undefined;
    onmenu?: (row: T, id: string) => void;
    /** The sheet's names on a phone. */
    sheetDismissLabel?: string;
    sheetGrabLabel?: string;
    /** The heading of the end column, announced only. */
    actionsLabel?: string;
    sort?: Sort;
    onSort?: (sort: Sort) => void;
    /** Selection mode: a checkbox per row; the page shows its `BulkBar`. */
    isSelecting?: boolean;
    selected?: string[];
    /** Rows are on their way: they fade where they are. */
    isBusy?: boolean;
    /** A group a row belongs to - a day - drawn as a sunken row where it changes. */
    group?: (row: T) => string;
    /** The empty state of the right kind (`EmptyState`), inside the region. */
    empty?: Snippet;
    /** *Showing 50 of 1 240*. */
    shownLabel?: string;
    /** What continues the list: `LoadMore`'s props. */
    more?: { label: string; busyLabel?: string; hasMore?: boolean; isBusy?: boolean; arrivedLabel?: string; endLabel?: string; failedLabel?: string; onLoadMore?: () => void };
    /** Rows per page, 25 · 50 · 100; the device keeps the choice. */
    perPage?: { label: string; value: number; onchange: (value: number) => void };
  }

  let {
    label,
    columns,
    rows,
    key,
    cell,
    lead,
    hiddenColumns = [],
    href,
    onopen,
    rowLabel,
    verb,
    menu,
    onmenu,
    sheetDismissLabel = '',
    sheetGrabLabel = '',
    actionsLabel = '',
    sort,
    onSort,
    isSelecting = false,
    selected = $bindable([]),
    isBusy = false,
    group,
    empty,
    shownLabel,
    more,
    perPage,
  }: Props = $props();

  const SELECT = '__select';
  const END = '__end';

  const visible = $derived(columns.filter((column) => !hiddenColumns.includes(column.id)));
  const hasEnd = $derived(verb !== undefined || menu !== undefined);
  const tableColumns = $derived<Column[]>([
    ...(isSelecting ? [{ id: SELECT, label: actionsLabel || label, isLabelHidden: true, priority: 1 as const }] : []),
    ...visible.map((column, index) => ({ ...column, priority: column.priority ?? (index === 0 ? 1 : 2) })),
    ...(hasEnd ? [{ id: END, label: actionsLabel, isLabelHidden: true, align: 'end' as const, priority: 1 as const }] : []),
  ]);

  const priorityOf = (column: DataColumn, index: number) => column.priority ?? (index === 0 ? 1 : 2);
  const titleColumn = $derived(visible.find((column, index) => priorityOf(column, index) === 1)?.id);

  /** The row whose verbs the phone's sheet shows. */
  let sheetRow = $state<T | null>(null);
  let isSheetOpen = $state(false);
  const sheetMenu = $derived(sheetRow !== null && menu ? menu(sheetRow) : undefined);

  function toggle(id: string, isChecked: boolean) {
    selected = isChecked ? [...selected.filter((value) => value !== id), id] : selected.filter((value) => value !== id);
  }

  const uid = $props.id();
  const perPageId = `per-page-${uid}`;

  const perPageOptions = [25, 50, 100].map((value) => ({ value: String(value), label: String(value) }));
</script>

<div class="data-table" data-selecting={isSelecting ? '' : undefined}>
  <Table {label} isLabelHidden columns={tableColumns} {sort} {onSort} {isBusy} {empty} isHeadPinned foot={shownLabel || more || perPage ? tableFoot : undefined}>
    {#each rows as row, index (key(row))}
      {@const id = key(row)}
      {@const rowGroup = group?.(row)}
      {#if rowGroup !== undefined && (index === 0 || group?.(rows[index - 1]!) !== rowGroup)}
        <tr class="group-row"><td colspan={tableColumns.length}>{rowGroup}</td></tr>
      {/if}
      {@const link = href?.(row)}
      <tr class="row" data-selected={selected.includes(id) ? '' : undefined} data-pressable={link || onopen ? '' : undefined}>
        {#if isSelecting}
          <td class="select" data-priority="1">
            <Checkbox label={rowLabel?.(row) ?? id} isLabelHidden checked={selected.includes(id)} onchange={(event) => toggle(id, event.currentTarget.checked)} />
          </td>
        {/if}
        {#each visible as column, columnIndex (column.id)}
          {@const priority = priorityOf(column, columnIndex)}
          <td
            data-priority={priority}
            data-align={column.align ?? 'start'}
            data-role={column.id === titleColumn ? 'title' : column.isStatus ? 'status' : priority === 2 ? 'line' : undefined}
          >
            {#if column.id === titleColumn && lead}<span class="lead">{@render lead(row)}</span>{/if}
            {#if column.id === titleColumn && link}
              <a class="press" href={link}>{@render cell(row, column)}</a>
            {:else if column.id === titleColumn && onopen}
              <button type="button" class="press" onclick={() => onopen(row)}>{@render cell(row, column)}</button>
            {:else}
              {@render cell(row, column)}
            {/if}
          </td>
        {/each}
        {#if hasEnd}
          {@const rowMenu = menu?.(row)}
          <td class="end" data-priority="1" data-align="end">
            <span class="end-inner">
              {#if verb}<span class="verb">{@render verb(row)}</span>{/if}
              {#if rowMenu && rowMenu.items.length > 0}
                <span class="menu-wide">
                  <Menu label={rowMenu.label} items={rowMenu.items} placement={{ side: 'block-end', align: 'end' }} onselect={(item) => onmenu?.(row, item)}>
                    {#snippet trigger(props)}
                      <IconButton icon="ellipsis" label={rowMenu.label} size="sm" {...props} />
                    {/snippet}
                  </Menu>
                </span>
                <span class="menu-phone">
                  <IconButton icon="ellipsis" label={rowMenu.label} size="sm" onclick={() => { sheetRow = row; isSheetOpen = true; }} />
                </span>
              {/if}
            </span>
          </td>
        {/if}
      </tr>
    {/each}
  </Table>
</div>

{#snippet tableFoot()}
    {#if shownLabel}<span class="shown">{shownLabel}</span>{/if}
    {#if more}
      <span class="more"><LoadMore {...more} /></span>
    {/if}
    {#if perPage}
      <span class="per-page">
        <span id={perPageId}>{perPage.label}</span>
        <Select label={perPage.label} isLabelHidden aria-labelledby={perPageId} size="sm" options={perPageOptions} value={String(perPage.value)} onchange={(event) => perPage.onchange(Number(event.currentTarget.value))} />
      </span>
    {/if}
{/snippet}

{#if menu}
  <Sheet
    bind:isOpen={isSheetOpen}
    title={sheetRow !== null ? (rowLabel?.(sheetRow) ?? sheetMenu?.label ?? '') : ''}
    closeLabel={sheetDismissLabel}
    grabLabel={sheetGrabLabel}
    items={sheetMenu?.items ?? []}
    onselect={(item) => { if (sheetRow !== null) onmenu?.(sheetRow, item); }}
  />
{/if}

<style>
  .data-table { min-width: 0; }

  /* The head as the mockups draw it: the table's own size, semibold, a hairline under it. */
  .data-table :global(thead th) {
    font-size: var(--fs-100);
    font-weight: var(--fw-semibold);
    border-block-end-width: var(--bw-hairline);
  }

  .data-table :global(td[data-role='title']) { font-weight: var(--fw-medium); color: var(--text-primary); }

  /* The row is pressable through its title: the link's box is stretched over the row, so the
     whole row answers the pointer while the ⋯ and the checkbox stay their own targets above it. */
  .data-table :global(tr.row[data-pressable]) { position: relative; cursor: pointer; }

  .press {
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: start;
    text-decoration: none;
    cursor: pointer;
  }

  .press::after { content: ''; position: absolute; inset: 0; }

  .press:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  .select,
  .lead,
  .end-inner { position: relative; z-index: var(--z-raised); }
  .lead { display: inline-flex; vertical-align: middle; }

  .end { white-space: nowrap; inline-size: 1%; }
  .end-inner { display: inline-flex; align-items: center; gap: var(--sp-100); }
  .menu-phone { display: none; }

  .data-table :global(tr.group-row td) {
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
  }

  .shown { color: var(--text-subtle); }
  .more { margin-inline: auto; }
  .per-page { display: inline-flex; align-items: center; gap: var(--sp-100); }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .data-table :global([data-priority='3']) { display: none; }
  }

  /* On a phone each row is a list row: title, one line under it, the state and ⋯ at the end. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .data-table :global(thead) { display: none; }
    .data-table :global(table),
    .data-table :global(tbody) { display: block; }
    .data-table :global(tr.row) {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto auto;
      grid-template-areas: 'title status menu' 'line status menu';
      align-items: center;
      column-gap: var(--sp-150);
      padding: var(--sp-150) var(--sp-200);
    }
    .data-table[data-selecting] :global(tr.row) {
      grid-template-columns: auto minmax(0, 1fr) auto auto;
      grid-template-areas: 'check title status menu' 'check line status menu';
    }
    .data-table :global(tr.row > td) { display: none; padding: 0; }
    .data-table :global(tr.row > td.select) { display: block; grid-area: check; }
    .data-table :global(tr.row > td[data-role='title']) { display: block; grid-area: title; }
    .data-table :global(tr.row > td[data-role='line']) { display: block; grid-area: line; color: var(--text-subtle); font-size: var(--fs-075); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .data-table :global(tr.row > td[data-role='line'] ~ td[data-role='line']) { display: none; }
    .data-table :global(tr.row > td[data-role='status']) { display: block; grid-area: status; }
    .data-table :global(tr.row > td.end) { display: block; grid-area: menu; }
    .data-table :global(tr.group-row) { display: block; }
    .data-table :global(tr.group-row td) { display: block; padding: var(--sp-100) var(--sp-200); }
    .verb,
    .menu-wide,
    .per-page { display: none; }
    .menu-phone { display: inline-flex; }
  }
</style>
