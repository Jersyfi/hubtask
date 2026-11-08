<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The sections of a page - a settings page's sections, an editor's objects - as links, each its
  // own address, each saying its state (page-system §6, PS-9). From `expanded` up it is the list
  // beside the section, sticky at the content's top and scrolling on its own when it is longer
  // than the window (§4.3). Below, it is the page itself: one region of rows with a chevron, and
  // choosing one opens the section with a way back - which the page kind draws, not this.
  //
  // A `nav` of links, not tabs: each switches the address. One link at a time is `aria-current`;
  // the arrows, Home and End move between them as in ViewBar, Tab leaves the list.

  import type { Snippet } from 'svelte';

  import Badge from './Badge.svelte';
  import Icon from './Icon.svelte';
  import { rovingIndex } from './focus.ts';

  export interface SectionLink {
    readonly id: string;
    /** Resolved text (ADR-0011). */
    readonly label: string;
    readonly href: string;
    /** The state of the section: *Default*, *2 set here*, *Locked*, a count, *live*. */
    readonly status?: string;
    /**
     * How the state reads: `set` in the brand colour, `locked` with the lock, a status tone as a
     * `Badge`; quiet otherwise.
     */
    readonly statusTone?: 'quiet' | 'set' | 'locked' | 'success' | 'warning' | 'danger' | 'info';
  }

  interface Props {
    /** The landmark's name: *Sections of Sign-in*, *Workflows*. */
    label: string;
    sections: readonly SectionLink[];
    /** The section open now. */
    current?: string;
    /** Called instead of following the link on a plain click; middle-click still opens a tab. */
    onselect?: (id: string) => void;
    /** Above the links: a search over an editor's objects. */
    search?: Snippet;
  }

  const { label, sections, current, onselect, search }: Props = $props();

  let list = $state<HTMLElement | null>(null);

  function follow(event: MouseEvent, id: string) {
    if (!onselect || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onselect(id);
  }

  function move(event: KeyboardEvent) {
    const links = [...(list?.querySelectorAll<HTMLAnchorElement>('a') ?? [])];
    const index = links.indexOf(document.activeElement as HTMLAnchorElement);
    const next = rovingIndex(event.key, index, links.length);
    if (next === null || index < 0) return;
    event.preventDefault();
    links[next]?.focus();
  }
</script>

<nav class="section-nav" aria-label={label}>
  {#if search}<div class="search">{@render search()}</div>{/if}
  <ul class="links" bind:this={list} role="list">
    {#each sections as section (section.id)}
      <li>
        <a
          href={section.href}
          class="link"
          aria-current={section.id === current ? 'page' : undefined}
          onclick={(event) => follow(event, section.id)}
          onkeydown={move}
        >
          <span class="label">{section.label}</span>
          {#if section.status}
            {#if section.statusTone === 'success' || section.statusTone === 'warning' || section.statusTone === 'danger' || section.statusTone === 'info'}
              <span class="status"><Badge tone={section.statusTone} size="sm">{section.status}</Badge></span>
            {:else}
              <span class="status" data-tone={section.statusTone ?? 'quiet'}>
                {#if section.statusTone === 'locked'}<Icon name="lock" size="sm" />{/if}
                {section.status}
              </span>
            {/if}
          {/if}
          <span class="chevron" aria-hidden="true"><Icon name="chevron-right" size="sm" /></span>
        </a>
      </li>
    {/each}
  </ul>
</nav>

<style>
  .section-nav {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    font-size: var(--fs-100);
    min-width: 0;
  }

  .links { display: flex; flex-direction: column; gap: var(--sp-025); margin: 0; padding: 0; list-style: none; }

  .link {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    border-radius: var(--r-md);
    color: var(--text-secondary);
    text-decoration: none;
    transition: background-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .link:hover { background: var(--bg-surface-hover); }

  .link[aria-current='page'] {
    background: var(--bg-surface);
    color: var(--text-primary);
    font-weight: var(--fw-semibold);
    box-shadow: inset var(--bw-thick) 0 0 var(--accent-primary);
  }

  :global([dir='rtl']) .link[aria-current='page'] { box-shadow: inset calc(var(--bw-thick) * -1) 0 0 var(--accent-primary); }

  .link:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .label { min-width: 0; overflow-wrap: anywhere; }

  .status {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    margin-inline-start: auto;
    white-space: nowrap;
    color: var(--text-subtle);
    font-size: var(--fs-075);
    font-weight: var(--fw-regular);
  }

  .status[data-tone='set'] { color: var(--text-brand); }

  .chevron { display: none; color: var(--text-subtle); }

  /* Below `expanded` the list is the page (D-4): one region of rows a thumb can hit, a chevron
     saying each opens, no current mark - the section is not beside it to be current in. */
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .links {
      gap: 0;
      background: var(--bg-surface);
      border: var(--bw-hairline) solid var(--border-subtle);
      border-radius: var(--r-lg);
      overflow: clip;
    }
    .links li + li .link { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
    .link { border-radius: 0; padding: var(--sp-200); font-size: var(--fs-200); }
    .link[aria-current='page'],
    :global([dir='rtl']) .link[aria-current='page'] { box-shadow: none; background: transparent; font-weight: var(--fw-medium); }
    .link:focus-visible { outline-offset: calc(var(--bw-ring) * -1); }
    .chevron { display: inline-flex; }
    .status + .chevron { margin-inline-start: 0; }
    .label + .chevron { margin-inline-start: auto; }
  }

  @media (prefers-reduced-motion: reduce) {
    .link { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .link { transition-duration: var(--dur-instant); }
</style>
