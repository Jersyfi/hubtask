<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A list of actions, fully operable from the keyboard - which is F1-06's acceptance and the
  // reason the items are data rather than a snippet of children.
  //
  // Roving focus, type-ahead and "which item is the third one" are questions about a list. A menu
  // that took children would have to read them back out of the DOM, and the arithmetic that
  // answers them then could not be tested without a browser. Here it is `rovingIndex` and
  // `typeAheadIndex` in focus.ts, and this component is what turns their answer into a `focus()`.
  //
  // One `tabindex` of 0 among the items and -1 on the rest, per the ARIA authoring practices: the
  // menu is one stop in the page's tab order, and the arrows move within it. `Tab` closes it and
  // moves on, because a menu that swallowed `Tab` would trap a keyboard user in a list of actions
  // they had decided against.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import { focusReturn, rovingIndex, typeAheadIndex } from './focus.ts';
  import { openOverlay, type MenuItem } from './overlay.ts';
  import type { Placement } from './positioning.ts';

  interface Props {
    /** What the list is called. A menu with no name is a list of verbs with no subject. */
    label: string;
    items: readonly MenuItem[];
    placement?: Placement;
    isOpen?: boolean;
    /** Called with the item's id. The menu closes first: what happens next may take focus itself. */
    onselect?: (id: string) => void;
    trigger: Snippet<[Record<string, unknown>]>;
  }

  let {
    label,
    items,
    placement = { side: 'block-end', align: 'start' },
    isOpen = $bindable(false),
    onselect,
    trigger,
  }: Props = $props();

  const id = `menu-${Math.random().toString(36).slice(2, 9)}`;

  let anchor = $state<HTMLElement | null>(null);
  let surface = $state<HTMLElement | null>(null);
  let active = $state(-1);
  let opener: Element | null = null;

  const labels = $derived(items.map((item) => item.label));

  const triggerProps = $derived({
    'aria-expanded': isOpen,
    'aria-controls': isOpen ? id : undefined,
    'aria-haspopup': 'menu',
    onclick: () => open(0),
    onkeydown: (event: KeyboardEvent) => {
      // The arrows open the menu and say where to land, which is what a reader who never touches
      // the pointer expects and what the practices specify.
      if (event.key === 'ArrowDown') open(0);
      else if (event.key === 'ArrowUp') open(items.length - 1);
      else return;
      event.preventDefault();
    },
  });

  function open(index: number) {
    if (isOpen) {
      close();
      return;
    }
    opener = document.activeElement;
    active = index;
    isOpen = true;
  }

  function close(returnFocus = true) {
    if (!isOpen) return;
    isOpen = false;
    active = -1;
    if (returnFocus && !focusReturn(opener)) focusReturn(anchor?.firstElementChild);
  }

  function select(item: MenuItem) {
    if (item.disabledReason !== undefined) return;
    // Closed before the action runs: what the action does may itself take focus, and a menu still
    // standing over it would be the thing that took it back.
    close();
    onselect?.(item.id);
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Tab') {
      close(false);
      return;
    }
    const moved = rovingIndex(event.key, active, items.length);
    const found = moved === null ? typeAheadIndex(event.key, active, labels) : null;
    const next = moved ?? found;
    if (next === null) return;
    active = next;
    event.preventDefault();
  }

  // Focus follows the active index rather than being moved at every call site: there are five
  // places that change it, and four of them would eventually forget.
  $effect(() => {
    if (!isOpen || !surface || active < 0) return;
    surface.querySelectorAll<HTMLElement>('[role="menuitem"]')[active]?.focus();
  });

  $effect(() => {
    if (!isOpen || !anchor || !surface) return;
    return openOverlay({ layer: 'popover', trigger: anchor, surface, placement, onDismiss: close });
  });
</script>

<span class="anchor" bind:this={anchor}>
  {@render trigger(triggerProps)}
</span>
{#if isOpen}
  <!-- The keydown listener is on the list rather than on each item: the arrows are a property of
       the menu, and one handler cannot disagree with itself. -->
  <div
    {id}
    class="menu"
    role="menu"
    aria-label={label}
    tabindex="-1"
    bind:this={surface}
    onkeydown={onKeydown}
  >
    {#each items as item, index (item.id)}
      {#if item.hasSeparatorBefore && index > 0}
        <hr class="separator" />
      {/if}
      <button
        type="button"
        class="item"
        role="menuitem"
        data-destructive={item.isDestructive ? 'true' : undefined}
        tabindex={index === active ? 0 : -1}
        aria-disabled={item.disabledReason !== undefined ? 'true' : undefined}
        aria-describedby={item.disabledReason !== undefined ? `${id}-${item.id}-reason` : undefined}
        onclick={() => select(item)}
      >
        {#if item.icon}<Icon name={item.icon} size="sm" />{/if}
        <span class="label">{item.label}</span>
        {#if item.disabledReason !== undefined}
          <span id={`${id}-${item.id}-reason`} class="reason">{item.disabledReason}</span>
        {/if}
      </button>
    {/each}
  </div>
{/if}

<style>
  .anchor { display: inline-flex; }

  .menu {
    position: fixed;
    /* Anchored to a trigger and dismissed by it: the `popover` rank, from tokens.json. */
    z-index: var(--z-popover);
    display: flex;
    flex-direction: column;
    min-width: var(--sp-1000);
    max-width: 40ch;
    max-height: 60vh;
    overflow: auto;
    margin: var(--sp-050);
    padding: var(--sp-050);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-overlay);
    animation: open var(--motion-attach-duration) var(--motion-attach-easing) both;
  }

  .menu:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .item {
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: center;
    gap: var(--density-row-gap);
    padding: var(--density-row-block) var(--sp-150);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-100);
    text-align: start;
    cursor: pointer;
  }

  .item:hover { background: var(--bg-surface-hover); }

  /* Rule 5, and the reason the ring is worth checking here in particular: with roving focus the
     ring is the only thing that says which item the arrows are on. */
  .item:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .item[data-destructive='true'] { color: var(--text-danger); }

  .item[aria-disabled='true'] { color: var(--text-subtle); cursor: not-allowed; }
  .item[aria-disabled='true']:hover { background: transparent; }

  .label { grid-column: 2; overflow-wrap: anywhere; }

  /* The reason sits under the label rather than replacing it: an item that cannot be used still
     says what it would have done. */
  .reason {
    grid-column: 2;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .separator {
    margin: var(--sp-050) 0;
    border: none;
    border-top: var(--bw-hairline) solid var(--border-subtle);
  }

  /* Opacity alone, and the `transform` that used to be here is the reason why. `animation-fill-mode:
     both` leaves the last keyframe standing, and a `transform: none` keyframe computes to an
     identity *matrix* rather than to `none` - which still makes the element a containing block for
     every `position: fixed` descendant. A popover opened from inside this one was then laid out in
     its box and clipped by its `overflow`. Rule 6 allows both properties; only one of them is safe
     for a surface that other overlays are opened from. */
  @keyframes open {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .menu { animation: none; }
  }

  :global([data-motion='reduced']) .menu { animation: none; }
</style>
