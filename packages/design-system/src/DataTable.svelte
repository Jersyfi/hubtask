<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script module lang="ts">
  import type { Column } from './Table.svelte';
  import type { IconName } from './icons/index.ts';
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

  /**
   * A phone's swipe on a row (work-surface concept §7): towards the inline end (right in a
   * left-to-right script) takes the row's one act - *Complete* -, towards the start opens its menu.
   * Never the only way: the act also heads the row's menu (WCAG 2.5.7).
   */
  export interface RowSwipe<T> {
    readonly toEnd?: { readonly label: string; readonly icon: IconName; readonly onswipe: (row: T) => void };
    readonly isToStartMenu?: boolean;
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
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import LoadMore from './LoadMore.svelte';
  import Menu from './Menu.svelte';
  import Select from './Select.svelte';
  import Sheet from './Sheet.svelte';
  import Table from './Table.svelte';
  import type { Sort } from './table.ts';
  import type { ListDensity } from './density.ts';
  import { swipeIntent, swipeOutcome, SWIPE_MIN } from './swipe.ts';

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
    /** A group a row belongs to - a day, a status - drawn as a sunken row where it changes. */
    group?: (row: T) => string;
    /**
     * The group's head instead of its name alone: a status's lozenge (WB-4). The count of its rows
     * follows it, and the head becomes a twist that folds the group.
     */
    groupHead?: Snippet<[string]>;
    /** The twist's names, with the group's name. */
    foldGroupLabel?: (group: string) => string;
    unfoldGroupLabel?: (group: string) => string;
    /** The groups folded now; the caller keeps them. */
    foldedGroups?: string[];
    /** *+ Entry in "In progress"*: the last row of each group opens the capture there (C-4). */
    add?: { label: (group: string | undefined) => string; onadd: (group: string | undefined) => void };
    /** Comfortable or compact (WB-4); the caller keeps the choice on the device (`density.ts`). */
    density?: ListDensity;
    /**
     * `records` is the page system's list of things to configure; `work` a collection of entries or
     * tickets (WB-4): the head in small capitals, and the row act and ⋯ coming forward on hover
     * and focus only, on a device that hovers - a keyboard always finds them, a touch screen
     * always shows them.
     */
    kind?: 'records' | 'work';
    /**
     * The title's whole words, for a `work` row whose cell cuts them with an ellipsis: shown on
     * hover, and the cell opens to them while the row has focus.
     */
    titleText?: (row: T) => string;
    /** A phone's row under its title: the meta line and a bar - key, status, priority, SLA. */
    stack?: Snippet<[T]>;
    swipe?: RowSwipe<T>;
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
    groupHead,
    foldGroupLabel,
    unfoldGroupLabel,
    foldedGroups = $bindable([]),
    add,
    density,
    kind = 'records',
    titleText,
    stack,
    swipe,
    empty,
    shownLabel,
    more,
    perPage,
  }: Props = $props();

  const SELECT = '__select';
  const END = '__end';

  const visible = $derived(columns.filter((column) => !hiddenColumns.includes(column.id)));
  const hasActs = $derived(verb !== undefined || menu !== undefined || swipe?.toEnd !== undefined);
  // A work row keeps its act and ⋯ in the title's cell, after the title (the queue's mockup): no
  // column is held for them while nothing hovers.
  const hasEnd = $derived(hasActs && kind !== 'work');
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

  function toggle(id: string, isChecked: boolean) {
    selected = isChecked ? [...selected.filter((value) => value !== id), id] : selected.filter((value) => value !== id);
  }

  const uid = $props.id();
  const perPageId = `per-page-${uid}`;

  const perPageOptions = [25, 50, 100].map((value) => ({ value: String(value), label: String(value) }));

  const groupCounts = $derived.by(() => {
    const counts = new Map<string, number>();
    if (group) for (const row of rows) counts.set(group(row), (counts.get(group(row)) ?? 0) + 1);
    return counts;
  });
  const isFolded = (name: string) => foldedGroups.includes(name);
  const toggleGroup = (name: string) => (foldedGroups = isFolded(name) ? foldedGroups.filter((value) => value !== name) : [...foldedGroups, name]);

  /** The swipe in progress: which row, where it started, and what letting go would do. */
  let swiping = $state<{ id: string; x: number; y: number; width: number; isRtl: boolean; intent?: 'swipe' | 'scroll'; side?: 'end' | 'start'; isArmed: boolean } | null>(null);

  function swipeStart(event: PointerEvent, id: string) {
    if (!swipe || event.pointerType === 'mouse') return;
    const node = event.currentTarget as HTMLElement;
    const isRtl = getComputedStyle(node).direction === 'rtl';
    swiping = { id, x: event.clientX, y: event.clientY, width: node.getBoundingClientRect().width, isRtl, isArmed: false };
  }

  function swipeMove(event: PointerEvent) {
    if (!swiping || !swipe) return;
    const dx = event.clientX - swiping.x;
    const dy = event.clientY - swiping.y;
    const intent = swiping.intent ?? swipeIntent(dx, dy);
    if (intent === 'scroll') {
      swiping = null;
      return;
    }
    if (intent !== 'swipe') return;
    const side = dx > 0 !== swiping.isRtl ? 'end' : 'start';
    const isAllowed = side === 'end' ? swipe.toEnd !== undefined : swipe.isToStartMenu === true;
    swiping = { ...swiping, intent, side: isAllowed && Math.abs(dx) >= SWIPE_MIN / 2 ? side : undefined, isArmed: isAllowed && swipeOutcome(dx, swiping.width) !== 'none' };
  }

  function swipeEnd(row: T) {
    if (!swiping) return;
    const { side, isArmed } = swiping;
    swiping = null;
    if (!isArmed || !swipe) return;
    if (side === 'end') swipe.toEnd?.onswipe(row);
    else if (side === 'start') {
      sheetRow = row;
      isSheetOpen = true;
    }
  }

  /** The row's menu, with the swipe's act at its head - the way to it without a gesture. */
  const menuOf = (row: T): RowMenu | undefined => {
    const base = menu?.(row);
    if (!swipe?.toEnd) return base;
    const act: MenuItem = { id: '__swipe-end', label: swipe.toEnd.label, icon: swipe.toEnd.icon };
    return base ? { ...base, items: [act, ...base.items] } : { label: rowLabel?.(row) ?? '', items: [act] };
  };

  const sheetMenu = $derived(sheetRow !== null ? menuOf(sheetRow) : undefined);

  function onRowMenu(row: T, id: string) {
    if (id === '__swipe-end') swipe?.toEnd?.onswipe(row);
    else onmenu?.(row, id);
  }
