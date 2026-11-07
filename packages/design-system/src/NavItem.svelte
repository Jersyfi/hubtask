<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  import type { IconName } from './icons/index.ts';
  import type { MenuItem } from './overlay.ts';

  /** A row's ⋯ or ＋: a menu where it offers a choice, a plain press where it does one thing. */
  export interface RowAction {
    /** Resolved text: "Manage Web app", "Create under Team". */
    readonly label: string;
    readonly items?: readonly MenuItem[];
    /** Resolved text above the items: "Create under “Team”". The menu then opens beside the row. */
    readonly heading?: string;
    readonly onselect: (itemId?: string) => void;
  }

  /** A mark after the name that says what kind of place it is: the sprint mark, *Private*. */
  export interface RowBadge {
    readonly icon: IconName;
    readonly label: string;
  }

  /** How the row stands in a drag (design-system.md §11.9): carried, a target, just landed. */
  export type DropState = 'lifted' | 'inside' | 'landed';
</script>

<script lang="ts">
  // One row of the sidebar (design-system.md §4, wave 5; §11.3; the navigation concept's §2.6):
  // a twist, a mark or a tile, the name, a badge, the figure - and ⋯ and ＋ in the figure's cell
  // on hover and focus. The current row carries the brand bar, the selection surface and
  // `aria-current`. Presentational: the caller says where it leads and what its actions do.
  //
  // Two shapes. Inside a `NavGroup` tree it is the `treeitem` itself, one tab stop for the tree
  // (the group roves); elsewhere it is a list item holding a link. The twist, ⋯ and ＋ stay out
  // of the tab order in both: the row keeps its one stop, the arrows open and close, and the
  // context-menu key or `Shift+F10` opens ⋯ (§11.3).
  import Icon from './Icon.svelte';
  import Menu from './Menu.svelte';
  import ProductTile from './ProductTile.svelte';
  import { indentOf, type MoveKey } from './navtree.ts';

  interface Props {
    /** Resolved text: the place's name. */
    label: string;
    href?: string;
    /** A plain mark, drawn in the secondary ink. */
    icon?: IconName;
    /** A label colour: the mark is drawn on its tile (`ProductTile`). */
    color?: string;
    /** The tree level, 0 at a part's top; the indent stops growing at the sixth. */
    level?: number;
    /** Where the row opens: `expanded` or `collapsed`; absent is a leaf, its twist's room kept. */
    twist?: 'expanded' | 'collapsed';
    /** What the twist is called, from the name and its state: "Open Team". */
    twistLabel?: string;
    /** The figure: an amount, quiet; resolved text ("Sprint 42 · 6 d", "14"). */
    figure?: string;
    /** The figure asks for action - SLA, overdue, at risk - and stands on the danger surface. */
    isFigureUrgent?: boolean;
    /** Heard with the figure where the figure alone would not say it: "14 open". */
    figureLabel?: string;
    badge?: RowBadge;
    isCurrent?: boolean;
    /** Ordering is allowed here: the grip appears on hover (§5.4). */
    isMovable?: boolean;
    menu?: RowAction;
    add?: RowAction;
    /** A quieter row: *n more*, or a link-coloured one such as *All contents*. */
    tone?: 'more' | 'link';
    /** Folded to the rail: the mark alone, the name its tooltip and accessible name. */
    isRail?: boolean;
    /** Inside a `NavGroup` tree: the row is the `treeitem`. */
    isTreeItem?: boolean;
    /** The tree's one tab stop (the group roves it). */
    isTabStop?: boolean;
    /** Applied at once, the server not answered yet (P-11): the *waiting* mark, with its word. */
    waitingLabel?: string;
    dropState?: DropState;
    /** For the tree: the row's id, so the group finds it again. */
    id?: string;
    /** The tree's ARIA numbers (`aria-setsize`, `aria-posinset`). */
    setSize?: number;
    position?: number;
    /** The compact drawing: a 44 px row, a separate › to descend, reorder grips at the end. */
    size?: 'md' | 'touch';
    /** On compact, the separate control that descends to the children, named "Open Team's pages". */
    descendLabel?: string;
    ondescend?: () => void;
    /** Reorder mode on compact (§9): the grips stand at the row's end. */
    isReordering?: boolean;
    /** On compact, the row leads one stack level down: a › at its end. */
    isDrillDown?: boolean;
    onnavigate?: () => void;
    ontoggle?: (isExpanded: boolean) => void;
    /** `Alt` with an arrow on the focused row: the caller answers it with `moveIntent`. */
    onmovekey?: (key: MoveKey) => void;
    /** `M` on the focused row: *Move to…* (§2.7). */
    onmoveto?: () => void;
    onpointerdown?: (event: PointerEvent) => void;
  }

  const {
    label,
    href,
    icon,
    color,
    level = 0,
    twist,
    twistLabel,
    figure,
    isFigureUrgent = false,
    figureLabel,
    badge,
    isCurrent = false,
    isMovable = false,
    menu,
    add,
    tone,
    isRail = false,
    isTreeItem = false,
    isTabStop = false,
    waitingLabel,
    dropState,
    id,
    setSize,
    position,
    size = 'md',
    descendLabel,
    ondescend,
    isReordering = false,
    isDrillDown = false,
    onnavigate,
    ontoggle,
    onmovekey,
    onmoveto,
    onpointerdown,
  }: Props = $props();

  let isMenuOpen = $state(false);
  let isAddOpen = $state(false);

  const indent = $derived(String(indentOf(level)));

  function follow(event: MouseEvent) {
    if (!onnavigate || event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return;
    event.preventDefault();
    onnavigate();
  }

  function keys(event: KeyboardEvent) {
    if (event.target !== event.currentTarget) return;
    if ((event.key === 'ContextMenu' || (event.key === 'F10' && event.shiftKey)) && menu) {
      event.preventDefault();
      if (menu.items) isMenuOpen = true;
      else menu.onselect();
      return;
    }
    if (event.altKey && event.key.startsWith('Arrow') && onmovekey) {
      event.preventDefault();
      event.stopPropagation();
      onmovekey({ key: event.key, altKey: true, ctrlKey: event.ctrlKey, metaKey: event.metaKey });
      return;
    }
    if ((event.key === 'm' || event.key === 'M') && !event.altKey && !event.ctrlKey && !event.metaKey && onmoveto) {
      event.preventDefault();
      onmoveto();
      return;
    }
    if (!isTreeItem) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onnavigate?.();
      return;
    }
    // Opening and closing is the row's own; moving between rows bubbles to the group.
    const rtl = getComputedStyle(event.currentTarget as Element).direction === 'rtl';
    const open = rtl ? 'ArrowLeft' : 'ArrowRight';
    const close = rtl ? 'ArrowRight' : 'ArrowLeft';
    if (event.key === open && twist === 'collapsed') {
      event.preventDefault();
      event.stopPropagation();
      ontoggle?.(true);
    } else if (event.key === close && twist === 'expanded') {
      event.preventDefault();
      event.stopPropagation();
      ontoggle?.(false);
    }
  }
