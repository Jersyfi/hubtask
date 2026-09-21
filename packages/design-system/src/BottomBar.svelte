<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The primary destinations on a phone (ADR-0061 decision 1): three to five, each a mark and a
  // word, pinned to the bottom of the screen where a thumb is.
  //
  // It switches routes, not panels, which is why it is a `<nav>` of links and not `Tabs`: a tab
  // owns the panel it reveals, a destination owns nothing. The current one carries
  // `aria-current="page"` and a colour, and the word beside the mark is what says it in
  // greyscale (rule 3). Never more than five - a sixth would be narrower than a thumb - and never
  // fewer than three, because two is a switch and a switch is a different control.
  //
  // It hides while an input has focus. A bar fixed to the bottom rides the on-screen keyboard up
  // and covers the field it belongs to; the hiding is opacity, so nothing moves (rule 6). The
  // caller decides on which width it appears - this component knows nothing about the viewport.

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';

  export interface Destination {
    readonly id: string;
    /** Resolved text (ADR-0011). Short: it sits under a mark in a fifth of a phone. */
    readonly label: string;
    readonly icon: IconName;
    readonly href: string;
    /** A count worth seeing before opening it: what waits in the inbox. */
    readonly count?: number;
  }

  interface Props {
    /** What the bar is called, for the landmark. */
    label: string;
    destinations: readonly Destination[];
    /** The destination the reader is on. Announced, not only coloured. */
    current?: string;
    onnavigate?: (id: string) => void;
  }

  const { label, destinations, current, onnavigate }: Props = $props();
</script>

<nav class="bar" aria-label={label}>
  <ul>
    {#each destinations as destination (destination.id)}
      <li>
        <a
          href={destination.href}
          aria-current={destination.id === current ? 'page' : undefined}
          onclick={(event) => {
            if (!onnavigate) return;
            event.preventDefault();
            onnavigate(destination.id);
          }}
        >
          <span class="mark">
            <Icon name={destination.icon} />
            {#if destination.count}
              <span class="count">{destination.count}</span>
            {/if}
          </span>
          <span class="word">{destination.label}</span>
        </a>
      </li>
    {/each}
  </ul>
</nav>

<style>
  .bar {
    position: fixed;
    inset-inline: 0;
    inset-block-end: 0;
    z-index: var(--z-sticky);
    /* Read, not written: the home indicator's inset on a phone, nothing in a browser. */
    padding-block-end: env(safe-area-inset-bottom, 0);
    background: var(--bg-surface);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    transition: opacity var(--motion-state-duration) var(--motion-state-easing);
  }

  ul {
    display: flex;
    justify-content: space-around;
    align-items: stretch;
    block-size: var(--layout-bottombar-height);
    margin: 0;
    padding: 0 var(--sp-050);
    list-style: none;
  }

  li { flex: 1; min-width: 0; display: flex; }

  a {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--sp-025);
    flex: 1;
    min-width: 0;
    padding: var(--sp-050) var(--sp-025);
    color: var(--text-secondary);
    font-size: var(--fs-050);
    font-weight: var(--fw-medium);
    text-decoration: none;
    border-radius: var(--r-md);
  }

  a:hover { color: var(--text-primary); }

  /* Rule 3: the current destination is the colour and the tint under the mark, and the word and
     `aria-current` say it without either. */
  a[aria-current='page'] { color: var(--text-brand); }

  a[aria-current='page'] .mark { background: var(--accent-primary-subtle); }

  /* Rule 5's ring at rule 5's offset, drawn inward of the link's box so it is not clipped by the
     screen's bottom edge: the link is inset by the same amount the ring stands off. */
  a {
    margin: var(--sp-025);
  }

  a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .mark {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    inline-size: var(--sp-600);
    block-size: var(--sp-300);
    border-radius: var(--r-full);
  }

  .count {
    position: absolute;
    inset-block-start: 0;
    inset-inline-end: 0;
    min-inline-size: var(--sp-200);
    padding: 0 var(--sp-050);
    border-radius: var(--r-full);
    background: var(--status-info-accent);
    color: var(--text-inverse);
    font-size: var(--fs-050);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-normal);
    text-align: center;
  }

  .word {
    max-inline-size: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* While a field has focus the keyboard has the bottom of the screen; the bar gives way. `:has`
     is on the browser row (ADR-0044). Opacity, not display: the bar is still there for the
     reader who tabs out of the field, and nothing moves. A checkbox, a radio or a button is an
     input that raises no keyboard, so the bar stays for those - a reader ticking off entries on
     a phone would otherwise lose the bar with every tick. */
  :global(:root:has(input:not([type='checkbox'], [type='radio'], [type='button'], [type='submit'], [type='reset'], [type='range'], [type='color'], [type='file']):focus, textarea:focus, select:focus)) .bar {
    opacity: 0;
    pointer-events: none;
  }

  @media (prefers-reduced-motion: reduce) {
    .bar { transition: none; }
  }

  :global([data-motion='reduced']) .bar { transition: none; }
</style>
