<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A panel beside the sidebar that opens from one of its controls (design-system.md §4, wave 5;
  // the navigation concept's §2.3, §2.4): *Recent* - the last twenty places - and the hub quick
  // switch - a search field, starred hubs, recently visited hubs, *All hubs*. It is a popover
  // (`Popover`'s overlay and layer), so `Escape` and a press outside close it and the focus
  // returns to its trigger; what it lists is the caller's, as `NavGroup`s and `NavItem`s.
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';
  import { focusReturn, focusables } from './focus.ts';
  import { openOverlay } from './overlay.ts';
  import type { Placement } from './anchor.ts';

  interface Props {
    /** The panel's heading and accessible name: *Recent*, *Switch hub*. */
    label: string;
    /** `row` draws the trigger as a sidebar row (*Recent ›*); `icon` as a small mark (▾). */
    trigger: 'row' | 'icon';
    icon: IconName;
    /** The row's words, or the icon's accessible name. */
    triggerLabel: string;
    /** A search field at the top, with its accessible name and placeholder. */
    searchLabel?: string;
    /** The words in it, bindable. */
    search?: string;
    isOpen?: boolean;
    /**
     * What the panel lines up with, when not its trigger: the hub's head, which the quick switch
     * drops from. The anchor needs a box of its own - a `display: contents` wrapper has none, and
     * the panel then lands at the viewport's corner.
     */
    anchor?: HTMLElement | null;
    placement?: Placement;
    children: Snippet;
  }

  let {
    label,
    trigger,
    icon,
    triggerLabel,
    searchLabel,
    search = $bindable(''),
    isOpen = $bindable(false),
    anchor,
    placement = { side: 'inline-end', align: 'start' },
    children,
  }: Props = $props();

  const instanceId = $props.id();
  const id = `flyout-${instanceId}`;
  let button = $state<HTMLButtonElement | null>(null);
  let surface = $state<HTMLElement | null>(null);
  let opener: Element | null = null;

  function close() {
    if (!isOpen) return;
    isOpen = false;
    if (!focusReturn(opener)) focusReturn(button);
  }

  $effect(() => {
    if (!isOpen || !button || !surface) return;
    const release = openOverlay({ layer: 'popover', trigger: button, anchor: anchor ?? button, surface, placement, onDismiss: close });
    focusables(surface)[0]?.focus();
    return release;
  });
</script>

<span class="anchor" data-trigger={trigger}>
  <button
    bind:this={button}
    type="button"
    class="trigger"
    aria-expanded={isOpen}
    aria-controls={isOpen ? id : undefined}
    aria-haspopup="dialog"
    aria-label={trigger === 'icon' ? triggerLabel : undefined}
    title={trigger === 'icon' ? triggerLabel : undefined}
    onclick={() => {
      opener = document.activeElement;
      isOpen = !isOpen;
    }}
  >
    <span class="mark" aria-hidden="true"><Icon name={icon} size="sm" /></span>
    {#if trigger === 'row'}
      <span class="word">{triggerLabel}</span>
      <span class="chev" aria-hidden="true">›</span>
    {/if}
  </button>
</span>

{#if isOpen}
  <div class="flyout" role="dialog" aria-label={label} {id} data-scrollbar="thin" bind:this={surface}>
    <h2 class="heading">{label}</h2>
    {#if searchLabel !== undefined}
      <label class="search">
        <Icon name="search" size="sm" />
        <input type="search" aria-label={searchLabel} placeholder={searchLabel} bind:value={search} />
      </label>
    {/if}
    {@render children()}
  </div>
{/if}

<style>
  .anchor { display: contents; }
  .anchor[data-trigger='row'] { display: block; }

  .trigger {
    display: flex;
    align-items: center;
    gap: calc(var(--sp-100) - var(--bw-hairline));
    margin: 0;
    padding: 0;
    border: 0;
    border-radius: var(--layout-nav-radius);
    background: none;
    color: var(--text-subtle);
    font: inherit;
    cursor: pointer;
  }

  [data-trigger='icon'] .trigger {
    display: grid;
    flex: none;
    place-items: center;
    inline-size: var(--layout-nav-action);
    block-size: var(--layout-nav-action);
    border-radius: var(--r-xs);
  }

  [data-trigger='icon'] .trigger :global(.icon) {
    inline-size: var(--layout-nav-actionglyph);
    block-size: var(--layout-nav-actionglyph);
  }

  /* As a row: the sidebar row's measures, a leaf's twist room kept so it lines up. */
  [data-trigger='row'] .trigger {
    inline-size: 100%;
    block-size: var(--layout-nav-row);
    padding-inline: calc(var(--sp-050) + var(--sp-025) + var(--layout-nav-twist) + var(--sp-100) - var(--bw-hairline)) calc(var(--sp-050) + var(--sp-025));
    color: var(--text-primary);
    font-size: var(--fs-090);
    text-align: start;
  }

  .trigger:hover,
  .trigger[aria-expanded='true'] { background: var(--bg-surface-hover); }

  .trigger:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .mark {
    display: grid;
    place-items: center;
    flex: none;
    color: var(--text-secondary);
  }

  [data-trigger='row'] .mark { inline-size: var(--layout-nav-mark); }

  [data-trigger='row'] .mark :global(.icon) {
    inline-size: var(--layout-nav-glyph);
    block-size: var(--layout-nav-glyph);
  }

  .word { flex: 1; }

  .chev {
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    color: var(--text-subtle);
  }

  /* One inline edge for the heading, the search and the rows. The plane is the sticky section
     heads' background, so a head is the panel's own surface and never a band across it. */
  .flyout {
    --nav-plane: var(--bg-raised);
    inline-size: var(--layout-sidenav-width);
    max-block-size: min(var(--layout-panel-width), 80vh);
    overflow: auto;
    padding: var(--sp-050) calc(var(--sp-050) + var(--sp-025));
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
    color: var(--text-primary);
    animation: arrive var(--motion-entrance-duration) var(--motion-entrance-easing) both;
  }

  @keyframes arrive {
    from { opacity: 0; transform: translateY(calc(-1 * var(--sp-050))); }
    to { opacity: 1; transform: none; }
  }

  .heading {
    margin: 0;
    padding: var(--sp-100) calc(var(--sp-100) + var(--bw-hairline)) var(--sp-050);
    font-size: var(--fs-090);
    font-weight: var(--fw-semibold);
  }

  .search {
    display: flex;
    align-items: center;
    gap: calc(var(--sp-050) + var(--sp-025));
    block-size: var(--layout-nav-filter);
    margin-block: var(--sp-050);
    padding: 0 var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--layout-nav-radius);
    background: var(--bg-canvas);
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .search:focus-within {
    border-color: var(--focus-ring);
    box-shadow: inset 0 0 0 var(--bw-hairline) var(--focus-ring);
  }

  .search input {
    flex: 1;
    min-inline-size: 0;
    border: 0;
    outline: none;
    background: none;
    color: var(--text-primary);
    font: inherit;
  }

  @media (prefers-reduced-motion: reduce) {
    .flyout { animation: none; }
  }

  :global([data-motion='reduced']) .flyout { animation: none; }
</style>
