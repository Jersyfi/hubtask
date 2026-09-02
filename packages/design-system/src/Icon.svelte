<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One component, one name, one set (ADR-0041).
  //
  // The usual argument against a name-taking `Icon` is the bundle: a component that can render any
  // of a thousand glyphs has to carry a thousand glyphs. It does not apply here, because the set
  // in the repository *is* the declared subset - `build/icons.js` writes only the names it is
  // given, so there is nothing for a tree-shaker to remove and nothing a direct import per icon
  // would save. What is left of the trade is the part that favours the name: one import, one
  // element, and a call site short enough that people use the icon rather than working around it.

  import { ICONS, type IconName } from './icons/index.ts';

  interface Props {
    name: IconName;
    /**
     * `md` is 24 px, the grid the set is drawn on; `sm` is 16 px, for a control that sits in a
     * line of text. The stroke is scaled with it so that the drawn weight looks the same at both -
     * a 1.5 stroke on a 24 grid shrunk to 16 px would read as 1, which is a different set.
     */
    size?: 'sm' | 'md';
    /**
     * The accessible name. Leave it out and the icon is `aria-hidden`, which is right whenever the
     * icon repeats a label beside it. Give it a name only when the icon is the whole meaning - and
     * for an icon-only control, prefer a real label through `VisuallyHidden` on the control itself.
     */
    label?: string;
  }

  const { name, size = 'md', label }: Props = $props();

  const nodes = $derived(ICONS[name] ?? []);
</script>

<svg
  class="icon"
  data-size={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden={label ? undefined : 'true'}
  role={label ? 'img' : undefined}
  aria-label={label}
>
  <!-- Elements rather than a string of markup: the shapes are known at build time, so nothing here
       needs `{@html}` and its parser. `svelte:element` inherits the SVG namespace from this tag. -->
  {#each nodes as [tag, attributes], index (index)}
    <svelte:element this={tag} {...attributes} />
  {/each}
</svg>

<style>
  .icon {
    display: inline-block;
    flex: none;
    /* The icon sits in a line of text and takes its colour from it: `currentColor` above, and a
       size in `em` so that it grows with the type scale rather than against it. */
    vertical-align: -0.125em;
    /* design-system-lint-ignore */
    stroke-width: 1.5;
  }

  /* 24 px and 16 px as multiples of the two font sizes they sit with, so that the pair stays
     correct when the whole page is zoomed (design-system.md §3 meets SC 1.4.4 through page zoom). */
  .icon[data-size='md'] {
    width: var(--fs-300);
    height: var(--fs-300);
  }

  .icon[data-size='sm'] {
    width: var(--fs-100);
    height: var(--fs-100);
    /* The 24-grid stroke thins as the box shrinks, so it is put back. 1.5 x 24/16 = 2.25. */
    /* design-system-lint-ignore */
    stroke-width: 2.25;
  }
</style>
