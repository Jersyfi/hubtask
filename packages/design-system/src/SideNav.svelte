<!-- SPDX-License-Identifier: BUSL-1.1
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
  // It knows nothing about a hub. Nodes are handed to it, and the domain arrives with the screen
  // that has one (F2-08). What it does own is the arithmetic of "which node is next when three of
  // five are collapsed", which is why the flattening happens here rather than in a caller.

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';
  import { flattenTree, treeIntent, type NavNode as StructureNode } from './structure.ts';

  /**
   * One node. Children make it a branch, unless `isBranch` says so on its own — which is what a
   * level fetched on demand needs, because "has children" is not known until it is opened.
   */
  export interface NavNode extends StructureNode {
    /** Narrowed from the module's `string`: a component may only name an icon that exists. */
    readonly icon?: IconName;
    readonly children?: readonly NavNode[];
  }

  interface Props {
    /** What this navigation is called. */
    label: string;
    nodes: readonly NavNode[];
    /** The node the reader is on. It is announced, not only highlighted. */
    current?: string;
    /** The branches that are open, by id. Bindable: a caller usually restores it. */
    expanded?: string[];
    onnavigate?: (id: string) => void;
  }

  let { label, nodes, current, expanded = $bindable([]), onnavigate }: Props = $props();

  let tree = $state<HTMLElement | null>(null);
  let active = $state(0);

  // The flattening and the key arithmetic are `structure.ts` — the visible list is what every
  // question the keyboard asks is about, and a component that walked the tree instead would answer
  // "the next node" with one nobody can see.
  const rows = $derived(flattenTree(nodes, expanded));
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

  function onKeydown(event: KeyboardEvent) {
    const row = rows[focused];
    if (!row) return;
    const dir = tree !== null && getComputedStyle(tree).direction === 'rtl' ? 'rtl' : 'ltr';

    const intent = treeIntent(event.key, rows, focused, dir);
    if (intent === null) return;
    event.preventDefault();

    if (intent.kind === 'expand') toggle(row.node.id, true);
    else if (intent.kind === 'collapse') toggle(row.node.id, false);
    else focusRow(intent.index);
  }
</script>

<nav class="side-nav" aria-label={label}>
  <ul class="tree" role="tree" bind:this={tree} onkeydown={onKeydown}>
    <!-- `aria-selected` and `aria-current` are both here and they say different things:
         the first is the tree's own state, the second is that this is the page the reader is on.
         A navigation tree is the one place the two coincide, and a screen reader is told each in
         its own vocabulary. -->
    {#each rows as row, index (row.node.id)}
      <li
        class="row"
        role="treeitem"
        data-index={index}
        style:--depth={row.depth}
        aria-expanded={row.isBranch ? row.isExpanded : undefined}
        aria-selected={row.node.id === current}
        aria-current={row.node.id === current ? 'page' : undefined}
        tabindex={index === focused ? 0 : -1}
        onclick={() => {
          active = index;
          if (row.isBranch) toggle(row.node.id, !row.isExpanded);
          else onnavigate?.(row.node.id);
        }}
        onkeydown={(event) => {
          if (event.key !== 'Enter' && event.key !== ' ') return;
          event.preventDefault();
          if (row.isBranch) toggle(row.node.id, !row.isExpanded);
          else onnavigate?.(row.node.id);
        }}
      >
        <span class="twist" aria-hidden="true">
          {#if row.isBranch}
            <Icon name={row.isExpanded ? 'chevron-down' : 'chevron-right'} size="sm" />
          {/if}
        </span>
        {#if row.node.icon}
          <span class="mark" aria-hidden="true"><Icon name={row.node.icon} size="sm" /></span>
        {/if}
        <span class="label">{row.node.label}</span>
      </li>
    {/each}
  </ul>
</nav>

<style>
  .side-nav { min-width: 0; }

  .tree { margin: 0; padding: 0; list-style: none; }

  .row {
    display: flex;
    align-items: center;
    gap: var(--density-row-gap);
    /* The indent is `padding-inline-start`, which mirrors itself: a `padding-left` would put the
       indent on the wrong side of an RTL tree, and that is what the direction axis catches. */
    padding-block: var(--density-row-block);
    padding-inline: calc(var(--sp-100) + var(--depth) * var(--sp-200)) var(--sp-100);
    border-radius: var(--r-md);
    color: var(--text-secondary);
    font-size: var(--fs-100);
    cursor: pointer;
    user-select: none;
  }

  .row:hover { background: var(--bg-surface-raised); color: var(--text-primary); }

  /* Rule 3: the current node is announced by `aria-current` and carries a mark of its own, so it
     does not rest on the colour alone. */
  .row[aria-current='page'] {
    background: var(--bg-surface-raised);
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

  :global([dir='rtl']) .twist { transform: scaleX(-1); }

  .label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