</script>

<div class="data-table" data-selecting={isSelecting ? '' : undefined} data-density={density} data-kind={kind} data-stacked={stack ? '' : undefined}>
  <Table {label} isLabelHidden columns={tableColumns} {sort} {onSort} {isBusy} {empty} isHeadPinned foot={shownLabel || more || perPage ? tableFoot : undefined}>
    {#each rows as row, index (key(row))}
      {@const id = key(row)}
      {@const rowGroup = group?.(row)}
      {@const isGroupStart = rowGroup !== undefined && (index === 0 || group?.(rows[index - 1]!) !== rowGroup)}
      {@const isGroupEnd = index === rows.length - 1 || (rowGroup !== undefined && group?.(rows[index + 1]!) !== rowGroup)}
      {#if isGroupStart && rowGroup !== undefined}
        {#if groupHead}
          <tr class="group-row" data-head="">
            <td colspan={tableColumns.length}>
              <button
                type="button"
                class="twist"
                aria-expanded={!isFolded(rowGroup)}
                aria-label={isFolded(rowGroup) ? unfoldGroupLabel?.(rowGroup) : foldGroupLabel?.(rowGroup)}
                onclick={() => toggleGroup(rowGroup)}
              ><Icon name={isFolded(rowGroup) ? 'chevron-right' : 'chevron-down'} size="sm" /></button>
              {@render groupHead(rowGroup)}
              <span class="group-count">{groupCounts.get(rowGroup) ?? 0}</span>
            </td>
          </tr>
        {:else}
          <tr class="group-row"><td colspan={tableColumns.length}>{rowGroup}</td></tr>
        {/if}
      {/if}
      {@const link = href?.(row)}
      {#if rowGroup === undefined || !groupHead || !isFolded(rowGroup)}
      <tr
        class="row"
        data-selected={selected.includes(id) ? '' : undefined}
        data-pressable={link || onopen ? '' : undefined}
        data-swipe={swiping?.id === id && swiping.side ? swiping.side : undefined}
        data-armed={swiping?.id === id && swiping.isArmed ? '' : undefined}
        onpointerdown={swipe ? (event) => swipeStart(event, id) : undefined}
        onpointermove={swipe ? swipeMove : undefined}
        onpointerup={swipe ? () => swipeEnd(row) : undefined}
        onpointercancel={swipe ? () => (swiping = null) : undefined}
      >
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
            {#if column.id === titleColumn && kind === 'work'}
              <span class="title-line">
                {#if lead}<span class="lead">{@render lead(row)}</span>{/if}
                {#if link}
                  <a class="press cut" href={link} title={titleText?.(row)}>{@render cell(row, column)}</a>
                {:else if onopen}
                  <button type="button" class="press cut" title={titleText?.(row)} onclick={() => onopen(row)}>{@render cell(row, column)}</button>
                {:else}
                  <span class="cut" title={titleText?.(row)}>{@render cell(row, column)}</span>
                {/if}
                {#if hasActs}{@render acts(row)}{/if}
              </span>
            {:else}
            {#if column.id === titleColumn && lead}<span class="lead">{@render lead(row)}</span>{/if}
            {#if column.id === titleColumn && link}
              <a class="press" href={link}>{@render cell(row, column)}</a>
            {:else if column.id === titleColumn && onopen}
              <button type="button" class="press" onclick={() => onopen(row)}>{@render cell(row, column)}</button>
            {:else}
              {@render cell(row, column)}
            {/if}
            {/if}
          </td>
        {/each}
        {#if stack}
          <td class="stack" data-priority="1">
            {@render stack(row)}
            {#if swipe?.toEnd && swiping?.id === id && swiping.side}
              <span class="swipe-hint" aria-hidden="true">
                {#if swiping.side === 'end'}<Icon name={swipe.toEnd.icon} size="sm" />{swipe.toEnd.label}{:else}<Icon name="ellipsis" size="sm" />{/if}
              </span>
            {/if}
          </td>
        {/if}
        {#if hasEnd}
          <td class="end" data-priority="1" data-align="end">{@render acts(row)}</td>
        {/if}
      </tr>
      {/if}
      {#if add && isGroupEnd && (rowGroup === undefined || !groupHead || !isFolded(rowGroup))}
        <tr class="add-row">
          <td colspan={tableColumns.length}>
            <button type="button" class="add" title={add.label(rowGroup)} onclick={() => add.onadd(rowGroup)}><Icon name="plus" size="sm" /><span class="add-label">{add.label(rowGroup)}</span></button>
          </td>
        </tr>
      {/if}
    {/each}
  </Table>
</div>

{#snippet acts(row: T)}
  {@const rowMenu = menuOf(row)}
  <span class="end-inner">
    {#if verb}<span class="verb">{@render verb(row)}</span>{/if}
    {#if rowMenu && rowMenu.items.length > 0}
      {#if (menu?.(row)?.items.length ?? 0) > 0}
      <span class="menu-wide">
        <Menu label={rowMenu.label} items={menu?.(row)?.items ?? []} placement={{ side: 'block-end', align: 'end' }} onselect={(item) => onmenu?.(row, item)}>
          {#snippet trigger(props)}
            <IconButton icon="ellipsis" label={rowMenu.label} size="sm" {...props} />
          {/snippet}
        </Menu>
      </span>
      {/if}
      <span class="menu-phone">
        <IconButton icon="ellipsis" label={rowMenu.label} size="sm" onclick={() => { sheetRow = row; isSheetOpen = true; }} />
      </span>
    {/if}
  </span>
{/snippet}

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

{#if menu || swipe}
  <Sheet
    bind:isOpen={isSheetOpen}
    title={sheetRow !== null ? (rowLabel?.(sheetRow) ?? sheetMenu?.label ?? '') : ''}
    closeLabel={sheetDismissLabel}
    grabLabel={sheetGrabLabel}
    items={sheetMenu?.items ?? []}
    onselect={(item) => { if (sheetRow !== null) onRowMenu(sheetRow, item); }}
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

  /* The work surface's calm table (WB-4, the queue's mockup): rows as tall as the density says, the
     head in small capitals, groups as sunken heads with their count, "+ entry" as the group's
     last row. */
  .data-table[data-density] :global(tbody td) { block-size: var(--density-row-height); }

  .data-table[data-kind='work'] :global(thead th) {
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.06em;
    text-transform: uppercase;
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  /* A work row is one line at the density's height (WB-4): no cell wraps, the title takes the
     room that is left and is cut with an ellipsis, its whole words on hover and while the row
     has focus. The padding goes, so the height is the token's and not the content's. */
  .data-table[data-kind='work'] :global(tbody) { font-size: var(--fs-090); }
  .data-table[data-kind='work'] :global(tbody td) { padding-block: 0; white-space: nowrap; }
  .data-table[data-kind='work']:not([data-density]) :global(tbody tr.row > td) { block-size: var(--density-row-height); }
  .data-table[data-kind='work'] :global(td[data-role='title']) { inline-size: 100%; max-inline-size: 0; }
  .data-table[data-kind='work'] :global(td.select) { inline-size: 1%; padding-inline-end: 0; }

  .title-line { display: flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }
  .cut { display: block; flex: 0 1 auto; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .title-line .end-inner { margin-inline-start: auto; flex: none; }
  /* The act reads as one, not as a disabled control: the primary text on the hovered row. */
  .data-table[data-kind='work'] .verb :global(.button) { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .data-table[data-kind='work'] :global(tr.row:focus-within .cut) { white-space: normal; padding-block: var(--sp-050); }

  .data-table :global(tr.group-row[data-head] td) { padding-block: var(--sp-050); }
  .data-table[data-density] :global(tr.group-row td) { block-size: auto; }

  .twist {
    display: inline-grid;
    place-items: center;
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
    margin-inline-end: var(--sp-050);
    padding: 0;
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-subtle);
    vertical-align: middle;
    cursor: pointer;
  }

  .twist:hover { background: var(--bg-surface-hover); color: var(--text-secondary); }

  .twist:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .group-count { margin-inline-start: var(--sp-100); color: var(--text-subtle); font-weight: var(--fw-regular); }

  /* A long collection's name wraps, never widening the table past a phone's width. */
  .add {
    max-inline-size: 100%;
    display: inline-flex;
    align-items: center;
    gap: calc(var(--sp-050) + var(--sp-025));
    padding: var(--sp-025) var(--sp-050);
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-subtle);
    font: inherit;
    font-size: var(--fs-090);
    cursor: pointer;
  }

  .add-label { min-inline-size: 0; text-align: start; white-space: normal; }
  .add:hover { color: var(--text-secondary); background: var(--bg-surface-hover); }

  .add:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .data-table :global(tr.row:hover > td) { background: var(--bg-surface-hover); }

  @media (hover: hover) {
    .data-table[data-kind='work'] :global(.end-inner) { opacity: 0; transition: opacity var(--motion-state-duration) var(--motion-state-easing); }
    .data-table[data-kind='work'] :global(tr.row:hover .end-inner),
    .data-table[data-kind='work'] :global(tr.row:focus-within .end-inner),
    .data-table[data-kind='work'] :global(.end-inner:focus-within) { opacity: 1; }
    /* Seen only on hover, the title's actions lie over the title's end rather than keep a room of
       their own beside it, so the title has the whole column at rest. */
    .data-table[data-kind='work'] .title-line { position: relative; }
    .data-table[data-kind='work'] .title-line .end-inner {
      position: absolute;
      inset-inline-end: 0;
      padding-inline-start: var(--sp-100);
      background: var(--bg-surface-hover);
    }
  }

  .stack { display: none; }
  .swipe-hint { display: none; }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the checkbox's
     input, the ⋯ and the group's twist grow to layout.target; the drawn box stays as it is. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .data-table :global(td.select .native) { inset: calc((var(--sp-250) - var(--layout-target)) / 2); width: auto; height: auto; }
    .data-table :global(.end-inner .icon-button[data-size]) { width: var(--layout-target); height: var(--layout-target); }
    .twist { inline-size: var(--layout-target); block-size: var(--layout-target); margin-block: calc((var(--sp-300) - var(--layout-target)) / 2); }
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

    /* The stacked row (§7): the title, then the caller's meta line and bar, instead of the line
       cells; the row as tall as its content. */
    .data-table[data-stacked] :global(tr.row) {
      grid-template-areas: 'title status menu' 'stack stack menu';
      align-items: start;
      row-gap: var(--sp-050);
    }
    .data-table[data-stacked][data-selecting] :global(tr.row) {
      grid-template-areas: 'check title status menu' 'check stack stack menu';
    }
    .data-table[data-stacked] :global(tr.row > td[data-role='line']) { display: none; }

    /* A work row on a phone has no end column: its ⋯ stands in the title's line. The title wraps
       here - the row is as tall as its content - and the checkbox keeps its gap to it. */
    .data-table[data-kind='work'][data-stacked] :global(tr.row) {
      grid-template-columns: minmax(0, 1fr) auto;
      grid-template-areas: 'title status' 'stack stack';
    }
    .data-table[data-kind='work'][data-stacked][data-selecting] :global(tr.row) {
      grid-template-columns: auto minmax(0, 1fr) auto;
      grid-template-areas: 'check title status' 'check stack stack';
    }
    .data-table[data-kind='work'] :global(tr.row > td[data-role='title']) { inline-size: auto; max-inline-size: none; }
    .data-table[data-kind='work'] .cut { white-space: normal; }
    .data-table[data-kind='work'] :global(tr.row > td.select) { padding-block-start: var(--sp-025); }
    /* As specific as the work row's fixed height, which would otherwise hold each line at it. */
    .data-table[data-stacked] :global(tbody td),
    .data-table[data-kind][data-stacked] :global(tbody tr.row > td) { block-size: auto; }
    .data-table :global(tr.row > td.stack) { display: flex; flex-direction: column; gap: var(--sp-050); grid-area: stack; min-inline-size: 0; }
    /* The row is a thumb's target (§7): the title's link or open button reaches over the whole
       row, and every other control in it stands above that reach. */
    .data-table :global(tr.row) { position: relative; }
    .data-table[data-kind='work'] .title-line { position: static; }
    .data-table :global(tr.row > td[data-role='title'] :is(a, .press):first-of-type::after) { content: ''; position: absolute; inset: 0; }
    .data-table :global(tr.row > td:not([data-role='title']) :is(a, button, input, select, [role='button'])),
    .data-table :global(tr.row > td[data-role='title'] :is(.lead, .end-inner) :is(a, button, input)) { position: relative; z-index: var(--z-raised); }
    .data-table :global(tr.row > td.select),
    .data-table :global(tr.row > td.end) { position: relative; z-index: var(--z-raised); }
    .data-table :global(tr.add-row) { display: block; }
    .data-table :global(tr.add-row td) { display: block; padding: var(--sp-100) var(--sp-200); }
    .add { min-block-size: var(--layout-nav-touchrow); }

    /* A swipe moves the row by the act's width, and armed by more: the row says "let go now"
       without following the thumb pixel by pixel, which a stylesheet cannot. */
    .data-table :global(tr.row) { transition: transform var(--motion-state-duration) var(--motion-state-easing); }
    .data-table :global(tr.row) { --swipe-sign: 1; }
    .data-table :global(tr.row:dir(rtl)) { --swipe-sign: -1; }
    .data-table :global(tr.row[data-swipe='end']) { transform: translateX(calc(var(--swipe-sign) * var(--sp-400))); }
    .data-table :global(tr.row[data-swipe='start']) { transform: translateX(calc(-1 * var(--swipe-sign) * var(--sp-400))); }
    .data-table :global(tr.row[data-swipe='end'][data-armed]) { transform: translateX(calc(var(--swipe-sign) * var(--sp-1000))); background: var(--accent-primary-subtle); }
    .data-table :global(tr.row[data-swipe='start'][data-armed]) { transform: translateX(calc(-1 * var(--swipe-sign) * var(--sp-1000))); background: var(--bg-surface-hover); }
    .data-table :global(tr.row[data-swipe] .swipe-hint) { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-brand); font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  }

  @media (prefers-reduced-motion: reduce) {
    .data-table :global(tr.row),
    .end-inner { transition: none; }
  }

  :global([data-motion='reduced']) .data-table :global(tr.row),
  :global([data-motion='reduced']) .end-inner { transition: none; }
</style>
