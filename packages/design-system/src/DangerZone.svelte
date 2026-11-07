<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The last region of a record page (page-system §6, PS-6): each destructive act with what it
  // costs and its button, which opens the Dialog S that confirms it. The words and the danger
  // buttons say it - colour never stands alone (design-system.md §6 rule 3).
  import Button from './Button.svelte';
  import RegionHead from './_RegionHead.svelte';

  export interface DangerAct {
    readonly id: string;
    /** *Delete this group*. */
    readonly title: string;
    /** What it costs: *Its 2 people keep their own roles; the role it grants ends for them.* */
    readonly cost: string;
    /** The button: *Delete group…*. */
    readonly actionLabel: string;
    readonly disabledReason?: string;
  }

  interface Props {
    /** *Danger zone*. */
    title: string;
    acts: readonly DangerAct[];
    onact: (id: string) => void;
  }

  const { title, acts, onact }: Props = $props();
</script>

<section class="danger-zone">
  <RegionHead {title} />
  <div class="region">
    {#each acts as act (act.id)}
      <div class="line">
        <span class="text"><b class="act-title">{act.title}</b>{act.cost}</span>
        <Button tone="danger" disabledReason={act.disabledReason} onclick={() => onact(act.id)}>{act.actionLabel}</Button>
      </div>
    {/each}
  </div>
</section>

<style>
  .danger-zone { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }

  /* The danger border frames the region; the words and the danger buttons say what it is, so the
     colour never stands alone. A pair declared in build/lint-status.js and measured by the
     contrast test. */
  .region {
    border: var(--bw-hairline) solid var(--status-danger-border);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-200);
    padding: var(--sp-200) var(--sp-250);
    color: var(--text-secondary);
    font-size: var(--fs-100);
  }

  .line + .line { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .text { flex: 1 1 var(--layout-barsearch-side); min-width: 0; }
  .act-title { display: block; color: var(--text-primary); font-weight: var(--fw-semibold); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .line { padding-inline: var(--sp-200); }
  }
</style>
