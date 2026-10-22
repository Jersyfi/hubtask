<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Why a whole area cannot be changed, said once at its top (design-system.md §4, UC-WRK-32
  // check 4): an archived collection, a role that reads, a feature the plan does not carry.
  //
  // **Once, and first.** The reason used to be repeated under every row and every field of such
  // an area; here it is one line above it, and the rows and fields below say nothing - their
  // controls are absent or simply not offered, which is the caller's (P-05).
  //
  // **Not a `Banner`**, which reports something that happened and can be dismissed, and **not a
  // `Callout`**, which is documentation, true whenever read. This is the state of this area now:
  // it stays as long as the state does, and goes with it. It may carry the one thing that changes
  // the state - *Restore*, *Ask the owner* - as an action.
  //
  // It is drawn with the area from the first render, so it never arrives over content a reader is
  // already looking at.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';

  interface Props {
    /** Why the area cannot be changed and, where there is one, what to do. Resolved text (ADR-0011). */
    text: string;
    /** The mark before it. A lock by default: the area is closed, not broken. */
    icon?: IconName;
    /** The one action that changes the state, drawn by the caller - a `Button`. */
    action?: Snippet;
    /**
     * An id for the sentence, so the controls of the area that stay drawn - a completion box that
     * still shows done - can name it as their description rather than repeating it (reason.ts,
     * `provideReasonGroup`).
     */
    textId?: string;
  }

  const { text, icon = 'lock', action, textId }: Props = $props();
</script>

<div class="notice" role="note">
  <span class="mark" aria-hidden="true"><Icon name={icon} size="sm" /></span>
  <p class="text" id={textId}>{text}</p>
  {#if action}<div class="action">{@render action()}</div>{/if}
</div>

<style>
  .notice {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100) var(--sp-150);
    padding: var(--sp-100) var(--sp-150);
    /* Rule 3: a surface and a mark, never a tint alone. The neutral status surface: this is a
       state, not a warning. */
    border: var(--bw-hairline) solid var(--status-neutral-border);
    border-radius: var(--r-md);
    background: var(--status-neutral-surface);
    color: var(--status-neutral-text);
  }

  .mark { display: inline-flex; flex: none; }

  .text {
    flex: 1 1 24ch;
    margin: 0;
    font-size: var(--fs-075);
    text-align: start;
    overflow-wrap: anywhere;
  }

  .action { flex: none; }
</style>
