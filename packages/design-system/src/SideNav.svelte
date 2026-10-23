<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The tree the application is navigated by - and a tree is a keyboard interaction before it is a
  // picture.
  //
  // The practices for a tree are specific and they are what this implements: one stop in the tab
  // order, the arrows move between visible nodes, the direction arrows expand and collapse, `Home`
  // and `End` reach the ends, and the current node is **announced as current** rather than merely
  // coloured (rule 3, and `aria-current`). A tree that was only a list of indented links would be
  // none of that.
  //
  // It knows nothing about a hub. Nodes are handed to it, and the domain stays with the screen
  // that has one. What it does own is the arithmetic of "which node is next when three of
  // five are collapsed", which is why the flattening happens here rather than in a caller.

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';
  import { labelTokens, type LabelToken } from '../dist/tokens.ts';
  import { focusReturn } from './focus.ts';
  import { openOverlay } from './overlay.ts';
  import Self from './SideNav.svelte';
  import { HOLD_MS, boundariesOf, dropIndex, hasLeftTheHandle, rankIntent } from './reorder.ts';
  import { flattenTree, levelOf, treeIntent, type NavNode as StructureNode } from './structure.ts';

  /**
   * One node. Children make it a branch, unless `isBranch` says so on its own — which is what a
   * level fetched on demand needs, because "has children" is not known until it is opened.
   */
  export interface NavNode extends StructureNode {
    /** Narrowed from the module's `string`: a component may only name an icon that exists. */
    readonly icon?: IconName;
    /**
     * The place's own colour, one of the ten label tokens: the mark is drawn on the label's tile,
     * in the label's ink. A place's colour, never a level's (design-system.md §6) - and never alone:
     * the label beside it, and the symbol on it, say which place it is. A name that is not one of
     * the ten draws the plain mark.
     */
    readonly colorToken?: string;
    readonly children?: readonly NavNode[];
    /**
     * A short figure at the end of the row - what counts in the place, "5 open", "38 %". Text the
     * caller resolved; part of the row's name, so it is heard with the label. Not drawn on the
     * rail, which is a column of marks.
     */
    readonly meta?: string;
  }

  interface Props {
    /** What this navigation is called. */
    label: string;
    nodes: readonly NavNode[];
    /** The node the reader is on. It is announced, not only highlighted. */
    current?: string;
    /** The branches that are open, by id. Bindable: a caller usually restores it. */
    expanded?: string[];
    /**
     * Folded to its marks (ADR-0063 decision 2).
     *
     * A rail is a **drawing**, not a narrower panel: one mark per row, centred in the column,
     * with no twist, no label and no indent — the label stays the row's accessible name and
     * becomes its tooltip. A caller that only narrowed the column would push the marks out of it,
     * so at a rail of 56 px the twist would take the first 24, and the mark would be drawn from 44
     * to 68 with the half past the edge clipped.
     *
     * **Depth is the one thing a rail cannot draw**, so it does not try: it lists the roots, and
     * a branch pressed there goes to the branch *and* opens its own subtree in a flyout beside the
     * column — this same component, unfolded, with the branch as its root. Nothing is unreachable
     * while the navigation is folded, there is no second tree, and a press means what it means
     * unfolded.
     */
    isRail?: boolean;
    /**
     * What the flyout is called, from the branch's name — a reader who arrives on it by keyboard
     * hears which branch they are inside. A function rather than a string with a placeholder in
     * it, because the sentence belongs to the caller's catalogue (ADR-0011) and only the caller
     * can render it around a name.
     */
    flyoutLabel?: (name: string) => string;
    /**
     * What the twist is called, from the branch's name and whether it is open.
     *
     * The twist is a **control**, so it needs a name like any other: pressing a
     * branch's row goes to the branch, and what opens and closes it is the mark at the end of the
     * row. A caller that offers no word gets the old behaviour on the row - there is then nothing
     * to navigate to, and a row that neither opened nor went anywhere would be a dead row.
     */
    branchLabel?: (name: string, isExpanded: boolean) => string;
    onnavigate?: (id: string) => void;
    /**
     * Ranks a node among its siblings: `to` is the position it asks for in its level as drawn,
     * the number `rankTarget` answers with (`reorder.ts`). Only a node with `isMovable` offers it:
     * a grip to drag by, and `Alt+↑`/`↓` (with `Shift`, to the ends) on the focused row.
     *
     * A position rather than a command, because a drag lands three rows away as easily as one.
     * The write, and the sentence that says it happened, are the caller's: the tree draws the
     * order it is handed and moves nothing by itself, so the order on screen is the server's.
     */
    onreorder?: (id: string, to: number) => void;
  }

  let { label, nodes, current, expanded = $bindable([]), isRail = false, flyoutLabel, branchLabel, onnavigate, onreorder }: Props = $props();

  let tree = $state<HTMLElement | null>(null);
  let active = $state(0);

  // The flattening and the key arithmetic are `structure.ts` — the visible list is what every
  // question the keyboard asks is about, and a component that walked the tree instead would answer
  // "the next node" with one nobody can see.
  // Folded, the tree is its roots: what the flyout shows is the rest, and a rail that listed an
  // opened hub's collections as marks would be drawing depth it has no room to distinguish.
  const rows = $derived(flattenTree(nodes, isRail ? [] : expanded));

  /** The node's colour where it is one of the ten; anything else is no colour rather than a guess. */
  function tileOf(node: NavNode): LabelToken | undefined {
    const token = node.colorToken;
    return token && (labelTokens as readonly string[]).includes(token) ? (token as LabelToken) : undefined;
  }

  /** The branch whose flyout is open, in the rail. One at a time, like a menu. */
  let opened = $state<string | null>(null);
  const openedNode = $derived(opened === null ? undefined : rows.find((row) => row.node.id === opened)?.node);
  let surface = $state<HTMLElement | null>(null);

  // Positioned, dismissed and layered by the same code every other overlay uses (ADR-0039): the
  // flyout is beside the row it belongs to, `Escape` closes it, and a press outside does too.
  $effect(() => {
    const trigger = opened === null ? null : tree?.querySelector<HTMLElement>(`[data-node="${CSS.escape(opened)}"]`);
    if (!trigger || !surface) return;
    const release = openOverlay({
      layer: 'popover',
      trigger,
      surface,
      placement: { side: 'inline-end', align: 'start' },
      onDismiss: () => closeFlyout(),
    });
    surface.querySelector<HTMLElement>('[role="treeitem"]')?.focus();
    return release;
  });

  function closeFlyout() {
    const trigger = opened === null ? null : tree?.querySelector<HTMLElement>(`[data-node="${CSS.escape(opened)}"]`);
    opened = null;
    focusReturn(trigger);
  }

  /**
   * What pressing a row does.
   *
   * **A branch is a place before it is a container.** A hub has a screen of its own -
   * its settings, its collections, the control that makes another one - and while pressing its row
   * only unfolded it, the only way in was through a collection and back up the breadcrumb. So the
   * row goes to the branch and opens it, and the twist at the end of the row is what closes it
   * again; the arrows do what they have always done. A caller that names no twist has no second
   * control, and keeps the fold on the row.
   *
   * **Pressing a branch means the same thing in both drawings**: go to it, and open
   * it. Unfolded, "open it" is the subtree in place; folded, it is the flyout beside the column,
   * because a rail has nowhere to put a level. A rail whose mark only opened the flyout was a
   * navigation where a hub could be looked into and never entered.
   *
   * The flyout **expands the branch as well**, and that is not a flourish: `expanded` is what a
   * caller watches to fetch a level that is loaded on demand, so a flyout that only set its own
   * state would open beside a hub whose collections nobody had asked the server for.
   */
  function choose(row: { node: NavNode; isBranch: boolean; isExpanded: boolean }) {
    if (!row.isBranch) return onnavigate?.(row.node.id);
    if (isRail) {
      // A second press on the mark the flyout belongs to closes it; the branch is already open.
      if (opened === row.node.id) {
        opened = null;
        return;
      }
      toggle(row.node.id, true);
      opened = row.node.id;
      if (branchLabel) onnavigate?.(row.node.id);
      return;
    }
    if (!branchLabel) return toggle(row.node.id, !row.isExpanded);
    // Opened as well as opened *into*: a reader who presses a hub is asking to see what is in it,
    // and the level is fetched by the same `expanded` the twist writes.
    toggle(row.node.id, true);
    onnavigate?.(row.node.id);
  }
  // Focus follows the current node when the caller moves it, so arrowing after a navigation
  // continues from where the reader is rather than from where they were.
  const focused = $derived(
    Math.max(0, active < rows.length ? active : rows.findIndex((row) => row.node.id === current)),
  );

  function toggle(id: string, open: boolean) {
    expanded = open ? [...new Set([...expanded, id])] : expanded.filter((each) => each !== id);
  }

  function focusRow(index: number) {
    active = index;
    tree?.querySelector<HTMLElement>(`[data-index="${index}"]`)?.focus();
  }

  /** Whether a row offers to be ranked here: folded, a rail has no room for a grip or a level. */
  const isMovable = (row: { node: NavNode }) => onreorder !== undefined && !isRail && row.node.isMovable === true;

  /**
   * The node just ranked by key, which keeps the focus while the caller's order arrives: the
   * rows are drawn from what the caller hands back, so the focused index would otherwise stay
   * where the node was and the next `Alt+↓` would rank its old neighbour.
   */
  let following = $state<string | null>(null);

  $effect(() => {
    if (following === null) return;
    const index = rows.findIndex((row) => row.node.id === following);
    if (index < 0 || index === active) return;
    if (tree?.contains(document.activeElement)) focusRow(index);
    else active = index;
  });

  function onKeydown(event: KeyboardEvent) {
    // The row the key was pressed on, which is the focused one however it got the focus - a
    // caller or a test that focused a row directly has not moved `active`.
    const pressed = Number((event.target as Element | null)?.closest?.('[data-index]')?.getAttribute('data-index') ?? focused);
    if (pressed !== focused && rows[pressed]) active = pressed;
    const row = rows[pressed];
    if (!row) return;
    if (isMovable(row) && event.altKey && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
      // Answered even where there is nowhere to go: a refused rank is not a request to move the focus.
      event.preventDefault();
      const level = levelOf(rows, pressed);
      const to = rankIntent(event, level.indexOf(pressed), level.length);
      if (to === null) return;
      following = row.node.id;
      onreorder?.(row.node.id, to);
      return;
    }
    following = null;
    const dir = tree !== null && getComputedStyle(tree).direction === 'rtl' ? 'rtl' : 'ltr';

    const intent = treeIntent(event.key, rows, pressed, dir);
    if (intent === null) return;
    event.preventDefault();

    if (intent.kind === 'expand') {
      // Folded, "towards the children" is the flyout: it is where the children are.
      toggle(row.node.id, true);
      if (isRail && row.isBranch) opened = row.node.id;
    } else if (intent.kind === 'collapse') {
      if (isRail) closeFlyout();
      else toggle(row.node.id, false);
    } else focusRow(intent.index);
  }

  /**
   * The pointer path to the same call (`reorder.ts`): a drag ends by naming a position in the
   * level, which is what `Alt+↓` names too, so the two cannot disagree about where a row lands.
   * It starts on the grip only, because the row is a link: a fine pointer once it has travelled,
   * a coarse one after the hold, as every list here does (design-system.md §11.9).
   */
  let drag = $state<{ id: string; level: string[]; offset: number; to: number; from: number } | null>(null);
  let hasJustDragged = false;
  let stopDrag: (() => void) | undefined;

  function beginDrag(event: PointerEvent, index: number) {
    const row = rows[index];
    if (event.button !== 0 || !row || !isMovable(row)) return;
    const level = levelOf(rows, index).map((at) => rows[at]!.node.id);
    const from = level.indexOf(row.node.id);
    const origin = { x: event.clientX, y: event.clientY };
    let mayBegin = event.pointerType === 'mouse';
    const held = mayBegin ? undefined : setTimeout(() => (mayBegin = true), HOLD_MS);
    let hasBegun = false;

    const extentOf = (id: string) => {
      const box = tree?.querySelector<HTMLElement>(`[data-node="${CSS.escape(id)}"]`)?.getBoundingClientRect();
      // The carried row is drawn where the pointer is; its slot is where it was.
      const shift = id === row.node.id ? (drag?.offset ?? 0) : 0;
      return { start: (box?.top ?? 0) - shift, end: (box?.bottom ?? 0) - shift };
    };

    const onMove = (move: PointerEvent) => {
      if (!hasBegun) {
        if (!hasLeftTheHandle(origin, { x: move.clientX, y: move.clientY })) return;
        if (!mayBegin) return finish();
        hasBegun = true;
        drag = { id: row.node.id, level, offset: 0, to: from, from };
      }
      if (!drag) return;
      const to = dropIndex(move.clientY, boundariesOf(level.map(extentOf)));
      drag = { ...drag, offset: move.clientY - origin.y, to };
    };

    const finish = () => {
      clearTimeout(held);
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', finish);
      window.removeEventListener('pointercancel', finish);
      stopDrag = undefined;
      const landed = drag;
      drag = null;
      hasJustDragged = hasBegun;
      // A drag let go where it began writes nothing: thinking better of it is not a request.
      if (landed && landed.to !== landed.from) onreorder?.(landed.id, landed.to);
    };

    stopDrag?.();
    stopDrag = finish;
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', finish);
    window.addEventListener('pointercancel', finish);
  }

  // The click a drag ends with belongs to the drag, never to the row it was let go over, so it is
  // swallowed before the row's own handler sees it.
  $effect(() => {
    const node = tree;
    if (!node) return;
    const onClick = (event: MouseEvent) => {
      if (!hasJustDragged) return;
      hasJustDragged = false;
      event.preventDefault();
      event.stopPropagation();
    };
    node.addEventListener('click', onClick, true);
    return () => {
      node.removeEventListener('click', onClick, true);
      stopDrag?.();
    };
  });

  /** Where the carried row would land: the row now in that slot, drawn with the marker. */
  const dropTarget = $derived(drag && drag.to !== drag.from ? drag.level[drag.to] : undefined);
