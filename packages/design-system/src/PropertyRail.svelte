<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script module lang="ts">
  import type { IconName } from './icons/index.ts';
  import type { ProductId } from './ProductTile.svelte';

  /**
   * A section of the rail. Products register theirs - service desk, time, affected items, links,
   * files, goal, code, retention - and the form's groups arrive as sections in the form's order. A
   * module switched off registers none, so it leaves no word (P-17).
   */
  export interface RailSection {
    readonly id: string;
    readonly title: string;
    /** The product's tile before the title, where the section is a product's. */
    readonly tile?: { icon: IconName; product?: ProductId };
    /** A short figure after the title: *1 breached*, *5*. */
    readonly count?: string;
    /** Nothing in it: drawn as one line with "+" (WB-3), opening it adds the first value. */
    readonly isEmpty?: boolean;
    /** The "+" line's words: *Add a goal*. */
    readonly addLabel?: string;
    /** Folded until opened: *more fields*, *code*. */
    readonly isFolded?: boolean;
  }
</script>

<script lang="ts">
  // The property rail (ET-1, WB-3): only properties and short tools, in sections a reader can fold;
  // an empty section is one line with "+", the custom fields nobody filled stay under *more
  // fields*. On a phone the rail is a sheet behind *All properties (n)*, with up to three
  // properties under the head (§7).
  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import ProductTile from './ProductTile.svelte';
  import Sheet from './Sheet.svelte';

  interface Props {
    /** The rail's name, an `aside`'s: *Properties*. */
    label: string;
    sections: readonly RailSection[];
    /** A section's content. */
    section: Snippet<[RailSection]>;
    /** Pressed "+" of an empty section. */
    onadd?: (id: string) => void;
    /** *Fold*, *Unfold*: the twist's names, with the section's title. */
    foldLabel: (title: string) => string;
    unfoldLabel: (title: string) => string;
    /** The phone's button: *All properties (18)*, and the up to three shown above it. */
    allLabel: string;
    /** The sheet's title: *All properties*, the button's words without the count. */
    sheetTitle?: string;
    summary?: Snippet;
    closeLabel: string;
    grabLabel?: string;
  }

  const { label, sections, section, onadd, foldLabel, unfoldLabel, allLabel, sheetTitle, summary, closeLabel, grabLabel }: Props = $props();

  const uid = $props.id();
  let unfolded = $state<Record<string, boolean>>({});
  let isSheetOpen = $state(false);

  const isOpen = (item: RailSection) => unfolded[item.id] ?? !item.isFolded;
  const toggle = (item: RailSection) => (unfolded = { ...unfolded, [item.id]: !isOpen(item) });
</script>

{#snippet sections_(where: string)}
  {#each sections as item (item.id)}
    {@const bodyId = `rail-${uid}-${where}-${item.id}`}
    <section class="section" data-empty={item.isEmpty ? '' : undefined} aria-labelledby={`${bodyId}-title`}>
      {#if item.isEmpty}
        <button type="button" class="add" onclick={() => onadd?.(item.id)}>
          {#if item.tile}<ProductTile icon={item.tile.icon} product={item.tile.product} size="sm" />{/if}
          <span class="title" id={`${bodyId}-title`}>{item.title}</span>
          <span class="add-label"><Icon name="plus" size="sm" />{item.addLabel ?? ''}</span>
        </button>
      {:else}
        <button
          type="button"
          class="head"
          aria-expanded={isOpen(item)}
          aria-controls={bodyId}
          aria-label={`${isOpen(item) ? foldLabel(item.title) : unfoldLabel(item.title)}${item.count ? `, ${item.count}` : ''}`}
          onclick={() => toggle(item)}
        >
          {#if item.tile}<ProductTile icon={item.tile.icon} product={item.tile.product} size="sm" />{/if}
          <span class="title" id={`${bodyId}-title`}>{item.title}</span>
          {#if item.count}<span class="count">{item.count}</span>{/if}
          <Icon name={isOpen(item) ? 'chevron-down' : 'chevron-right'} size="sm" />
        </button>
        {#if isOpen(item)}
          <div class="body" id={bodyId}>{@render section(item)}</div>
        {/if}
      {/if}
    </section>
  {/each}
{/snippet}

<aside class="rail" aria-label={label}>
  <div class="inline">{@render sections_('rail')}</div>
  <div class="phone">
    {#if summary}<div class="summary">{@render summary()}</div>{/if}
    <Button tone="secondary" icon="sliders-horizontal" isFull onclick={() => (isSheetOpen = true)}>{allLabel}</Button>
  </div>
</aside>

<Sheet bind:isOpen={isSheetOpen} title={sheetTitle ?? label} {closeLabel} {grabLabel}>
  {#if isSheetOpen}<div class="in-sheet">{@render sections_('sheet')}</div>{/if}
</Sheet>

<style>
  .rail { display: flex; flex-direction: column; min-inline-size: 0; }
  .inline { display: flex; flex-direction: column; }
  .phone { display: none; }

  .section { padding-block: var(--sp-150); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .section:first-child { padding-block-start: 0; }
  .section:last-child { border-block-end: 0; }

  .head,
  .add {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    inline-size: 100%;
    min-block-size: var(--sp-300);
    padding: 0;
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-subtle);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .head:hover,
  .add:hover { color: var(--text-secondary); }

  .head:focus-visible,
  .add:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .title {
    flex: 1;
    min-inline-size: 0;
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .count { color: var(--text-subtle); font-size: var(--fs-075); }

  .add-label {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-025);
    color: var(--text-brand);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
  }

  .body { display: flex; flex-direction: column; margin-block-start: var(--sp-100); }

  .summary { display: flex; flex-direction: column; margin-block-end: var(--sp-100); }

  .in-sheet { display: flex; flex-direction: column; }
  .in-sheet .head,
  .in-sheet .add { min-block-size: var(--sp-600); }

  /* A thumb's target (§7): a section's head and add are layout.target tall where the pointer is coarse. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .head,
    .add { min-block-size: var(--layout-target); }
  }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .inline { display: none; }
    .phone { display: flex; flex-direction: column; gap: var(--sp-100); }
  }
</style>
