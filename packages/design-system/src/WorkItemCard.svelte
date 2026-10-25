<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One entry on a board.
  //
  // §4 asks for one thing beyond the row's content: `cover` as a **colour or an image**. Both are
  // the same field with two shapes (`domain-model.md` §3.4), and a card that supported only one
  // would render half the entries wrong rather than plainly.
  //
  // The colour cover is a `colorToken` like a label's, so it is one of the ten and the same pair
  // that makes a chip legible in both themes. The image cover is a media object the application uploads; this
  // renders one where it exists and never asks for one.
  //
  // It is a link, for `ListRow`'s reason: readers open cards in new tabs, and a `div` with a click
  // handler takes that away from them.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';

  interface Props {
    title: string;
    href?: string;
    /**
     * What a plain press on the title does instead of following `href`: opening the entry beside
     * the board (ADR-0061 decision 4), as `TaskRow` does in a list. The link keeps its address for
     * a new tab, and a press with a modifier is the browser's.
     */
    onOpen?: () => void;
    /** Whether this card is the one open beside the board. Announced as current, not only tinted. */
    isCurrent?: boolean;
    isCompleted?: boolean;
    /** `COLOR` paints the strip from a token; `IMAGE` shows the picture behind it. */
    coverKind?: 'COLOR' | 'IMAGE' | null;
    coverColorToken?: string | null;
    /** Already resolved to something the browser may load. Absent means nothing is drawn. */
    coverImageUrl?: string | null;
    /** What a screen reader is told the cover is. Empty where it is decoration. */
    coverAlt?: string;
    /**
     * The card's zones (concept round 2, §3), each drawn only where it holds something, so a card
     * with a title alone is as calm as a full one: the labels above the title, at most two and
     * "+n"; then the title, three lines at most; then one footer line - the date and the counts
     * at its start (`footer`), the people at its end (`people`).
     */
    labels?: Snippet;
    footer?: Snippet;
    people?: Snippet;
    children?: Snippet;
    /** A change on its way to the server: the `pending` role beside a word. Resolved text. */
    pendingLabel?: string;
  }

  const {
    title,
    href,
    onOpen,
    isCurrent = false,
    pendingLabel,
    isCompleted = false,
    coverKind,
    coverColorToken,
    coverImageUrl,
    coverAlt = '',
    labels,
    footer,
    people,
    children,
  }: Props = $props();

  const hasCover = $derived(
    (coverKind === 'COLOR' && !!coverColorToken) || (coverKind === 'IMAGE' && !!coverImageUrl),
  );
</script>

<article class="card" data-completed={isCompleted ? '' : undefined} data-pending={pendingLabel !== undefined ? '' : undefined} data-current={isCurrent ? '' : undefined}>
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
    {#if labels}<div class="labels">{@render labels()}</div>{/if}
    {#if href}
      <a
        class="title"
        {href}
        aria-current={isCurrent ? 'true' : undefined}
        onclick={(event) => {
          if (!onOpen || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
          event.preventDefault();
          onOpen();
        }}>{title}</a
      >
    {:else}
      <span class="title">{title}</span>
    {/if}
    {#if pendingLabel !== undefined}
      <span class="pending"><span class="pending-mark" aria-hidden="true"><Icon name="cloud-upload" size="sm" /></span>{pendingLabel}</span>
    {/if}
    {#if children}<div class="detail">{@render children()}</div>{/if}
    {#if footer || people}
      <div class="footer">
        {#if footer}{@render footer()}{/if}
        {#if people}<span class="people">{@render people()}</span>{/if}
      </div>
    {/if}
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
    background: var(--bg-raised);
    box-shadow: var(--shadow-raised);
  }

  .card:hover { border-color: var(--border-default); }

  /* Rule 3, as a list row marks it: the open card carries the rail as well as the border. */
  .card[data-current] {
    border-color: var(--accent-primary);
    box-shadow: var(--shadow-raised), inset var(--bw-thick) 0 0 0 var(--accent-primary);
  }

  :global([dir='rtl']) .card[data-current] {
    box-shadow: var(--shadow-raised), inset calc(var(--bw-thick) * -1) 0 0 0 var(--accent-primary);
  }

  /* A change waiting to be sent: the `pending` role on the mark, opacity only, stilled
     under reduced motion; the word carries it too (rule 3). */
  .pending {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .pending-mark {
    display: inline-flex;
    animation: breathe var(--motion-pending-duration) var(--motion-pending-easing) infinite alternate;
  }

  @keyframes breathe {
    from { opacity: 1; }
    to { opacity: 0.4; }
  }

  @media (prefers-reduced-motion: reduce) {
    .pending-mark { animation: none; }
  }

  :global([data-motion='reduced']) .pending-mark { animation: none; }

  /* Rule 3: a completed card is not told apart by a tint. The title is struck as well. */
  .card[data-completed] .title { color: var(--text-subtle); text-decoration: line-through; }

  /* Tier 1 of §7 on a card, as on a row: the title settles with the `celebration` role, in
     transform and opacity alone; under reduced motion the strike is the acknowledgement. */
  .card[data-completed] .title {
    animation: settle var(--motion-celebration-duration) var(--motion-celebration-easing) both;
  }

  @keyframes settle {
    0% { opacity: 0.4; transform: scale(0.98); }
    100% { opacity: 1; transform: scale(1); }
  }

  @media (prefers-reduced-motion: reduce) {
    .card[data-completed] .title { animation: none; }
  }

  :global([data-motion='reduced']) .card[data-completed] .title { animation: none; }

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

  /* Three lines at most: a long title is read whole in the panel, and the card stays a card. */
  .title {
    display: -webkit-box;
    overflow: hidden;
    color: var(--text-primary);
    font-size: var(--fs-100);
    font-weight: var(--fw-medium);
    line-height: var(--lh-snug);
    text-decoration: none;
    overflow-wrap: anywhere;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    line-clamp: 3;
  }

  a.title:hover { text-decoration: underline; }

  a.title:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-sm);
  }

  .detail { color: var(--text-secondary); font-size: var(--fs-075); }

  .labels { display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  /* A card without labels has no labels zone: an empty one kept its gap above the title, so a card
     of a title alone had more room at the top than at the bottom. */
  .labels:not(:has(*)) { display: none; }

  /* One line: what is dated and counted at the start, the people at the end. */
  .footer { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050) var(--sp-100); color: var(--text-secondary); font-size: var(--fs-075); }
  .people { display: inline-flex; margin-inline-start: auto; }

  /* A card with nothing to say beyond its title is the title alone, not a title and a gap: the
     footer goes when nothing in it is drawn - no date, no count, nobody on it. */
  .footer:empty,
  .footer:has(> :global(:only-child:empty)),
  .footer:not(:has(> :global(:not(.people)))):has(> .people:empty) { display: none; }
</style>
