<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An overview's tile (page-system §6, §7.5): a region, not a raised card - a title, a figure or a
  // state with its mark, one line, one link. The one raised card a screen may have is the
  // overview's focal one, which `OverviewPage` draws.
  import Badge from './Badge.svelte';

  interface Props {
    title: string;
    /** *1 / 1 000*: a figure, drawn large. */
    figure?: string;
    /** A state with its tone: *All working*, *No target*. */
    state?: { label: string; tone?: 'success' | 'warning' | 'danger' | 'info' | 'neutral' };
    /** One line. */
    line?: string;
    link?: { label: string; href: string; onnavigate?: () => void };
  }

  const { title, figure, state, line, link }: Props = $props();
</script>

<article class="summary-card">
  <h3 class="title">{title}</h3>
  {#if figure}<span class="figure">{figure}</span>{/if}
  {#if state || line}
    <p class="line">
      {#if state}<Badge tone={state.tone ?? 'neutral'}>{state.label}</Badge>{/if}
      {#if line}<span>{line}</span>{/if}
    </p>
  {/if}
  {#if link}
    <a
      class="link"
      href={link.href}
      onclick={(event) => {
        if (link.onnavigate) {
          event.preventDefault();
          link.onnavigate();
        }
      }}>{link.label}</a
    >
  {/if}
</article>

<style>
  .summary-card {
    display: grid;
    align-content: start;
    gap: var(--sp-100);
    padding: var(--sp-200) var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    min-width: 0;
  }

  .title { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .figure { font-family: var(--font-display); font-size: var(--fs-600); line-height: var(--lh-tight); }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .link { justify-self: start; color: var(--text-brand); font-size: var(--fs-100); text-decoration: none; }
  .link:hover { text-decoration: underline; }

  .link:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
