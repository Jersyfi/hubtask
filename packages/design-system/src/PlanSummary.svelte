<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  export type PlanTone = 'add' | 'keep' | 'change' | 'remove' | 'refuse';

  export interface PlanLine {
    /** Resolved text: *Creates 3 collections and 248 entries*. */
    readonly text: string;
    readonly tone?: PlanTone;
  }
</script>

<script lang="ts">
  // A plan in words (user-experience.md §5.4): what an act would do - or did - one sentence a
  // change, before anybody presses the act. The sentences arrive resolved; the component knows no
  // plan and no key. Each line's kind is a mark and the sentence, never a colour alone (rule 3).
  //
  // A list, not a live region: the caller announces that a plan arrived, once, and a reader then
  // walks the lines. `reach` is the one line under them that says how far it goes.

  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';

  interface Props {
    /** What the list is, as its accessible name and its heading: *What the import would do*. */
    label: string;
    lines: readonly PlanLine[];
    /** Said instead of the list when it is empty: *Nothing would change*. */
    emptyLabel: string;
    reach?: string;
  }

  const { label, lines, emptyLabel, reach }: Props = $props();

  const MARK = {
    add: 'plus',
    keep: 'check',
    change: 'pencil',
    remove: 'minus',
    refuse: 'circle-alert',
  } as const satisfies Record<PlanTone, IconName>;
</script>

<section class="plan" aria-label={label}>
  <p class="label">{label}</p>
  {#if lines.length === 0}
    <p class="empty">{emptyLabel}</p>
  {:else}
    <ul class="lines">
      {#each lines as line, index (index)}
        <li class="line" data-tone={line.tone ?? 'add'}>
          <span class="mark"><Icon name={MARK[line.tone ?? 'add']} size="sm" /></span>
          <span class="text">{line.text}</span>
        </li>
      {/each}
    </ul>
  {/if}
  {#if reach}<p class="reach">{reach}</p>{/if}
</section>

<style>
  .plan {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150) var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  .label { margin: 0; font-weight: var(--fw-semibold); color: var(--text-primary); }

  .lines { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }

  .line { display: flex; align-items: start; gap: var(--sp-100); color: var(--text-primary); }

  .mark { display: inline-flex; margin-block-start: var(--sp-025); color: var(--plan-accent); }

  .line[data-tone='add'] { --plan-accent: var(--text-success); }
  .line[data-tone='keep'] { --plan-accent: var(--text-secondary); }
  .line[data-tone='change'] { --plan-accent: var(--text-brand); }
  .line[data-tone='remove'] { --plan-accent: var(--text-warning); }
  .line[data-tone='refuse'] { --plan-accent: var(--text-danger); }

  .text { min-width: 0; overflow-wrap: anywhere; }

  .empty,
  .reach { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
