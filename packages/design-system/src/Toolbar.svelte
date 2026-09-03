<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A row of controls that belong together, and one stop in the tab order for the row.
  //
  // The same trade the `Tabs` strip makes and for the same reason: a toolbar of eight icon buttons
  // that were each tabbable would put eight presses between a keyboard reader and the content
  // under it. `role="toolbar"` plus the arrows is what the ARIA practices specify, and this
  // component owns the roving `tabindex` so its children do not have to know they are in one.
  //
  // It takes children rather than data, unlike `Menu` and `SideNav`, because a toolbar's contents
  // are heterogeneous - a button, a separator, a select, a search field - and a list of items
  // would have to grow a kind for each. What it needs from them is only which are focusable, and
  // `focusables` answers that from the DOM at the moment a key is pressed.

  import type { Snippet } from 'svelte';

  import { focusables, rovingIndex } from './focus.ts';

  interface Props {
    /** What this group of controls is called. */
    label: string;
    /** Wraps onto a second line rather than scrolling, for rule 4's German. */
    children: Snippet;
  }

  const { label, children }: Props = $props();

  let bar = $state<HTMLElement | null>(null);

  // Read at the moment a key is pressed rather than held: a toolbar's controls appear and
  // disappear with what is selected, and a list captured on mount would be stale by the second
  // selection.
  function onKeydown(event: KeyboardEvent) {
    if (!bar) return;
    const controls = focusables(bar);
    if (controls.length === 0) return;

    const current = controls.indexOf(document.activeElement as HTMLElement);
    const next = rovingIndex(event.key, current, controls.length, {
      orientation: 'horizontal',
      dir: getComputedStyle(bar).direction === 'rtl' ? 'rtl' : 'ltr',
    });
    if (next === null) return;
    event.preventDefault();
    controls[next]?.focus();
  }

  // One stop in the tab order: the control that has focus keeps its own `tabindex`, and the rest
  // are taken out of it. Reapplied whenever the children change, which is what the effect is for.
  //
  // Setting `tabindex="-1"` here does not hide a control from `focusables` above, and that is load
  // bearing rather than luck: its selector matches native controls by their element
  // (`button:not([disabled])` and the rest), and only a *custom* focusable - a `div` given a
  // `tabindex` - is matched by the attribute and would drop out. A toolbar of those would lose its
  // arrows, so `Toolbar.test.js` pins it.
  $effect(() => {
    if (!bar) return;
    const controls = focusables(bar);
    const focused = controls.indexOf(document.activeElement as HTMLElement);
    const stop = focused >= 0 ? focused : 0;
    for (const [index, control] of controls.entries()) {
      control.tabIndex = index === stop ? 0 : -1;
    }
  });
</script>

<!-- `tabindex="-1"` for the reason the tab strip carries one: the stop in the tab order is a
     control inside, never this element. -->
<div
  class="toolbar"
  role="toolbar"
  aria-label={label}
  tabindex="-1"
  bind:this={bar}
  onkeydown={onKeydown}
>
  {@render children()}
</div>

<style>
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--density-row-gap);
    min-width: 0;
  }
</style>
