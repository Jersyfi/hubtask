<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // News not read yet, as a small dot in the accent: a post in the hub column, a card of the posts.
  //
  // Neutral by intent - it says "new", never "act now". What asks for action is the figure on the
  // danger surface (`NavItem`'s `isFigureUrgent`); a red dot here would teach a reader to fear news.
  // With a `label` it is an image that says so; without one it is decoration beside words that
  // already say it, hidden from the tree.

  interface Props {
    /** What the dot says - "Not read yet" - where nothing beside it does. */
    label?: string;
  }

  const { label }: Props = $props();
</script>

{#if label}
  <span class="dot" role="img" aria-label={label} title={label}></span>
{:else}
  <span class="dot" aria-hidden="true"></span>
{/if}

<style>
  .dot {
    display: inline-block;
    flex: none;
    inline-size: var(--sp-100);
    block-size: var(--sp-100);
    border-radius: var(--r-full);
    background: var(--accent-primary);
    vertical-align: middle;
  }

  @media (forced-colors: active) {
    .dot { background: CanvasText; }
  }
</style>
