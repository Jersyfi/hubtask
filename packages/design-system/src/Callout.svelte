<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A note in prose that is true whenever it is read.
  //
  // Not a `Banner`: a banner announces something about *this page, now* - a degradation, a
  // maturity stage - and is a live region for it. A callout is documentation: "an idempotency key
  // is required on every mutation", true on Tuesday and true in print, read in the flow of the
  // text it sits in and announced by nobody. So `role="note"`, no dismissal, no timeout, and the
  // tone says what kind of note it is - rule 3's mark beside the colour, as everywhere.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import { STATUS_ICON, type StatusTone } from './control.ts';
  import type { IconName } from './icons/index.ts';

  interface Props {
    tone?: StatusTone;
    /** The one line a reader takes away. Resolved text (ADR-0011). */
    title?: string;
    icon?: IconName;
    children: Snippet;
  }

  const { tone = 'info', title, icon, children }: Props = $props();
</script>

<aside class="note" data-tone={tone} role="note">
  <span class="mark"><Icon name={icon ?? STATUS_ICON[tone]} size="sm" /></span>
  <div class="body">
    {#if title}<p class="title">{title}</p>{/if}
    <div class="text">{@render children()}</div>
  </div>
</aside>

<style>
  .note {
    display: flex;
    align-items: start;
    gap: var(--sp-150);
    padding: var(--sp-150) var(--sp-200);
    border-inline-start: var(--bw-thick) solid var(--note-accent);
    border-radius: var(--r-sm);
    background: var(--note-surface);
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
  }

  /* A status as a surface (ADR-0061): the tone's surface under the prose, its accent on the
     rule and the mark. Rule 3: the mark and the rule carry the tone; the prose stays the reading
     colour. */
  .mark { display: inline-flex; margin-block-start: var(--sp-025); color: var(--note-accent); }
  .note[data-tone='info'] { --note-surface: var(--status-info-surface); --note-accent: var(--status-info-accent); }
  .note[data-tone='success'] { --note-surface: var(--status-success-surface); --note-accent: var(--status-success-accent); }
  .note[data-tone='warning'] { --note-surface: var(--status-warning-surface); --note-accent: var(--status-warning-accent); }
  .note[data-tone='danger'] { --note-surface: var(--status-danger-surface); --note-accent: var(--status-danger-accent); }

  .body { display: flex; flex-direction: column; gap: var(--sp-050); flex: 1; min-width: 0; }
  .title { margin: 0; font-weight: var(--fw-medium); overflow-wrap: anywhere; }
  .text { max-width: 80ch; color: var(--text-secondary); overflow-wrap: anywhere; }
  .text :global(p) { margin: 0; }
  .text :global(p + p) { margin-block-start: var(--sp-100); }
  .text :global(code) { font-family: var(--font-mono); font-size: var(--fs-075); }
</style>
