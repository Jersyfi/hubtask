<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The outline that writes itself (S-3): the page's headings, handed in by the caller, as a list
  // beside the reading column that shows where one is. The heading in view is the topmost whose
  // top has passed the upper third of the screen; pressing one scrolls to it and moves the focus.
  interface Heading {
    /** The id of the heading's element in the page. */
    readonly id: string;
    readonly label: string;
    /** 2 or 3: a level-3 heading is drawn indented under its level-2 one. */
    readonly level: number;
  }

  interface Props {
    /** *On this page*: the list's heading and the navigation's name. */
    label: string;
    headings: readonly Heading[];
    /** The heading in view. */
    current?: string;
    onselect?: (id: string) => void;
    /** Finds a heading's element by its id; by default the document's element of that id. */
    locate?: (id: string) => HTMLElement | null;
  }

  let { label, headings, current = $bindable(headings[0]?.id), onselect, locate = (id) => document.getElementById(id) }: Props = $props();

  function jump(id: string) {
    current = id;
    onselect?.(id);
    const target = typeof document === 'undefined' ? null : locate(id);
    if (!target) return;
    target.scrollIntoView({ block: 'start' });
    if (!target.hasAttribute('tabindex')) target.setAttribute('tabindex', '-1');
    target.focus({ preventScroll: true });
  }

  $effect(() => {
    if (typeof window === 'undefined' || headings.length === 0) return;
    const follow = () => {
      const line = window.innerHeight / 3;
      let seen: string | undefined = headings[0]?.id;
      for (const heading of headings) {
        const element = locate(heading.id);
        if (element && element.getBoundingClientRect().top <= line) seen = heading.id;
      }
      if (seen) current = seen;
    };
    follow();
    window.addEventListener('scroll', follow, { passive: true, capture: true });
    return () => window.removeEventListener('scroll', follow, { capture: true });
  });
</script>

<nav class="outline" aria-labelledby="page-outline-label" data-page-outline>
  <h2 class="label" id="page-outline-label">{label}</h2>
  <ul>
    {#each headings as heading (heading.id)}
      <li data-level={heading.level >= 3 ? '3' : '2'}>
        <a
          href={`#${heading.id}`}
          aria-current={heading.id === current ? 'location' : undefined}
          onclick={(event) => {
            event.preventDefault();
            jump(heading.id);
          }}
        >{heading.label}</a>
      </li>
    {/each}
  </ul>
</nav>

<style>
  .outline { display: grid; gap: var(--sp-100); }

  .label {
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  ul {
    display: grid;
    margin: 0;
    padding: 0;
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    list-style: none;
  }

  a {
    display: block;
    margin-inline-start: calc(-1 * var(--bw-hairline));
    padding: var(--sp-050) var(--sp-100);
    border-inline-start: var(--bw-thick) solid transparent;
    color: var(--text-secondary);
    font-size: var(--fs-090);
    text-decoration: none;
    overflow-wrap: anywhere;
  }

  li[data-level='3'] a { padding-inline-start: var(--sp-250); }
  a:hover { color: var(--text-primary); }

  a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  a[aria-current] {
    border-inline-start-color: var(--accent-primary);
    color: var(--text-brand);
    font-weight: var(--fw-medium);
  }
</style>
