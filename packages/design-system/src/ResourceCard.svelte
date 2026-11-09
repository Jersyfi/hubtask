<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One item a person picks from - a template, a hub, a dashboard - flat on the canvas (page-system
  // §4.2, level 1′): a hairline and no shadow, the hover a step of surface and a firmer border. The
  // card is the link; the act (*Use*) is on its record page or in its dialog, never on the card,
  // and a card holds no form (PS-4). On a phone it reads in three lines - mark and name, the
  // sentence held to two, chips and state - so a catalogue stays a few screens long (§4.4).
  import Badge from './Badge.svelte';
  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';

  interface Props {
    title: string;
    /** One sentence. */
    description?: string;
    mark?: IconName;
    /** Up to three; a fourth is not drawn. */
    chips?: readonly string[];
    /** A state line: *In use · update*. */
    state?: { label: string; tone?: 'success' | 'warning' | 'danger' | 'info' | 'neutral' };
    /** The record's address. */
    href?: string;
    /** Pressing a card without an address: its dialog. */
    onopen?: () => void;
  }

  const { title, description, mark, chips = [], state, href, onopen }: Props = $props();
</script>

<li class="card">
  {#if mark}<span class="mark" aria-hidden="true"><Icon name={mark} size="sm" /></span>{/if}
  <h3 class="title">
    {#if href}
      <a class="press" {href}>{title}</a>
    {:else if onopen}
      <button type="button" class="press" onclick={onopen}>{title}</button>
    {:else}
      {title}
    {/if}
  </h3>
  {#if description}<p class="description">{description}</p>{/if}
  {#if chips.length > 0}
    <span class="chips">
      {#each chips.slice(0, 3) as chip (chip)}<Badge size="sm">{chip}</Badge>{/each}
    </span>
  {/if}
  {#if state}<span class="state"><Badge tone={state.tone ?? 'neutral'} size="sm">{state.label}</Badge></span>{/if}
</li>

<style>
  .card {
    position: relative;
    display: grid;
    align-content: start;
    gap: var(--sp-050);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-primary);
    transition: background-color var(--motion-state-duration) var(--motion-state-easing), border-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .card:has(.press):hover { border-color: var(--border-default); background: var(--bg-surface-hover); }

  .mark {
    display: grid;
    place-items: center;
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    border-radius: var(--r-md);
    background: var(--accent-primary-subtle);
    color: var(--text-brand);
  }

  .title { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); line-height: var(--lh-tight); overflow-wrap: anywhere; }

  .press {
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: start;
    text-decoration: none;
    cursor: pointer;
  }

  .press::after { content: ''; position: absolute; inset: 0; border-radius: var(--r-lg); }

  .press:focus-visible { outline: none; }
  .press:focus-visible::after { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .description { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .state { justify-self: start; }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .card { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050) var(--sp-100); padding: var(--sp-100) var(--sp-150); }
    .mark { flex: none; inline-size: var(--sp-300); block-size: var(--sp-300); }
    .title { flex: 1 1 0; min-inline-size: 0; font-size: var(--fs-100); }
    .description {
      flex-basis: 100%;
      font-size: var(--fs-075);
      display: -webkit-box;
      -webkit-box-orient: vertical;
      -webkit-line-clamp: 2;
      line-clamp: 2;
      overflow: hidden;
    }
    .chips, .state { flex: none; }
  }

  @media (prefers-reduced-motion: reduce) {
    .card { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .card { transition-duration: var(--dur-instant); }
</style>