</script>

<nav class="side-nav" data-rail={isRail ? '' : undefined} aria-label={label}>
  <ul class="tree" role="tree" bind:this={tree} onkeydown={onKeydown}>
    <!-- `aria-selected` and `aria-current` are both here and they say different things:
         the first is the tree's own state, the second is that this is the page the reader is on.
         A navigation tree is the one place the two coincide, and a screen reader is told each in
         its own vocabulary. -->
    {#each rows as row, index (row.node.id)}
      {#if row.depth === 0 && row.node.band !== undefined}
        <!-- Where a band begins, as an element of its own: the hairline, the air above it, and the
             caption where there is one. `role="none"` because it is not a node of the tree — it
             says what the rows under it are, and the arrows walk past it the way they walk past a
             heading.
             **The separation is never drawn on a row.** A row carrying `padding-block-start` for it
             would need that padding taken off again at the head of the column — which takes the
             row's own vertical padding with it, and draws the first row of every band short and its
             background against its text.
             Folded, the caption is not drawn: a rail is a column of marks, and a five-word group
             name in it is words in a place that has no room for any. What separates
             one group of marks from the next is the hairline, which is what is left here. -->
        <li class="band" role="none">
          {#if row.node.band.caption && !isRail}<span>{row.node.band.caption}</span>{/if}
        </li>
      {/if}
      <li
        class="row"
        role="treeitem"
        data-index={index}
        data-node={row.node.id}
        title={isRail ? row.node.label : undefined}
        aria-label={isRail ? row.node.label : undefined}
        aria-expanded={row.isBranch ? (isRail ? opened === row.node.id : row.isExpanded) : undefined}
        aria-selected={row.node.id === current}
        aria-current={row.node.id === current ? 'page' : undefined}
        aria-keyshortcuts={isMovable(row) ? 'Alt+ArrowUp Alt+ArrowDown' : undefined}
        data-dragging={drag?.id === row.node.id ? '' : undefined}
        data-drop={dropTarget === row.node.id ? '' : undefined}
        style:--drag-offset={drag?.id === row.node.id ? `${drag.offset}px` : undefined}
        tabindex={index === focused ? 0 : -1}
        onclick={() => {
          active = index;
          following = null;
          choose(row);
        }}
        onkeydown={(event) => {
          if (event.key !== 'Enter' && event.key !== ' ') return;
          event.preventDefault();
          choose(row);
        }}
      >
        <!-- The mark first, at one inline position for every level: it is the column that
             survives the fold, and an indent in front of it is what would push it out of the
             rail. The indent moves the label instead. -->
        <span class="mark" aria-hidden="true">
          {#if row.node.icon && tileOf(row.node)}
            <span class="tile" data-token={tileOf(row.node)}><Icon name={row.node.icon} size="sm" /></span>
          {:else if row.node.icon}
            <Icon name={row.node.icon} size="sm" />
          {/if}
        </span>
        {#if !isRail}
          <span class="label" style:--depth={row.depth}>{row.node.label}</span>
          {#if row.node.meta}<span class="meta">{row.node.meta}</span>{/if}
          <!-- The grip: what a pointer drags by. Hidden from the accessibility tree, because the
               keys on the row are the same command, and a press on it that does not travel is the
               row's own click. -->
          {#if isMovable(row)}
            <span class="grip" data-grip aria-hidden="true" onpointerdown={(event) => beginDrag(event, index)}>
              <Icon name="grip-vertical" size="sm" />
            </span>
          {/if}
          <!-- And the twist at the end of the row, where the reading direction ends: `inline-end`
               through the logical padding, so it mirrors with the document.
               A **control** where the caller named one, because the row itself now goes to the
               branch: it is out of the tab order - the tree keeps its one stop and
               the arrows keep expanding - and the row's `aria-expanded` is what says the state,
               so this is a second way to reach it with a pointer rather than a second statement
               about it. -->
          {#if row.isBranch && branchLabel}
            <button
              type="button"
              class="twist"
              tabindex="-1"
              aria-label={branchLabel(row.node.label, row.isExpanded)}
              title={branchLabel(row.node.label, row.isExpanded)}
              onclick={(event) => {
                event.stopPropagation();
                active = index;
                toggle(row.node.id, !row.isExpanded);
              }}
            >
              <Icon name={row.isExpanded ? 'chevron-down' : 'chevron-right'} size="sm" />
            </button>
          {:else}
            <span class="twist" aria-hidden="true">
              {#if row.isBranch}
                <Icon name={row.isExpanded ? 'chevron-down' : 'chevron-right'} size="sm" />
              {/if}
            </span>
          {/if}
        {/if}
      </li>
    {/each}
  </ul>
  <!-- The branch, unfolded, beside the column: this same component with the branch as its root,
       so the keyboard, the announcements and the current mark are the ones the tree already has
       and there is no second tree to keep in step. -->
  {#if isRail && openedNode}
    <div class="flyout" bind:this={surface}>
      <Self
        label={flyoutLabel ? flyoutLabel(openedNode.label) : openedNode.label}
        nodes={[openedNode]}
        {current}
        expanded={[openedNode.id]}
        onnavigate={(id) => {
          closeFlyout();
          onnavigate?.(id);
        }}
      />
    </div>
  {/if}
</nav>

<style>
  .side-nav { min-width: 0; }

  .tree { margin: 0; padding: 0; list-style: none; }

  /* Where a band begins: a hairline and the air that says "these are a different kind of thing".
     Its own element, so that no row's geometry depends on which band it opens. */
  .band {
    margin-block-start: var(--sp-200);
    padding-block-start: var(--sp-200);
    padding-inline: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  /* The column's own head opens no band, whatever the first node carries: there is nothing above
     it to be separated from. */
  .tree > .band:first-child {
    margin-block-start: 0;
    padding-block-start: 0;
    border-block-start: 0;
  }

  /* The caption, in the `label` role §3 gives a field name and a group title. The air under it is
     the caption's, so a band with none is the hairline and the space above it and nothing else. */
  .band > span {
    display: block;
    padding-block-end: var(--sp-050);
    color: var(--text-subtle);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--density-row-gap);
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-100);
    border-radius: var(--r-md);
    color: var(--text-secondary);
    font-size: var(--fs-100);
    cursor: pointer;
    user-select: none;
  }

  .row:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  /* Rule 3: the current node is announced by `aria-current` and carries a mark of its own, so it
     does not rest on the colour alone. */
  .row[aria-current='page'] {
    background: var(--bg-surface-pressed);
    color: var(--text-primary);
    font-weight: var(--fw-medium);
    box-shadow: inset var(--bw-thick) 0 0 0 var(--accent-primary);
  }

  :global([dir='rtl']) .row[aria-current='page'] {
    box-shadow: inset calc(var(--bw-thick) * -1) 0 0 0 var(--accent-primary);
  }

  .row:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(var(--sp-025) * -1);
  }

  .twist,
  .mark {
    display: inline-flex;
    flex: none;
    width: var(--sp-300);
    justify-content: center;
    color: var(--text-subtle);
  }

  /* A place's own colour: the label's measured pair, its ink on its tile, so the symbol keeps the
     contrast the label test holds it to in both modes. The tile fits inside the mark's column,
     which is why the rail and the tree align whether a row has a colour or not. */
  .tile {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    inline-size: var(--sp-250);
    block-size: var(--sp-250);
    border-radius: var(--r-sm);
    background: var(--tile-bg);
    color: var(--tile-fg);
  }

  .tile[data-token='slate'] { --tile-bg: var(--label-slate-bg); --tile-fg: var(--label-slate-fg); }
  .tile[data-token='blue'] { --tile-bg: var(--label-blue-bg); --tile-fg: var(--label-blue-fg); }
  .tile[data-token='teal'] { --tile-bg: var(--label-teal-bg); --tile-fg: var(--label-teal-fg); }
  .tile[data-token='green'] { --tile-bg: var(--label-green-bg); --tile-fg: var(--label-green-fg); }
  .tile[data-token='lime'] { --tile-bg: var(--label-lime-bg); --tile-fg: var(--label-lime-fg); }
  .tile[data-token='amber'] { --tile-bg: var(--label-amber-bg); --tile-fg: var(--label-amber-fg); }
  .tile[data-token='orange'] { --tile-bg: var(--label-orange-bg); --tile-fg: var(--label-orange-fg); }
  .tile[data-token='red'] { --tile-bg: var(--label-red-bg); --tile-fg: var(--label-red-fg); }
  .tile[data-token='magenta'] { --tile-bg: var(--label-magenta-bg); --tile-fg: var(--label-magenta-fg); }
  .tile[data-token='violet'] { --tile-bg: var(--label-violet-bg); --tile-fg: var(--label-violet-fg); }

  /* The twist where it is a control: no chrome of its own until it is under the pointer, so the
     row still reads as one thing. */
  button.twist {
    align-items: center;
    align-self: stretch;
    padding: 0;
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    font: inherit;
    cursor: pointer;
  }

  button.twist:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  /* Rule 5's own ring, drawn outside the twist: the row's is inset because a row fills the column
     and an outer ring would be cut by it; the twist has room around it. */
  button.twist:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* The indent is the label's, and it is `padding-inline-start`, which mirrors itself: a
     `padding-left` would put the indent on the wrong side of an RTL tree, and that is what the
     direction axis catches.
     Two lines before it gives up, not one: a row whose label says what the place is for — the
     installation's "applies to every workspace you create" in a phone's drawer — said nothing
     once cut after its first words. A third line is where a row stops being a row, so the clamp
     is there and the whole text stays the row's accessible name. The prefixed form is what every
     engine implements; the standard property stands beside it, as in `TaskRow`. */
  .label {
    flex: 1;
    min-width: 0;
    padding-inline-start: calc(var(--depth) * var(--sp-200));
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    /* Between words, and inside one only when it alone is wider than the row: "anywhere" broke a
       short name mid-word as soon as a figure stood beside it. */
    overflow-wrap: break-word;
    hyphens: auto;
  }

  /* The figure keeps to one line and to its own width, so a long label wraps beside it rather than
     pushing it out of the row. */
  .meta {
    flex: none;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  /* The grip, faded until the pointer is over the row or the focus is in it, where a pointer
     hovers at all (design-system.md §11.9): faded rather than taken out, so the row keeps its
     width. A touch screen draws it always. `touch-action: none` is what lets a finger drag it at
     all, and it is on the grip alone, so the column still scrolls everywhere else. */
  .grip {
    display: inline-flex;
    flex: none;
    align-items: center;
    color: var(--text-subtle);
    cursor: grab;
    touch-action: none;
  }

  /* Where a pointer hovers, the grip stands in the gutter before the row, as on a list's rows, so
     it takes no width from the label: a row with a grip wraps exactly where one without does. */
  @media (hover: hover) {
    .row:has(.grip) { position: relative; }

    .row .grip {
      position: absolute;
      inset-inline-start: calc(-1 * var(--sp-150));
      opacity: 0;
      transition: opacity var(--motion-state-duration) var(--motion-state-easing);
    }

    .row:hover .grip,
    .row:focus-within .grip,
    .row[data-dragging] .grip { opacity: 1; }
  }

  /* Rule 6: the carried row moves by a `translate` and nothing else, raised off the column
     (rule 1), and out of the measuring: the element under the pointer is the column, not it. */
  .row[data-dragging] {
    translate: 0 var(--drag-offset);
    background: var(--bg-surface);
    box-shadow: var(--shadow-overlay);
    pointer-events: none;
  }

  .row[data-dragging] .grip { cursor: grabbing; }

  /* Where it would land. Rule 3: an outline, not a tint. */
  .row[data-drop] {
    outline: var(--bw-thick) dashed var(--accent-primary);
    outline-offset: calc(var(--sp-025) * -1);
  }

  @media (prefers-reduced-motion: reduce) {
    .row[data-dragging] { translate: none; }
  }

  :global([data-motion='reduced']) .row[data-dragging] { translate: none; }

  /* The flyout: positioned by `anchorTo`, layered by `openOverlay`, drawn like every other
     surface that is temporarily over the page (rule 1). */
  .flyout {
    position: fixed;
    z-index: var(--z-popover);
    min-inline-size: var(--layout-sidenav-width);
    max-inline-size: 40ch;
    max-block-size: 80vh;
    overflow: auto;
    margin: var(--sp-050);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
  }

  /* The fold: the mark alone, centred, and the row no wider than the column it is in. The
     padding goes with the indent and the twist, because what is left has nothing to sit beside. */
  .side-nav[data-rail] .row {
    justify-content: center;
    padding-inline: var(--sp-050);
  }
</style>
