<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The views of one place - a product's home, a hub, a collection - as one strip under its head
  // (user-experience.md §2.4, §5.1). It switches routes, so it is a navigation of links with
  // `aria-current`, not `Tabs`, which own the panel they reveal: the same reason `BottomBar` is
  // not `Tabs`. One tab stop, the arrows between the links (Home and End to either end). Narrow,
  // it scrolls sideways inside itself with the current link kept in view, and never wraps.
  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';

  export interface PlaceTab {
    readonly id: string;
    /** Resolved text (ADR-0011): one or two words. */
    readonly label: string;
    readonly href: string;
    readonly icon?: IconName;
    /** A count worth seeing before opening it. */
    readonly count?: number;
  }

  interface Props {
    /** What the strip is called, for the landmark: the place's name. */
    label: string;
    tabs: readonly PlaceTab[];
    /** The tab the reader is on. Announced, not only coloured (rule 3). */
    current?: string;
    onnavigate?: (id: string) => void;
  }

  const { label, tabs, current, onnavigate }: Props = $props();

  let list = $state<HTMLUListElement | undefined>(undefined);
  /** The one tab stop: the current tab, or the first while none is current. */
  const stop = $derived(tabs.some((tab) => tab.id === current) ? current : tabs[0]?.id);

  // The current tab kept in view where the strip scrolls; `nearest` moves nothing that is already
  // visible, so a wide strip never jumps.
  $effect(() => {
    void current;
    list?.querySelector<HTMLElement>('[aria-current="page"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  });

  function walk(event: KeyboardEvent) {
    const links = [...(list?.querySelectorAll<HTMLAnchorElement>('a') ?? [])];
    const at = links.indexOf(document.activeElement as HTMLAnchorElement);
    if (at < 0 || links.length === 0) return;
    const rtl = getComputedStyle(event.currentTarget as Element).direction === 'rtl';
    const forward = rtl ? 'ArrowLeft' : 'ArrowRight';
    const back = rtl ? 'ArrowRight' : 'ArrowLeft';
    let next = at;
    if (event.key === forward) next = (at + 1) % links.length;
    else if (event.key === back) next = (at - 1 + links.length) % links.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = links.length - 1;
    else return;
    event.preventDefault();
    links[next]?.focus();
  }
</script>

<nav class="strip" aria-label={label}>
  <!-- The keys move focus between the links of one strip; the links stay the controls. -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <ul bind:this={list} onkeydown={walk}>
    {#each tabs as tab (tab.id)}
      <li>
        <a
          href={tab.href}
          aria-current={tab.id === current ? 'page' : undefined}
          tabindex={tab.id === stop ? 0 : -1}
          onclick={(event) => {
            if (!onnavigate || event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return;
            event.preventDefault();
            onnavigate(tab.id);
          }}
        >
          {#if tab.icon}<Icon name={tab.icon} size="sm" />{/if}
          <span class="word">{tab.label}</span>
          {#if tab.count}<span class="count">{tab.count}</span>{/if}
        </a>
      </li>
    {/each}
  </ul>
</nav>

<style>
  .strip {
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    min-inline-size: 0;
  }

  ul {
    display: flex;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    list-style: none;
    overflow-x: auto;
    scrollbar-width: none;
  }

  li { flex: none; display: flex; }

  a {
    margin: var(--sp-025);
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-md-min);
    padding: var(--sp-100) var(--sp-200);
    color: var(--text-secondary);
    font: inherit;
    text-decoration: none;
    white-space: nowrap;
    border-block-end: var(--bw-thick) solid transparent;
    border-radius: var(--r-md) var(--r-md) 0 0;
    transition: color var(--motion-state-duration) var(--motion-state-easing);
  }

  a:hover { color: var(--text-primary); background: var(--bg-surface-hover); }

  /* Rule 3: the current tab is the colour, the weight and the bar under it, and `aria-current`
     says it without any of them. */
  a[aria-current='page'] {
    color: var(--text-brand);
    font-weight: var(--fw-medium);
    border-block-end-color: var(--accent-primary);
  }

  a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .count {
    min-inline-size: var(--sp-250);
    padding: 0 var(--sp-050);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    text-align: center;
  }

  @media (prefers-reduced-motion: reduce) {
    a { transition: none; }
  }
  :global([data-motion='reduced']) a { transition: none; }
</style>
