<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The navigation as a drawer, below `expanded` (ADR-0061 decision 2).
  //
  // Composition and nothing else: a `Drawer` from the start edge, as wide as the pinned
  // navigation is, holding whatever the caller puts in a `SideNav` - the product's tree, the
  // workbench's index. There is no second overlay code here and no second tree; what `Escape`
  // reaches, where focus returns and what the backdrop does are `Drawer`'s answers, and the
  // register's (layers.ts).
  //
  // It comes from `inline-start` and only from there. A navigation is where reading begins, and
  // a drawer that opened from the end would put the way in at the far side of the direction.
  //
  // It does not close itself on a navigation, because it cannot know one happened: the caller
  // that handles `onnavigate` sets `isOpen` to false, and the drawer returns focus to the trigger
  // that opened it - which on a phone is the bar's menu button, exactly where the reader's thumb
  // still is.

  import type { Snippet } from 'svelte';

  import Drawer from './Drawer.svelte';

  interface Props {
    /** What the navigation is called, as the drawer's heading. Resolved text (ADR-0011). */
    title: string;
    isOpen?: boolean;
    /** The name of the close control. */
    dismissLabel: string;
    onClose?: () => void;
    children: Snippet;
  }

  let { title, isOpen = $bindable(false), dismissLabel, onClose, children }: Props = $props();
</script>

<div class="navdrawer">
  <Drawer bind:isOpen edge="inline-start" {title} {dismissLabel} {onClose}>
    {@render children()}
  </Drawer>
</div>

<style>
  /* The one thing this adds to Drawer: the width is the shell's, so the tree is the same width
     open over the page as it is pinned beside it. Inherited into the dialog's top layer. */
  .navdrawer {
    display: contents;
    --drawer-inline-size: min(var(--layout-sidenav-width), 100%);
  }
</style>
