<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry on a board.
  //
  // §4 asks for one thing beyond the row's content: `cover` as a **colour or an image**. Both are
  // the same field with two shapes (`domain-model.md` §3.4), and a card that supported only one
  // would render half the entries wrong rather than plainly.
  //
  // The colour cover is a `colorToken` like a label's, so it is one of the ten and the same pair
  // that makes a chip legible in both themes. The image cover is a media object F3 uploads; this
  // renders one where it exists and never asks for one.
  //
  // It is a link, for `ListRow`'s reason: readers open cards in new tabs, and a `div` with a click
  // handler takes that away from them.

  import type { Snippet } from 'svelte';

  interface Props {
    title: string;
    href?: string;
    isCompleted?: boolean;
    /** `COLOR` paints the strip from a token; `IMAGE` shows the picture behind it. */
    coverKind?: 'COLOR' | 'IMAGE' | null;
    coverColorToken?: string | null;
    /** Already resolved to something the browser may load. Absent means nothing is drawn. */
    coverImageUrl?: string | null;
    /** What a screen reader is told the cover is. Empty where it is decoration. */
    coverAlt?: string;
    /** Labels, a badge, a count. */
    footer?: Snippet;
    children?: Snippet;
  }

  const {
    title,
    href,
    isCompleted = false,
    coverKind,
    coverColorToken,
    coverImageUrl,
    coverAlt = '',
    footer,
    children,
  }: Props = $props();

  const hasCover = $derived(
    (coverKind === 'COLOR' && !!coverColorToken) || (coverKind === 'IMAGE' && !!coverImageUrl),
  );
</script>

<article class="card" data-completed={isCompleted ? '' : undefined}>
  {#if hasCover}
    {#if coverKind === 'IMAGE'}
      <!-- `alt` is the caller's: a cover that means something is described, and one that is
           decoration carries the empty string rather than the file name. -->
      <img class="cover" src={coverImageUrl} alt={coverAlt} />
    {:else}
      <span class="cover" data-token={coverColorToken} aria-hidden="true"></span>
    {/if}
  {/if}

  <div class="body">
    {#if href}
      <a class="title" {href}>{title}</a>
    {:else}
      <span class="title">{title}</span>
    {/if}
    {#if children}<div class="detail">{@render children()}</div>{/if}
    {#if footer}<div class="footer">{@render footer()}</div>{/if}
  </div>
</article>

<style>
  .card {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    /* Rule 1: a card is a standalone element, so it is raised. */
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .card:hover { border-color: var(--border-default); }

  /* Rule 3: a completed card is not told apart by a tint. The title is struck as well. */
  .card[data-completed] .title { color: var(--text-subtle); text-decoration: line-through; }

  .cover { display: block; inline-size: 100%; }

  img.cover { block-size: var(--sp-1000); object-fit: cover; }

  /* A colour cover is a strip rather than a filled card: the words have to stay legible, and a
     label token's `bg` was measured against its own `fg` rather than against the card's. */
  span.cover { block-size: var(--sp-100); }

  span.cover[data-token='slate'] { background: var(--label-slate-bg); }
  span.cover[data-token='blue'] { background: var(--label-blue-bg); }
  span.cover[data-token='teal'] { background: var(--label-teal-bg); }
  span.cover[data-token='green'] { background: var(--label-green-bg); }
  span.cover[data-token='lime'] { background: var(--label-lime-bg); }
  span.cover[data-token='amber'] { background: var(--label-amber-bg); }
  span.cover[data-token='orange'] { background: var(--label-orange-bg); }
  span.cover[data-token='red'] { background: var(--label-red-bg); }
  span.cover[data-token='magenta'] { background: var(--label-magenta-bg); }
  span.cover[data-token='violet'] { background: var(--label-violet-bg); }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--density-row-block) var(--sp-150);
    min-width: 0;
  }

  .title {
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-decoration: none;
    overflow-wrap: anywhere;
  }

  a.title:hover { text-decoration: underline; }

  a.title:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-sm);
  }

  .detail { color: var(--text-secondary); font-size: var(--fs-075); }

  .footer { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
</style>
