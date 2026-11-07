<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  import type { IconName } from './icons/index.ts';
  import type { RowAction, RowBadge } from './NavItem.svelte';
  import type { TreeNode } from './navtree.ts';

  /** A row of a group's tree: the drag's node plus what its `NavItem` draws. */
  export interface NavTreeNode extends TreeNode {
    readonly children?: readonly NavTreeNode[];
    readonly icon?: IconName;
    readonly color?: string;
    readonly figure?: string;
    readonly isFigureUrgent?: boolean;
    readonly figureLabel?: string;
    readonly badge?: RowBadge;
    readonly tone?: 'more' | 'link';
    /** Applied at once, not yet answered: the *waiting* mark's word (P-11). */
    readonly waitingLabel?: string;
  }

  /** A choice of the filter's kind: *All · Pages · Whiteboards*. `all` passes every kind. */
  export interface FilterKind {
    readonly id: string;
    readonly label: string;
  }
</script>

<script lang="ts">
  // A section of the sidebar (design-system.md §4, wave 5; the navigation concept's §2): a head
  // that sticks while its rows scroll under it, with a twist, the name and its count, and ＋ and
  // ⋯ on hover; then its rows - handed in as `NavItem`s, or as a tree it draws itself.
  //
  // As a tree it is ARIA's `tree` with one tab stop: the arrows move between the visible rows,
  // the row itself opens and closes, `Home` and `End` reach the ends. It bounds itself - *n more*
  // past `limit` top-level rows, a filter from `filterFrom` - and it carries the drag of §5.4:
  // the grip on hover, 4 px before it starts, the line at the target level or the row outlined
  // for *inside*, a folded branch opening under a hovering row, the region scrolling near its
  // edge, one sentence of reason on a refused target, `Escape` to cancel. The arithmetic is
  // `navtree.ts`; the write is the caller's (`onplace`).
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import Menu from './Menu.svelte';
  import NavItem from './NavItem.svelte';
  import { parseDuration } from './motion.ts';
  import {
    TREE_DRAG_THRESHOLD_PX,
    autoScrollStep,
    dropTarget,
    filterTree,
    indentOf,
    levelAtInline,
    rowsOf,
    zoneAt,
    type DropTarget,
    type MoveKey,
    type Placement,
    type Refusal,
  } from './navtree.ts';
  import { treeIntent } from './structure.ts';

  interface Props {
    /** Resolved text: *Collections*, *Contents*. */
    label: string;
    /** The section's count after its name: *Contents · 131*. */
    count?: string;
    /** Whether the section is open; bindable. A section without `collapseLabel` stays open. */
    isOpen?: boolean;
    /** What the head's twist is called: "Fold Collections". Absent, the head does not fold. */
    collapseLabel?: string;
    add?: RowAction;
    menu?: RowAction;
    /** Rows handed in (`NavItem`s), for a section that is a list rather than a tree. */
    children?: Snippet;

    /** The section as a tree. */
    tree?: readonly NavTreeNode[];
    /** What the tree is called for a screen reader: "Contents of Product development". */
    treeLabel?: string;
    /** The open branches, bindable. */
    expanded?: string[];
    current?: string;
    twistLabel?: (name: string, isExpanded: boolean) => string;
    rowMenu?: (id: string) => RowAction | undefined;
    rowAdd?: (id: string) => RowAction | undefined;
    /** Whether a row may be ordered here; absent, nothing drags (archived hub, a reader). */
    canOrder?: (id: string) => boolean;
    onnavigate?: (id: string) => void;
    /** A drop or a key asked for a placement (`navtree.ts`); the write is the caller's. */
    onplace?: (id: string, placement: Placement) => void;
    /** `Alt` and an arrow on a focused row, answered by the caller with `moveIntent`. */
    onmovekey?: (id: string, key: MoveKey) => void;
    onmoveto?: (id: string) => void;
    /** A drag ended by `Escape`: the caller announces *Cancelled. Nothing changed.* */
    oncancel?: () => void;
    /** The sentence on the drag image for a refused target. */
    refusalLabel?: (refusal: Refusal) => string;
    /** A row that just landed, marked for the highlight (the caller clears it). */
    landed?: string;

    /** Past this many top-level rows, *n more*. */
    limit?: number;
    /** The *n more* row's words, from how many are hidden. */
    moreLabel?: (hidden: number) => string;
    /** From this many rows (all levels) the filter field shows. */
    filterFrom?: number;
    filterLabel?: string;
    /** The words in the filter, bindable. */
    filter?: string;
    kinds?: readonly FilterKind[];
    /** The chosen kind, bindable. */
    kind?: string;
    /** What the kind's menu is called: "Show". */
    kindLabel?: string;
    /** On compact: the head as a quiet label over a card of touch rows (§9). */
    size?: 'md' | 'touch';
    /** On compact: a link at the head's end, *All*. */
    headLink?: { readonly label: string; readonly onselect: () => void };
  }

  let {
    label,
    count,
    isOpen = $bindable(true),
    collapseLabel,
    add,
    menu,
    children,
    tree,
    treeLabel,
    expanded = $bindable([]),
    current,
    twistLabel,
    rowMenu,
    rowAdd,
    canOrder,
    onnavigate,
    onplace,
    onmovekey,
    onmoveto,
    oncancel,
    refusalLabel,
    landed,
    limit,
    moreLabel,
    filterFrom,
    filterLabel,
    filter = $bindable(''),
    kinds,
    kind = $bindable('all'),
    kindLabel,
    size = 'md',
    headLink,
  }: Props = $props();

  const instanceId = $props.id();
  const headId = `navgroup-${instanceId}`;
  let isShowingAll = $state(false);
  let isKindOpen = $state(false);
  let isHeadMenuOpen = $state(false);
  let isHeadAddOpen = $state(false);

  function total(nodes: readonly NavTreeNode[]): number {
    return nodes.reduce((sum, node) => sum + 1 + total(node.children ?? []), 0);
  }

  const hasFilter = $derived(tree !== undefined && filterFrom !== undefined && total(tree) >= filterFrom);
  const filtered = $derived(hasFilter ? filterTree(tree ?? [], filter, kind) : { nodes: [...(tree ?? [])], open: [], count: -1 });
  const isFiltering = $derived(filtered.count >= 0);
  const bounded = $derived(
    !isFiltering && limit !== undefined && !isShowingAll && filtered.nodes.length > limit ? filtered.nodes.slice(0, limit) : filtered.nodes,
  );
  const hidden = $derived(filtered.nodes.length - bounded.length);
  const open = $derived(isFiltering ? [...new Set([...expanded, ...filtered.open])] : expanded);
  const rows = $derived(rowsOf(bounded, open));

  /** Each row's place among its siblings, for `aria-setsize` and `aria-posinset`. */
  const numbers = $derived.by(() => {
    const parents: number[] = [];
    const counts = new Map<number, number>();
    const at: { parent: number; at: number }[] = [];
    rows.forEach((row, index) => {
      parents.length = row.depth;
      const parent = row.depth === 0 ? -1 : (parents[row.depth - 1] ?? -1);
      const n = (counts.get(parent) ?? 0) + 1;
      counts.set(parent, n);
      at.push({ parent, at: n });
      parents[row.depth] = index;
    });
    return at.map((entry) => ({ at: entry.at, of: counts.get(entry.parent) ?? entry.at }));
  });

  let list = $state<HTMLUListElement | null>(null);
  let active = $state(-1);
  const stop = $derived(active >= 0 && active < rows.length ? active : Math.max(0, rows.findIndex((row) => row.node.id === current)));

  function toggle(id: string, isExpanded: boolean) {
    expanded = isExpanded ? [...expanded.filter((x) => x !== id), id] : expanded.filter((x) => x !== id);
  }

  function focusRow(index: number) {
    active = index;
    list?.querySelectorAll<HTMLElement>('[role="treeitem"]')[index]?.focus();
  }

  function walk(event: KeyboardEvent) {
    const at = [...(list?.querySelectorAll('[role="treeitem"]') ?? [])].indexOf(event.target as Element);
    if (at < 0 || event.altKey || event.ctrlKey || event.metaKey) return;
    if (drag && event.key === 'Escape') return;
    const dir = getComputedStyle(event.currentTarget as Element).direction === 'rtl' ? 'rtl' : 'ltr';
    const intent = treeIntent(event.key, rows, at, dir);
    if (!intent) return;
    event.preventDefault();
    if (intent.kind === 'focus') focusRow(intent.index);
    else toggle(rows[at]!.node.id, intent.kind === 'expand');
  }

  // --- the drag (§5.4) ------------------------------------------------------------------------

  interface Drag {
    readonly id: string;
    readonly node: NavTreeNode;
    readonly startX: number;
    readonly startY: number;
    isCarried: boolean;
    x: number;
    y: number;
    target: DropTarget;
  }

  let drag = $state<Drag | null>(null);
  let expandTimer: ReturnType<typeof setTimeout> | null = null;
  let expandFor: string | null = null;
  let scrollFrame = 0;

  function timing(name: string): number {
    return list ? parseDuration(getComputedStyle(list).getPropertyValue(name)) : 0;
  }

  function scroller(): HTMLElement | null {
    return list?.closest<HTMLElement>('[data-nav-scroll]') ?? null;
  }

  function begin(event: PointerEvent, node: NavTreeNode) {
    if (event.button !== 0 || event.pointerType === 'touch' || !onplace || !canOrder?.(node.id)) return;
    if ((event.target as Element).closest('.act, .twist')) return;
    drag = { id: node.id, node, startX: event.clientX, startY: event.clientY, isCarried: false, x: event.clientX, y: event.clientY, target: { kind: 'unchanged' } };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
    window.addEventListener('keydown', escape, true);
  }

  function measure(clientX: number, clientY: number): DropTarget {
    const items = [...(list?.querySelectorAll<HTMLElement>('[role="treeitem"]') ?? [])];
    const indent = list ? parseFloat(getComputedStyle(list).getPropertyValue('--layout-nav-indent')) || 0 : 0;
    const start = list?.getBoundingClientRect();
    for (let index = 0; index < items.length; index++) {
      const box = items[index]!.getBoundingClientRect();
      if (clientY < box.top || clientY >= box.bottom) continue;
      const row = rows[index]!;
      // A whiteboard's middle is *inside* so it can refuse with its reason; a collection's splits.
      const zone = zoneAt(clientY - box.top, box.height, (row.node.canHoldChildren ?? true) || row.node.kind === 'whiteboard');
      const rtl = list ? getComputedStyle(list).direction === 'rtl' : false;
      const inline = start ? (rtl ? start.right - clientX : clientX - start.left) : 0;
      return dropTarget(rows, drag!.node, index, zone, levelAtInline(inline, indent));
    }
    return { kind: 'unchanged' };
  }

  function move(event: PointerEvent) {
    if (!drag) return;
    drag.x = event.clientX;
    drag.y = event.clientY;
    if (!drag.isCarried) {
      if (Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) < TREE_DRAG_THRESHOLD_PX) return;
      drag.isCarried = true;
      autoScroll();
    }
    drag.target = measure(event.clientX, event.clientY);
    // Hovering a folded branch opens it after a while (§5.4).
    const over = drag.target.kind === 'inside' ? rows[drag.target.index] : undefined;
    const folded = over && over.isBranch && !over.isExpanded ? over.node.id : null;
    if (folded !== expandFor) {
      if (expandTimer) clearTimeout(expandTimer);
      expandTimer = null;
      expandFor = folded;
      if (folded) expandTimer = setTimeout(() => toggle(folded, true), timing('--dur-expand'));
    }
  }

  function autoScroll() {
    const region = scroller();
    if (!drag?.isCarried || !region) return;
    const box = region.getBoundingClientRect();
    const step = autoScrollStep(drag.y, box.top, box.bottom);
    if (step !== 0) {
      region.scrollTop += step;
      drag.target = measure(drag.x, drag.y);
    }
    scrollFrame = requestAnimationFrame(autoScroll);
  }

  function stopDrag() {
    window.removeEventListener('pointermove', move);
    window.removeEventListener('pointerup', end);
    window.removeEventListener('keydown', escape, true);
    cancelAnimationFrame(scrollFrame);
    if (expandTimer) clearTimeout(expandTimer);
    expandTimer = null;
    expandFor = null;
    drag = null;
  }

  function end() {
    const done = drag;
    stopDrag();
    if (!done?.isCarried) return;
    // A carried row is not a click on the row it was let go over.
    window.addEventListener('click', (event) => event.stopPropagation(), { capture: true, once: true });
    if (done.target.kind === 'line' || done.target.kind === 'inside') onplace?.(done.id, done.target.placement);
  }

  function escape(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !drag) return;
    event.preventDefault();
    event.stopPropagation();
    const wasCarried = drag.isCarried;
    stopDrag();
    if (wasCarried) oncancel?.();
  }

  $effect(() => () => stopDrag());

  /** Where the line stands: below the row it follows, at the target level's indent. */
  const line = $derived.by(() => {
    if (!drag?.isCarried || drag.target.kind !== 'line' || !list) return null;
    const items = list.querySelectorAll<HTMLElement>('[role="treeitem"]');
    const after = drag.target.afterIndex;
    const top = after >= 0 ? items[after]!.offsetTop + items[after]!.offsetHeight : items[0]?.offsetTop ?? 0;
    return { top, level: String(indentOf(drag.target.level)) };
  });

  const refusal = $derived(drag?.isCarried && drag.target.kind === 'refused' ? refusalLabel?.(drag.target.refusal) : undefined);