</script>

{#snippet menuHeading()}<span class="menu-heading" data-menu-label>{menu?.heading}</span>{/snippet}
{#snippet addHeading()}<span class="menu-heading" data-menu-label>{add?.heading}</span>{/snippet}

{#snippet action(kind: 'menu' | 'add', act: RowAction)}
  {#if act.items}
    <Menu
      label={act.label}
      items={act.items}
      placement={act.heading ? { side: 'inline-end', align: 'start' } : { side: 'block-end', align: 'end' }}
      bind:isOpen={() => (kind === 'menu' ? isMenuOpen : isAddOpen), (open) => (kind === 'menu' ? (isMenuOpen = open) : (isAddOpen = open))}
      onselect={(itemId) => act.onselect(itemId)}
      head={act.heading ? (kind === 'menu' ? menuHeading : addHeading) : undefined}
    >
      {#snippet trigger(props)}
        <button type="button" class="act" tabindex="-1" aria-label={act.label} title={act.label} {...props}>
          <Icon name={kind === 'menu' ? 'ellipsis' : 'plus'} size="sm" />
        </button>
      {/snippet}
    </Menu>
  {:else}
    <button type="button" class="act" tabindex="-1" aria-label={act.label} title={act.label} onclick={() => act.onselect()}>
      <Icon name={kind === 'menu' ? 'ellipsis' : 'plus'} size="sm" />
    </button>
  {/if}
{/snippet}

{#snippet body()}
  {#if isMovable && !isRail && size === 'md'}
    <span class="grip" aria-hidden="true"><Icon name="grip-vertical" size="sm" /></span>
  {/if}
  {#if !isRail && size === 'md'}
    {#if twist}
      <button
        type="button"
        class="twist"
        tabindex="-1"
        aria-label={twistLabel}
        title={twistLabel}
        aria-hidden={twistLabel ? undefined : 'true'}
        onclick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          ontoggle?.(twist === 'collapsed');
        }}
      >
        <Icon name={twist === 'expanded' ? 'chevron-down' : 'chevron-right'} size="sm" />
      </button>
    {:else}
      <span class="twist" aria-hidden="true"></span>
    {/if}
  {/if}
  <span class="mark" aria-hidden="true">
    {#if icon && color}<ProductTile {icon} {color} />{:else if icon}<Icon name={icon} size="sm" />{/if}
  </span>
  {#if !isRail}
    <span class="name" title={label}>{label}</span>
    {#if badge}
      <span class="badge" title={badge.label}><Icon name={badge.icon} label={badge.label} size="sm" /></span>
    {/if}
    {#if waitingLabel}
      <span class="badge waiting" title={waitingLabel}><Icon name="cloud-upload" label={waitingLabel} size="sm" /></span>
    {/if}
    {#if figure || menu || add}
      <!-- The figure and the actions share one cell: hovered or focused, the actions take the
           figure's place, so a row with actions wraps where one without does (§11.3). -->
      <span class="end" data-open={isMenuOpen || isAddOpen ? '' : undefined}>
        {#if figure}
          <span class="figure" data-urgent={isFigureUrgent ? '' : undefined} aria-label={figureLabel}>{figure}</span>
        {/if}
        {#if (menu || add) && size === 'md'}
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
          <span class="acts" onclick={(event) => event.stopPropagation()}>
            {#if menu}{@render action('menu', menu)}{/if}
            {#if add}{@render action('add', add)}{/if}
          </span>
        {/if}
      </span>
    {/if}
    {#if size === 'touch' && isReordering}
      <span class="reorder" aria-hidden="true"><Icon name="grip-vertical" size="sm" /></span>
    {:else if size === 'touch' && isDrillDown}
      <span class="chevron" aria-hidden="true"><Icon name="chevron-right" size="sm" /></span>
    {/if}
  {/if}
{/snippet}

{#if isTreeItem}
  <li
    class="row"
    role="treeitem"
    data-size={size}
    data-tone={tone}
    data-level={indent}
    data-drop={dropState}
    data-movable={isMovable ? '' : undefined}
    data-rail={isRail ? '' : undefined}
    data-id={id}
    aria-level={level + 1}
    aria-setsize={setSize}
    aria-posinset={position}
    aria-expanded={twist ? twist === 'expanded' : undefined}
    aria-selected={isCurrent}
    aria-current={isCurrent ? 'page' : undefined}
    aria-label={isRail ? label : undefined}
    aria-keyshortcuts={isMovable ? 'Alt+ArrowUp Alt+ArrowDown' : undefined}
    tabindex={isTabStop ? 0 : -1}
    onclick={() => onnavigate?.()}
    onkeydown={keys}
    {onpointerdown}
  >
    {@render body()}
  </li>
{:else}
  <li
    class="row"
    data-size={size}
    data-tone={tone}
    data-level={indent}
    data-drop={dropState}
    data-movable={isMovable ? '' : undefined}
    data-rail={isRail ? '' : undefined}
    data-current={isCurrent ? '' : undefined}
    data-id={id}
    {onpointerdown}
  >
    <!-- An anchor or a button: both are interactive, the linter only cannot tell from the tag. -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <svelte:element
      this={href ? 'a' : 'button'}
      class="link"
      type={href ? undefined : 'button'}
      {href}
      aria-current={isCurrent ? 'page' : undefined}
      aria-label={isRail ? label : undefined}
      title={isRail ? label : undefined}
      onclick={follow}
      onkeydown={keys}
    >
      {@render body()}
    </svelte:element>
    {#if size === 'touch' && ondescend}
      <button type="button" class="descend" aria-label={descendLabel} title={descendLabel} onclick={ondescend}>
        <Icon name="chevron-right" size="sm" />
      </button>
    {/if}
  </li>
{/if}

<style>
  .row {
    position: relative;
    display: flex;
    align-items: center;
    list-style: none;
    min-block-size: var(--layout-nav-row);
    border-radius: var(--layout-nav-radius);
    color: var(--text-primary);
    font-size: var(--fs-090);
    cursor: pointer;
    transition: background-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .row[role='treeitem'],
  .link {
    flex: 1;
    min-inline-size: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    block-size: var(--layout-nav-row);
    padding-block: 0;
    padding-inline: calc(var(--sp-050) + var(--sp-025) + var(--indent, 0) * var(--layout-nav-indent)) calc(var(--sp-050) + var(--sp-025));
    white-space: nowrap;
  }

  /* The gap between the twist and the mark, and between the mark and the name, is the mockup's
     single step less a hairline: the cells already carry their own room. */
  .row[role='treeitem'], .link { gap: calc(var(--sp-100) - var(--bw-hairline)); }

  .link {
    margin: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: start;
    text-decoration: none;
    border-radius: inherit;
    cursor: pointer;
  }

  .row[data-level='1'] { --indent: 1; }
  .row[data-level='2'] { --indent: 2; }
  .row[data-level='3'] { --indent: 3; }
  .row[data-level='4'] { --indent: 4; }
  .row[data-level='5'] { --indent: 5; }
  .row[data-level='6'] { --indent: 6; }

  .row:hover { background: var(--bg-surface-hover); }

  /* A menu's heading, as the sidebar's section heads are set (the mockup's *Create under “…”*). */
  .menu-heading {
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-065);
    font-weight: var(--fw-medium);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
    white-space: nowrap;
  }

  .row[aria-current='page'],
  .row[data-current] {
    background: var(--accent-primary-subtle);
    font-weight: var(--fw-semibold);
  }

  /* The brand bar stands in the column's padding, outside the row, as the mockup draws it. */
  .row[aria-current='page']::before,
  .row[data-current]::before {
    content: '';
    position: absolute;
    inset-inline-start: calc(-1 * var(--sp-100));
    inset-block: var(--sp-050) var(--sp-050);
    inline-size: var(--layout-nav-bar);
    border-start-end-radius: var(--layout-nav-bar);
    border-end-end-radius: var(--layout-nav-bar);
    background: var(--accent-primary);
  }

  .row[role='treeitem']:focus-visible,
  .link:focus-visible,
  .act:focus-visible,
  .twist:focus-visible,
  .descend:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .row[data-tone='more'] { color: var(--text-secondary); }
  .row[data-tone='more'] .name { font-size: var(--fs-075); }
  .row[data-tone='link'] { color: var(--text-brand); }

  .grip {
    position: absolute;
    inset-inline-start: calc(-1 * (var(--sp-050) + var(--sp-025) + var(--bw-hairline)));
    display: grid;
    place-items: center;
    color: var(--text-subtle);
    opacity: 0;
    cursor: grab;
    transition: opacity var(--motion-state-duration) var(--motion-state-easing);
  }

  .row[data-movable]:hover .grip { opacity: 1; }

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
    color: var(--text-subtle);
    cursor: pointer;
  }

  .grip :global(.icon),
  .twist :global(.icon) {
    inline-size: var(--layout-nav-twistglyph);
    block-size: var(--layout-nav-twistglyph);
  }

  .mark {
    display: grid;
    place-items: center;
    flex: none;
    inline-size: var(--layout-nav-mark);
    color: var(--text-secondary);
  }

  .mark > :global(.icon),
  .reorder :global(.icon),
  .chevron :global(.icon),
  .descend :global(.icon) {
    inline-size: var(--layout-nav-glyph);
    block-size: var(--layout-nav-glyph);
    /* design-system-lint-ignore */
    stroke-width: 2;
  }

  .name {
    flex: 1;
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .badge {
    display: grid;
    place-items: center;
    flex: none;
    color: var(--text-subtle);
  }

  .badge :global(.icon) {
    inline-size: var(--layout-nav-actionglyph);
    block-size: var(--layout-nav-actionglyph);
  }

  .waiting { color: var(--text-secondary); }

  .end {
    display: flex;
    align-items: center;
    flex: none;
    margin-inline-start: auto;
  }

  .figure {
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    font-weight: var(--fw-regular);
    color: var(--text-subtle);
  }

  .figure[data-urgent] {
    padding: var(--bw-hairline) calc(var(--sp-050) + var(--sp-025));
    border-radius: var(--r-full);
    background: var(--status-danger-surface);
    color: var(--status-danger-text);
  }

  .acts {
    display: none;
    gap: var(--sp-025);
  }

  /* Hovered, focused, or with its menu open: the actions take the figure's place. */
  .row:hover .acts,
  .row:focus-within .acts,
  .end[data-open] .acts { display: flex; }

  .row:hover .end:has(.acts) .figure,
  .row:focus-within .end:has(.acts) .figure,
  .end[data-open] .figure { display: none; }

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

  .act:hover { background: var(--border-subtle); }

  .act :global(.icon) {
    inline-size: var(--layout-nav-actionglyph);
    block-size: var(--layout-nav-actionglyph);
  }

  /* The rail: one mark per row, centred; the name is the tooltip and the accessible name. */
  .row[data-rail] .link,
  .row[data-rail][role='treeitem'] {
    justify-content: center;
    padding: 0;
  }

  /* A drag (design-system.md §11.9): the carried row stays faded where it was, a target for
     *inside* is outlined, a row that just landed is marked and fades back. */
  .row[data-drop='lifted'] { opacity: 0.35; }

  .row[data-drop='inside'] {
    background: var(--accent-primary-subtle);
    box-shadow: inset 0 0 0 var(--bw-ring) var(--accent-primary);
  }

  .row[data-drop='landed'] { animation: landed var(--motion-highlight-duration) var(--motion-highlight-easing) var(--motion-settle-duration) both; }

  @keyframes landed {
    from { background-color: var(--accent-primary-subtle); }
    to { background-color: transparent; }
  }

  /* Compact: a thumb's row inside a grouped card, a hairline between rows. */
  .row[data-size='touch'] {
    min-block-size: var(--layout-nav-touchrow);
    border-radius: 0;
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    font-size: var(--fs-100);
  }

  .row[data-size='touch']:last-child { border-block-end: 0; }

  .row[data-size='touch'] .link,
  .row[data-size='touch'][role='treeitem'] {
    block-size: var(--layout-nav-touchrow);
    gap: calc(var(--sp-100) + var(--sp-025));
    padding-inline: var(--sp-150);
  }

  .row[data-size='touch'] .acts { display: none; }
  .row[data-size='touch'] .figure { display: inline; }

  .reorder,
  .chevron {
    display: grid;
    place-items: center;
    margin-inline-start: auto;
    color: var(--text-subtle);
  }

  .end + .chevron,
  .end + .reorder { margin-inline-start: 0; }

  .descend {
    display: grid;
    place-items: center;
    flex: none;
    /* A thumb's square (§9: 44 px), its own target beside the name's. */
    inline-size: var(--layout-nav-touchrow);
    block-size: var(--layout-nav-touchrow);
    margin: 0;
    padding: 0;
    border: 0;
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    background: none;
    color: var(--text-subtle);
    cursor: pointer;
  }

  @media (prefers-reduced-motion: reduce) {
    .row, .grip { transition: none; }
    .row[data-drop='landed'] { animation: none; }
  }

  :global([data-motion='reduced']) .row,
  :global([data-motion='reduced']) .grip { transition: none; }
  :global([data-motion='reduced']) .row[data-drop='landed'] { animation: none; }
</style>
