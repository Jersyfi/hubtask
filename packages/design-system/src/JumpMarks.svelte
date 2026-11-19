<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Jump marks with counts under an object's head (ET-2): *Description · Parts 2/8 · Links 7 ·
  // Files 9 · Conversation 13*, staying on top while the page scrolls. On a phone the same marks
  // are the switch between the sections (§7): pressing one hands its id to the caller, which shows
  // that section alone, instead of scrolling to it.
  import { isCompactWidth } from './media.ts';

  interface Mark {
    /** The id of the section it jumps to. */
    readonly id: string;
    readonly label: string;
    /** *2/8*, *13*: drawn quieter after the word, and part of the mark's name. */
    readonly count?: string;
  }

  interface Props {
    /** The navigation's name: *Sections of TAMA-18*. */
    label: string;
    marks: readonly Mark[];
    /** The section in view - or, on a phone, the one shown. */
    current?: string;
    onselect?: (id: string) => void;
  }

  let { label, marks, current = $bindable(marks[0]?.id), onselect }: Props = $props();

  let isJumping = false;

  function jump(id: string) {
    current = id;
    onselect?.(id);
    if (isCompactWidth()) return;
    const target = typeof document === 'undefined' ? null : document.getElementById(id);
    if (!target) return;
    isJumping = true;
    target.scrollIntoView({ block: 'start' });
    target.focus({ preventScroll: true });
    setTimeout(() => (isJumping = false), 0);
  }

  // The mark follows the reading: the topmost section whose head has passed under the marks.
  $effect(() => {
    if (typeof IntersectionObserver === 'undefined') return;
    const targets = marks.map((mark) => document.getElementById(mark.id)).filter((node): node is HTMLElement => node !== null);
    if (targets.length === 0) return;
    const seen = new Map<string, boolean>();
    const observer = new IntersectionObserver((entries) => {
      if (isJumping || isCompactWidth()) return;
      for (const entry of entries) seen.set(entry.target.id, entry.isIntersecting);
      const first = marks.find((mark) => seen.get(mark.id));
      if (first) current = first.id;
    });
    for (const target of targets) observer.observe(target);
    return () => observer.disconnect();
  });
</script>

<nav class="marks" aria-label={label}>
  <ul>
    {#each marks as mark (mark.id)}
      <li>
        <a
          href={`#${mark.id}`}
          class="mark"
          aria-current={mark.id === current ? 'location' : undefined}
          onclick={(event) => {
            event.preventDefault();
            jump(mark.id);
          }}
        >
          {mark.label}{#if mark.count}<span class="count">{mark.count}</span>{/if}
        </a>
      </li>
    {/each}
  </ul>
</nav>

<style>
  .marks {
    position: sticky;
    inset-block-start: var(--page-sticky-top, var(--layout-appbar-height));
    z-index: var(--z-sticky);
    padding-block: var(--sp-100);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    background: var(--bg-canvas);
  }

  ul {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .mark {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--sp-400);
    padding: var(--sp-050) calc(var(--sp-100) + var(--sp-025));
    border-radius: var(--r-full);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    text-decoration: none;
    white-space: nowrap;
  }

  .mark:hover { background: var(--bg-surface-hover); }

  .mark:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .mark[aria-current] {
    background: var(--accent-primary-subtle);
    color: var(--text-brand);
    font-weight: var(--fw-medium);
  }

  .count { color: var(--text-subtle); }

  /* A phone: one row that scrolls sideways, a thumb's target each - the switch between sections. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    ul { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; }
    .mark { min-block-size: var(--layout-nav-touchrow); }
  }
</style>