</script>

{#snippet headAction(kind: 'menu' | 'add', act: RowAction)}
  {#if act.items}
    <Menu
      label={act.label}
      items={act.items}
      placement={{ side: 'block-end', align: 'end' }}
      bind:isOpen={() => (kind === 'menu' ? isHeadMenuOpen : isHeadAddOpen), (o) => (kind === 'menu' ? (isHeadMenuOpen = o) : (isHeadAddOpen = o))}
      onselect={(itemId) => act.onselect(itemId)}
    >
      {#snippet trigger(props)}
        <button type="button" class="act" aria-label={act.label} title={act.label} {...props}>
          <Icon name={kind === 'menu' ? 'ellipsis' : 'plus'} size="sm" />
        </button>
      {/snippet}
    </Menu>
  {:else}
    <button type="button" class="act" aria-label={act.label} title={act.label} onclick={() => act.onselect()}>
      <Icon name={kind === 'menu' ? 'ellipsis' : 'plus'} size="sm" />
    </button>
  {/if}
{/snippet}

<section class="group" data-size={size} aria-labelledby={headId}>
  <div class="head" data-open={isHeadMenuOpen || isHeadAddOpen ? '' : undefined}>
    {#if collapseLabel}
      <button
        type="button"
        class="twist"
        aria-expanded={isOpen}
        aria-label={collapseLabel}
        title={collapseLabel}
        onclick={() => (isOpen = !isOpen)}
      >
        <Icon name={isOpen ? 'chevron-down' : 'chevron-right'} size="sm" />
      </button>
    {:else}
      <span class="twist" aria-hidden="true"><Icon name="chevron-down" size="sm" /></span>
    {/if}
    <h3 class="title" id={headId}>{label}{#if count}<span class="count">{` · ${count}`}</span>{/if}</h3>
    {#if headLink}
      <button type="button" class="head-link" onclick={headLink.onselect}>{headLink.label}</button>
    {/if}
    {#if add || menu}
      <span class="acts">
        {#if add}{@render headAction('add', add)}{/if}
        {#if menu}{@render headAction('menu', menu)}{/if}
      </span>
    {/if}
  </div>

  {#if isOpen}
    {#if hasFilter}
      <div class="filter">
        <Icon name="search" size="sm" />
        <input class="field" type="search" aria-label={filterLabel} placeholder={filterLabel} bind:value={filter} />
        {#if kinds && kinds.length > 0}
          <Menu
            label={kindLabel ?? ''}
            items={kinds.map((k) => ({ id: k.id, label: k.label }))}
            placement={{ side: 'block-end', align: 'end' }}
            bind:isOpen={isKindOpen}
            onselect={(id) => (kind = id)}
          >
            {#snippet trigger(props)}
              <button type="button" class="kind" aria-label={kindLabel} {...props}>
                {kinds.find((k) => k.id === kind)?.label ?? kinds[0]!.label}<span class="caret" aria-hidden="true">▾</span>
              </button>
            {/snippet}
          </Menu>
        {/if}
      </div>
    {/if}

    {#if tree}
      <!-- The keys move focus between the rows of one tree; the rows stay the controls. -->
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <ul
        class="rows"
        role="tree"
        aria-label={treeLabel ?? label}
        bind:this={list}
        onkeydown={walk}
        data-dragging={drag?.isCarried ? '' : undefined}
        data-refused={refusal ? '' : undefined}
      >
        {#each rows as row, index (row.node.id)}
          {@const node = row.node as NavTreeNode}
          <NavItem
            id={node.id}
            label={node.label}
            href={node.href}
            icon={node.icon}
            color={node.color}
            level={row.depth}
            twist={row.isBranch ? (row.isExpanded ? 'expanded' : 'collapsed') : undefined}
            twistLabel={twistLabel?.(node.label, row.isExpanded)}
            figure={node.figure}
            isFigureUrgent={node.isFigureUrgent}
            figureLabel={node.figureLabel}
            badge={node.badge}
            tone={node.tone}
            waitingLabel={node.waitingLabel}
            isCurrent={node.id === current}
            isMovable={!!onplace && !!canOrder?.(node.id)}
            menu={rowMenu?.(node.id)}
            add={rowAdd?.(node.id)}
            isTreeItem
            isTabStop={index === stop}
            setSize={numbers[index]?.of}
            position={numbers[index]?.at}
            dropState={drag?.isCarried && drag.id === node.id
              ? 'lifted'
              : drag?.isCarried && drag.target.kind === 'inside' && drag.target.index === index
                ? 'inside'
                : landed === node.id
                  ? 'landed'
                  : undefined}
            onnavigate={() => {
              active = index;
              onnavigate?.(node.id);
            }}
            ontoggle={(isExpanded) => {
              active = index;
              toggle(node.id, isExpanded);
            }}
            onmovekey={onmovekey ? (key) => onmovekey(node.id, key) : undefined}
            onmoveto={onmoveto ? () => onmoveto(node.id) : undefined}
            onpointerdown={(event) => begin(event, node)}
          />
        {/each}
        {#if line}
          <li class="line" role="none" data-level={line.level} style:--line-top={`${line.top}px`}></li>
        {/if}
      </ul>
      {#if hidden > 0 && moreLabel}
        <ul class="rows">
          <NavItem label={moreLabel(hidden)} icon="ellipsis" tone="more" onnavigate={() => (isShowingAll = true)} />
        </ul>
      {/if}
    {:else if children}
      <ul class="rows">{@render children()}</ul>
    {/if}
  {/if}
</section>

{#if drag?.isCarried}
  <!-- The drag image follows the pointer, fixed to the window so nothing cuts it; a refused
       target says why on it (§5.4). -->
  <div class="ghost" aria-hidden="true" style:--ghost-x={`${drag.x}px`} style:--ghost-y={`${drag.y}px`}>
    {#if drag.node.icon}<Icon name={drag.node.icon} size="sm" />{/if}
    <span class="ghost-name">{drag.node.label}</span>
    {#if refusal}<span class="why">{refusal}</span>{/if}
  </div>
{/if}

<style>
  .group { display: block; }

  /* The head sticks while its rows scroll under it, over the column's own surface. */
  .head {
    position: sticky;
    inset-block-start: calc(-1 * (var(--sp-050) + var(--sp-025)));
    z-index: var(--z-raised);
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-150) calc(var(--sp-050) + var(--sp-025)) var(--sp-050);
    background: var(--bg-surface);
    color: var(--text-subtle);
  }

  .title {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-065);
    font-weight: var(--fw-medium);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .twist {
    display: grid;
    place-items: center;
    flex: none;
    inline-size: var(--layout-nav-twist);
    block-size: var(--layout-nav-twist);
    margin: 0;
    padding: 0;
    border: 0;
    border-radius: var(--r-xs);
    background: none;
    color: inherit;
    cursor: pointer;
  }

  .twist :global(.icon) {
    inline-size: var(--layout-nav-twistglyph);
    block-size: var(--layout-nav-twistglyph);
  }

  .acts {
    display: flex;
    gap: var(--sp-025);
    margin-inline-start: auto;
    opacity: 0;
    transition: opacity var(--motion-state-duration) var(--motion-state-easing);
  }

  .head:hover .acts,
  .head:focus-within .acts,
  .head[data-open] .acts { opacity: 1; }

  .act {
    display: grid;
    place-items: center;
    inline-size: var(--layout-nav-action);
    block-size: var(--layout-nav-action);
    margin: 0;
    padding: 0;
    border: 0;
    border-radius: var(--r-xs);
    background: none;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .act:hover { background: var(--bg-surface-hover); }

  .act :global(.icon) {
    inline-size: var(--layout-nav-actionglyph);
    block-size: var(--layout-nav-actionglyph);
  }

  .act:focus-visible,
  .twist:focus-visible,
  .kind:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .filter {
    display: flex;
    align-items: center;
    gap: calc(var(--sp-050) + var(--sp-025));
    block-size: var(--layout-nav-filter);
    margin: var(--sp-050) var(--sp-025) calc(var(--sp-050) + var(--sp-025));
    padding: 0 var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--layout-nav-radius);
    background: var(--bg-canvas);
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .filter:focus-within {
    border-color: var(--focus-ring);
    box-shadow: inset 0 0 0 var(--bw-hairline) var(--focus-ring);
  }

  .filter :global(.icon) {
    inline-size: var(--layout-nav-tileglyph);
    block-size: var(--layout-nav-tileglyph);
  }

  .field {
    flex: 1;
    min-inline-size: 0;
    border: 0;
    outline: none;
    background: none;
    color: var(--text-primary);
    font: inherit;
  }

  .field::placeholder { color: var(--text-subtle); }

  .kind {
    flex: none;
    border: 0;
    border-radius: var(--r-xs);
    padding: 0 var(--sp-025);
    background: none;
    color: var(--text-subtle);
    font: inherit;
    cursor: pointer;
  }

  .caret { margin-inline-start: var(--sp-025); font-size: var(--fs-050); }

  .rows {
    position: relative;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  /* A press that travels carries the row; it never selects the names it passes. */
  .rows[role='tree'] { user-select: none; }
  .rows[data-dragging] { cursor: grabbing; }
  .rows[data-refused] { cursor: not-allowed; }

  /* The drop line: at the target level's indent, a ring at its start (§11.9). */
  .line {
    position: absolute;
    inset-block-start: calc(var(--line-top) - var(--bw-hairline));
    inset-inline: calc(var(--sp-050) + var(--sp-025) + var(--layout-nav-twist) + var(--line-indent, 0) * var(--layout-nav-indent)) 0;
    block-size: var(--bw-ring);
    border-radius: var(--bw-hairline);
    background: var(--accent-primary);
    pointer-events: none;
    list-style: none;
  }

  .line::before {
    content: '';
    position: absolute;
    inset-inline-start: calc(-1 * (var(--sp-050) + var(--sp-025)));
    inset-block-start: calc(-1 * var(--sp-050));
    inline-size: calc(var(--sp-050) + var(--sp-025));
    block-size: calc(var(--sp-050) + var(--sp-025));
    border: var(--bw-ring) solid var(--accent-primary);
    border-radius: var(--r-full);
    background: var(--bg-surface);
  }

  .line[data-level='1'] { --line-indent: 1; }
  .line[data-level='2'] { --line-indent: 2; }
  .line[data-level='3'] { --line-indent: 3; }
  .line[data-level='4'] { --line-indent: 4; }
  .line[data-level='5'] { --line-indent: 5; }
  .line[data-level='6'] { --line-indent: 6; }

  .ghost {
    position: fixed;
    /* The pointer's coordinates are physical, so the image is placed from the physical corner. */
    inset: 0 auto auto 0;
    z-index: var(--z-toast);
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-050) calc(var(--sp-100) + var(--sp-025));
    border: var(--bw-hairline) solid var(--accent-primary);
    border-radius: var(--r-md);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
    color: var(--text-primary);
    font-size: var(--fs-090);
    pointer-events: none;
    white-space: nowrap;
    transform: translate(calc(var(--ghost-x) + var(--sp-150)), calc(var(--ghost-y) + var(--sp-100)));
  }

  .why {
    margin-inline-start: var(--sp-050);
    padding: var(--bw-hairline) var(--sp-050);
    border-radius: var(--r-xs);
    background: var(--status-danger-surface);
    color: var(--status-danger-text);
    font-size: var(--fs-050);
  }

  /* Compact: a quiet label, then the rows as one card with hairlines between them. */
  [data-size='touch'] .head {
    position: static;
    justify-content: space-between;
    margin: var(--sp-150) var(--sp-050) calc(var(--sp-050) + var(--sp-025));
    padding: 0;
    background: none;
  }

  [data-size='touch'] .head > .twist { display: none; }

  /* The head's link a thumb's target (§9: 44 px), its words as small as the head's. */
  [data-size='touch'] .head-link {
    min-inline-size: var(--layout-nav-touchrow);
    min-block-size: var(--layout-nav-touchrow);
    margin-block: calc((var(--layout-nav-touchrow) - var(--fs-075)) / -2);
    text-align: end;
  }

  [data-size='touch'] .rows {
    margin-block-end: var(--sp-150);
    overflow: hidden;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .head-link {
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-brand);
    font: inherit;
    font-family: var(--font-ui);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    cursor: pointer;
  }

  .head-link:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  @media (prefers-reduced-motion: reduce) {
    .acts { transition: none; }
  }

  :global([data-motion='reduced']) .acts { transition: none; }
</style>
