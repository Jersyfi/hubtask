<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What narrows a list, on the canvas above it (page-system §6, PS-7): the search, the filters as
  // chips that each open their picker, *Clear*, and the count. Never a form open on arrival.
  //
  // Per width (§4.4): from `expanded` up every chip is inline; on `medium` the first two, the rest
  // behind *More filters*; on a phone the search fills the line, one *Filters (n)* button opens a
  // `Sheet` with every picker, and the count goes under the search so it stays in sight. The
  // pickers are the caller's - one snippet, handed the filter - because what a filter chooses from
  // is the screen's; where the filters live (the address) is the caller's too.

  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';
  import SearchField from './SearchField.svelte';
  import Sheet from './Sheet.svelte';

  export interface Filter {
    readonly id: string;
    /** The chip's words, its choice included: *Role: any*, *Last 7 days*. */
    readonly label: string;
    /** Whether it narrows the list now: drawn in the brand colour and counted. */
    readonly isActive?: boolean;
  }

  interface Props {
    /** The search field's name: *Search people*. */
    searchLabel: string;
    clearSearchLabel: string;
    search?: string;
    onsearch?: (value: string) => void;
    filters?: readonly Filter[];
    /** The picker of one filter, in its popover - or, on a phone, in the sheet. */
    picker?: Snippet<[Filter]>;
    /** *More filters*, from `medium` to `expanded`. */
    moreLabel?: string;
    /** *Filters (2)*: the phone's one button, with the number of active filters. */
    filtersLabel?: string;
    /** The sheet's heading. */
    sheetTitle?: string;
    sheetDismissLabel?: string;
    sheetGrabLabel?: string;
    /** The sheet's act that shows the result: *Show 9 people*. */
    showLabel?: string;
    /** *Clear*, shown while a filter is active. */
    clearLabel?: string;
    onclear?: () => void;
    /** *9 of 9*, *23 found*. */
    countLabel?: string;
  }

  let {
    searchLabel,
    clearSearchLabel,
    search = $bindable(''),
    onsearch,
    filters = [],
    picker,
    moreLabel,
    filtersLabel,
    sheetTitle,
    sheetDismissLabel,
    sheetGrabLabel,
    showLabel,
    clearLabel,
    onclear,
    countLabel,
  }: Props = $props();

  let isSheetOpen = $state(false);
  const hasActive = $derived(filters.some((filter) => filter.isActive));
</script>

<div class="filter-bar">
  <div class="search">
    <SearchField
      label={searchLabel}
      isLabelHidden
      clearLabel={clearSearchLabel}
      placeholder={searchLabel}
      bind:value={search}
      oninput={() => onsearch?.(search)}
      onclear={() => onsearch?.('')}
    />
  </div>

  {#each filters as filter, index (filter.id)}
    <span class="chip-slot" data-rank={index < 2 ? 'first' : 'rest'}>
      <Popover label={filter.label}>
        {#snippet trigger(props)}
          <button type="button" class="chip" data-active={filter.isActive ? '' : undefined} {...props}>
            <span>{filter.label}</span>
            <Icon name="chevron-down" size="sm" />
          </button>
        {/snippet}
        {@render picker?.(filter)}
      </Popover>
    </span>
  {/each}

  {#if filters.length > 2 && moreLabel}
    <span class="more">
      <button type="button" class="chip" onclick={() => (isSheetOpen = true)}>
        <Icon name="plus" size="sm" />
        <span>{moreLabel}</span>
      </button>
    </span>
  {/if}

  {#if hasActive && clearLabel}
    <span class="clear"><Button tone="subtle" size="sm" onclick={() => onclear?.()}>{clearLabel}</Button></span>
  {/if}

  {#if filters.length > 0 && filtersLabel}
    <span class="filters-button">
      <Button size="sm" icon="funnel" onclick={() => (isSheetOpen = true)}>{filtersLabel}</Button>
    </span>
  {/if}

  {#if countLabel}<span class="count" aria-live="polite">{countLabel}</span>{/if}
</div>

{#if filters.length > 0}
  <Sheet bind:isOpen={isSheetOpen} title={sheetTitle ?? filtersLabel ?? ''} closeLabel={sheetDismissLabel ?? ''} grabLabel={sheetGrabLabel ?? ''}>
    {#each filters as filter (filter.id)}
      <section class="sheet-filter">
        <h3 class="sheet-filter-name">{filter.label}</h3>
        {@render picker?.(filter)}
      </section>
    {/each}
    {#snippet actions()}
      {#if clearLabel}<Button tone="subtle" onclick={() => { onclear?.(); }}>{clearLabel}</Button>{/if}
      {#if showLabel}<Button tone="primary" onclick={() => (isSheetOpen = false)}>{showLabel}</Button>{/if}
    {/snippet}
  </Sheet>
{/if}

<style>
  .filter-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
    min-width: 0;
  }

  .search { inline-size: var(--layout-barsearch-side); max-inline-size: 100%; }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-sm-min);
    padding: var(--sp-025) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-100);
    white-space: nowrap;
    cursor: pointer;
    transition: background-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .chip:hover { background: var(--bg-surface-hover); }

  .chip[data-active] {
    background: var(--accent-primary-subtle);
    border-color: var(--accent-primary);
    color: var(--text-brand);
    font-weight: var(--fw-medium);
  }

  .chip:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .count { margin-inline-start: auto; color: var(--text-subtle); font-size: var(--fs-100); white-space: nowrap; }

  .more,
  .filters-button { display: none; }

  .sheet-filter { display: flex; flex-direction: column; gap: var(--sp-100); }
  .sheet-filter-name { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); color: var(--text-secondary); }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .chip-slot[data-rank='rest'] { display: none; }
    .more { display: inline-flex; }
  }

  /* On a phone: the search fills the line, one button opens the sheet, the count under them. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .search { flex: 1 1 0; inline-size: auto; min-inline-size: 0; }
    .chip-slot,
    .more,
    .clear { display: none; }
    .filters-button { display: inline-flex; }
    .count { inline-size: 100%; margin: 0; }
  }

  @media (prefers-reduced-motion: reduce) {
    .chip { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .chip { transition-duration: var(--dur-instant); }
</style>
