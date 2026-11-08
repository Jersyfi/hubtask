<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  import type { IconName } from './icons/index.ts';

  export interface View {
    readonly id: string;
    /** Resolved text: one or two words, *Board*, *Sprint 42*. */
    readonly label: string;
    readonly href: string;
    readonly icon?: IconName;
    /** A count worth seeing before opening it. */
    readonly count?: number;
  }
</script>

<script lang="ts">
  // The views of one object - a product's home, a hub tool, a collection - as one bar under its
  // head (design-system.md §4, wave 5; the navigation concept's §3). It switches routes, so it is
  // a navigation of links with `aria-current`, not `Tabs`: one tab stop, the arrows, Home and End
  // between the links. The first link is fixed; the others are dragged into order where the
  // reader may (`onreorder`); ＋ adds a view; tools stand at its end. There is never a second bar.
  // On compact it is a chip under the title that opens a `Sheet` with the same views in the same
  // order; a swipe changes no view.
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import NavItem from './NavItem.svelte';
  import Popover from './Popover.svelte';
  import Sheet from './Sheet.svelte';
  import { DRAG_THRESHOLD_PX, boundariesOf, dropIndex } from './reorder.ts';

  interface Props {
    /** What the bar is called, for the landmark: the object's name and "views". */
    label: string;
    views: readonly View[];
    current?: string;
    onnavigate?: (id: string) => void;
    /** ＋: a new view. Its words name it, "Add a view". */
    addLabel?: string;
    onadd?: () => void;
    /** Ordering for everyone (`STRUCTURE`): `to` is the position asked for; the first stays first. */
    onreorder?: (id: string, to: number) => void;
    /**
     * *Filter* and the like, at the end of the bar. Where the bar is too narrow for them and the
     * views, a tool with a mark shows only its mark - its words stay its name and become its
     * tooltip -, and narrower still they all move behind one ⋯ named `toolsLabel`.
     */
    tools?: Snippet;
    toolsLabel?: string;
    /**
     * A menu among the views, after the last (the inventory's *Manage ▾*): the caller's menu, its
     * trigger a plain button drawn as a view, `aria-current="page"` while one of its parts is open.
     */
    more?: Snippet;
    /** The compact drawing: the chip and its sheet. The width is the caller's to know. */
    isCompact?: boolean;
    /** The sheet's heading and its *Close*. */
    sheetTitle?: string;
    closeLabel?: string;
  }

  const { label, views, current, onnavigate, addLabel, onadd, onreorder, tools, toolsLabel, more, isCompact = false, sheetTitle, closeLabel }: Props = $props();

  let list = $state<HTMLUListElement | null>(null);
  let isSheetOpen = $state(false);
  const stop = $derived(views.some((view) => view.id === current) ? current : views[0]?.id);
  const shown = $derived(views.find((view) => view.id === current) ?? views[0]);

  // How the tools fit (the owner's review R-13): whole, as their marks, or behind ⋯ - never over
  // the views. Decided by measuring, because what fits depends on the words in both: whole while
  // the views and the tools fit side by side; as marks while the marks leave the views their room
  // (`layout.viewbar.views`), the views scrolling; behind ⋯ below that. The widths of both
  // drawings are measured on the element itself and kept, so the bar can tell from behind ⋯
  // when the marks would fit again.
  type Fit = 'whole' | 'marks' | 'menu';
  let bar = $state<HTMLElement | null>(null);
  let toolBox = $state<HTMLElement | null>(null);
  let fit = $state<Fit>('whole');
  const widths: { whole?: number; marks?: number } = {};

  function measure() {
    if (!bar || !list) return;
    const box = toolBox;
    if (box) {
      for (const each of ['whole', 'marks'] as const) {
        box.dataset.fit = each;
        widths[each] = box.getBoundingClientRect().width;
      }
      box.dataset.fit = fit;
    }
    if (widths.whole === undefined || widths.marks === undefined) return;
    const room = bar.clientWidth;
    const gap = Number.parseFloat(getComputedStyle(bar).columnGap) || 0;
    const kept = Number.parseFloat(getComputedStyle(bar).getPropertyValue('--layout-viewbar-views')) || 0;
    const wanted = list.scrollWidth;
    const next: Fit = wanted + gap + widths.whole <= room ? 'whole' : Math.min(wanted, kept) + gap + widths.marks <= room ? 'marks' : 'menu';
    if (next !== fit) fit = next;
    if (box) box.dataset.fit = next;
    named();
  }

  // A tool drawn as its mark keeps its words as its name; the words also become its tooltip.
  function named() {
    for (const button of toolBox?.querySelectorAll<HTMLElement>('button') ?? []) {
      const words = button.querySelector('.label')?.textContent?.trim();
      if (fit === 'marks' && words && !button.hasAttribute('aria-label')) {
        button.title = words;
        button.dataset.markTitle = '';
      } else if (button.dataset.markTitle !== undefined) {
        button.removeAttribute('title');
        delete button.dataset.markTitle;
      }
    }
  }

  $effect(() => {
    const element = bar;
    const box = toolBox;
    void views.length;
    if (!element || !tools || typeof ResizeObserver === 'undefined') return;
    // Answered in the next frame: a change of the bar within the observer's own delivery is left
    // undelivered, which WebKit throws (as `PageHeader` does).
    let frame = 0;
    const observer = new ResizeObserver(() => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(measure);
    });
    observer.observe(element);
    if (box) observer.observe(box);
    // A tool's words change - *Filter · 2* - without its mark's width changing: its tooltip follows.
    const words = new MutationObserver(named);
    if (box) words.observe(box, { subtree: true, childList: true, characterData: true });
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
      words.disconnect();
    };
  });

  $effect(() => {
    void current;
    list?.querySelector<HTMLElement>('[aria-current="page"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  });

  function walk(event: KeyboardEvent) {
    const links = [...(list?.querySelectorAll<HTMLElement>('a, .add') ?? [])];
    const at = links.indexOf(document.activeElement as HTMLElement);
    if (at < 0 || links.length === 0) return;
    const rtl = getComputedStyle(event.currentTarget as Element).direction === 'rtl';
    const forward = rtl ? 'ArrowLeft' : 'ArrowRight';
    const back = rtl ? 'ArrowRight' : 'ArrowLeft';
    let next = at;
    if (event.key === forward) next = (at + 1) % links.length;
    else if (event.key === back) next = (at - 1 + links.length) % links.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = links.length - 1;
    else return;
    event.preventDefault();
    links[next]?.focus();
  }

  function follow(event: MouseEvent, id: string) {
    if (moved) {
      event.preventDefault();
      moved = false;
      return;
    }
    if (!onnavigate || event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return;
    event.preventDefault();
    onnavigate(id);
  }

  // Dragging a view into place: the first is fixed, so neither the first link nor the slot before
  // it is ever a target.
  let carried = $state<{ id: string; from: number; x: number; to: number } | null>(null);
  let moved = false;
  let marker = $state<number | null>(null);

  function grab(event: PointerEvent, index: number) {
    if (!onreorder || index === 0 || event.button !== 0 || event.pointerType === 'touch') return;
    carried = { id: views[index]!.id, from: index, x: event.clientX, to: index };
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  }

  function drag(event: PointerEvent) {
    if (!carried || !list) return;
    if (marker === null && Math.abs(event.clientX - carried.x) < DRAG_THRESHOLD_PX) return;
    const items = [...list.querySelectorAll<HTMLElement>('li[data-view]')];
    const rtl = getComputedStyle(list).direction === 'rtl';
    const extents = items.map((item) => {
      const box = item.getBoundingClientRect();
      return rtl ? { start: -box.right, end: -box.left } : { start: box.left, end: box.right };
    });
    const along = rtl ? -event.clientX : event.clientX;
    const slot = Math.max(1, dropIndex(along, boundariesOf(extents)));
    carried.to = slot > carried.from ? slot - 1 : slot;
    // The marker stands at the slot's leading edge, measured physically like the pointer.
    const item = items[Math.min(slot, items.length - 1)]!;
    const isPastEnd = slot >= items.length;
    const west = item.offsetLeft;
    const east = item.offsetLeft + item.offsetWidth;
    marker = rtl ? (isPastEnd ? west : east) : isPastEnd ? east : west;
  }

  function drop() {
    if (!carried) return;
    const done = carried;
    const wasDragged = marker !== null;
    carried = null;
    marker = null;
    if (!wasDragged) return;
    moved = true;
    if (done.to !== done.from) onreorder?.(done.id, Math.max(1, done.to));
  }
</script>

{#if isCompact}
  <!-- The pill is small, its target a thumb's height (the navigation concept §9: 44 px). -->
  <button type="button" class="chip" aria-haspopup="dialog" aria-expanded={isSheetOpen} onclick={() => (isSheetOpen = true)}>
    <span class="pill">
      {#if shown?.icon}<Icon name={shown.icon} size="sm" />{/if}
      <span>{shown?.label}</span>
      <Icon name="chevron-down" size="sm" />
    </span>
  </button>
  {#if more}<span class="more">{@render more()}</span>{/if}
  <Sheet title={sheetTitle ?? label} closeLabel={closeLabel ?? ''} bind:isOpen={isSheetOpen}>
    <nav aria-label={label}>
      <ul class="sheet-list">
        {#each views as view (view.id)}
          <NavItem
            label={view.label}
            href={view.href}
            icon={view.icon}
            figure={view.count ? String(view.count) : undefined}
            isCurrent={view.id === current}
            size="touch"
            onnavigate={() => {
              isSheetOpen = false;
              onnavigate?.(view.id);
            }}
          />
        {/each}
        {#if onadd && addLabel}
          <NavItem
            label={addLabel}
            icon="plus"
            size="touch"
            onnavigate={() => {
              isSheetOpen = false;
              onadd();
            }}
          />
        {/if}
      </ul>
    </nav>
  </Sheet>
{:else}
  <nav class="bar" aria-label={label} bind:this={bar}>
    <!-- The keys move focus between the links of one bar; the links stay the controls. -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <ul bind:this={list} onkeydown={walk} data-dragging={marker !== null ? '' : undefined}>
      {#each views as view, index (view.id)}
        <li data-view={view.id} data-lifted={carried?.id === view.id && marker !== null ? '' : undefined}>
          <a
            href={view.href}
            aria-current={view.id === current ? 'page' : undefined}
            tabindex={view.id === stop ? 0 : -1}
            draggable="false"
            onclick={(event) => follow(event, view.id)}
            onpointerdown={(event) => grab(event, index)}
            onpointermove={drag}
            onpointerup={drop}
            onpointercancel={drop}
          >
            {#if view.icon}<Icon name={view.icon} size="sm" />{/if}
            <span class="word">{view.label}</span>
            {#if view.count}<span class="count">{view.count}</span>{/if}
          </a>
        </li>
      {/each}
      {#if more}<li class="more">{@render more()}</li>{/if}
      {#if onadd && addLabel}
        <li>
          <button type="button" class="add" tabindex="-1" aria-label={addLabel} title={addLabel} onclick={onadd}>
            <Icon name="plus" size="sm" />
          </button>
        </li>
      {/if}
      {#if marker !== null}
        <li class="marker" role="none" style:--marker={`${marker}px`}></li>
      {/if}
    </ul>
    {#if tools && fit === 'menu'}
      <span class="tools-more">
        <Popover label={toolsLabel ?? label} placement={{ side: 'block-end', align: 'end' }}>
          {#snippet trigger(props)}
            <IconButton icon="ellipsis" label={toolsLabel ?? label} size="sm" tone="secondary" {...props} />
          {/snippet}
          <div class="tools-menu">{@render tools()}</div>
        </Popover>
      </span>
    {:else if tools}
      <div class="tools" data-fit={fit} bind:this={toolBox}>{@render tools()}</div>
    {/if}
  </nav>
{/if}

<style>
  .bar {
    display: flex;
    align-items: center;
    gap: var(--sp-025);
    min-inline-size: 0;
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  ul {
    position: relative;
    display: flex;
    align-items: stretch;
    gap: var(--sp-025);
    min-inline-size: 0;
    margin: 0;
    padding: 0;
    list-style: none;
    overflow-x: auto;
    scrollbar-width: none;
  }

  li { flex: none; display: flex; }
  li[data-lifted] { opacity: 0.5; }

  /* A view is a control's height and a little more, in either density, so a bar with tools and one
     without stand equally tall. */
  a,
  .add,
  .more :global(button) {
    box-sizing: border-box;
    min-block-size: calc(var(--density-control-sm-min) + var(--sp-050) + var(--sp-025));
    display: inline-flex;
    align-items: center;
    gap: calc(var(--sp-050) + var(--sp-025));
    margin: 0 0 calc(-1 * var(--bw-hairline));
    padding: var(--sp-100) calc(var(--sp-100) + var(--sp-025));
    border: 0;
    border-block-end: var(--bw-ring) solid transparent;
    background: none;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-090);
    text-decoration: none;
    white-space: nowrap;
    cursor: pointer;
    transition: color var(--motion-state-duration) var(--motion-state-easing);
  }

  a :global(.icon),
  .add :global(.icon),
  .more :global(.icon),
  .chip :global(.icon) {
    inline-size: var(--layout-nav-actionglyph);
    block-size: var(--layout-nav-actionglyph);
  }

  a:hover,
  .add:hover,
  .more :global(button:hover) { color: var(--text-primary); }

  /* Rule 3: the colour, the weight and the bar under it - and `aria-current` without any of them. */
  a[aria-current='page'],
  .more :global(button[aria-current='page']) {
    color: var(--text-brand);
    font-weight: var(--fw-semibold);
    border-block-end-color: var(--accent-primary);
  }

  a:focus-visible,
  .add:focus-visible,
  .more :global(button:focus-visible) {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  ul[data-dragging] a { cursor: grabbing; }

  .marker {
    position: absolute;
    /* Placed from the physical corner: the marker's offset is measured that way. */
    inset: var(--sp-050) auto var(--sp-050) 0;
    inline-size: var(--bw-ring);
    background: var(--accent-primary);
    transform: translateX(calc(var(--marker) - var(--bw-hairline)));
    pointer-events: none;
  }

  .count {
    min-inline-size: var(--sp-250);
    padding: 0 var(--sp-050);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    text-align: center;
  }

  /* The tools keep their own width - the views give way, never the tools' words to a second line. */
  .tools,
  .tools-more {
    display: flex;
    flex: none;
    align-items: center;
    gap: calc(var(--sp-050) + var(--sp-025));
    margin-inline-start: auto;
    white-space: nowrap;
  }

  /* As marks: a tool with a mark keeps its words for its name, not drawn, and stands square. */
  .tools[data-fit='marks'] :global(.button:has(> .icon)) {
    position: relative;
    gap: 0;
    min-inline-size: var(--density-control-sm-min);
    padding-inline: 0;
  }

  .tools[data-fit='marks'] :global(.button:has(> .icon) > .label) {
    position: absolute;
    inline-size: var(--bw-hairline);
    block-size: var(--bw-hairline);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  /* Behind ⋯: the tools one under another, whole. */
  .tools-menu {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: var(--sp-050);
    padding: var(--sp-100);
  }

  .tools-menu :global(.button) { justify-content: flex-start; inline-size: 100%; }

  .chip {
    display: inline-flex;
    align-items: center;
    min-block-size: var(--layout-nav-touchrow);
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-primary);
    font: inherit;
    font-weight: var(--fw-medium);
    cursor: pointer;
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: calc(var(--sp-050) + var(--sp-025));
    padding: calc(var(--sp-050) + var(--sp-025)) calc(var(--sp-100) + var(--sp-025));
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-full);
    background: var(--bg-surface);
  }

  .chip:focus-visible { outline: none; }
  .chip:focus-visible .pill { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  /* The sheet's views: plain rows, top to bottom, in the bar's order (the sheet is the surface). */
  .sheet-list { display: block; }

  @media (prefers-reduced-motion: reduce) {
    a, .add { transition: none; }
  }

  :global([data-motion='reduced']) a,
  :global([data-motion='reduced']) .add { transition: none; }
</style>
