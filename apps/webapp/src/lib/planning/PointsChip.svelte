<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's points as scenes sprint and backlog draw them: *3 P*, or *estimate* dashed where
  // there is none. A press makes the chip a field; Enter or leaving it saves, Escape keeps what was
  // (UC-PLN-07 check 5) - no per-row *Save* (user-experience.md §7.5). One the reader may not
  // change is the chip alone, and nothing where there is no estimate (P-05).
  import { tick } from 'svelte';
  import { Input } from '@hubtask/design-system/components';
  import type { Estimate } from '@hubtask/sync-engine';

  import { estimateOf, estimateText, type EstimateInput } from '../data/sprints.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The entry's title, which names the chip and the field. */
    title: string;
    estimate: Estimate | null | undefined;
    isEditable: boolean;
    onsave: (input: EstimateInput | null) => void;
  }

  const { title, estimate, isEditable, onsave }: Props = $props();

  let isEditing = $state(false);
  let draft = $state('');
  let error = $state<string | undefined>(undefined);
  let field = $state<HTMLElement | undefined>(undefined);

  const word = $derived(
    !estimate ? '' : estimate.unit === 'SIZE' ? (estimate.size ?? '') : t('app.sprints.column_value', { value: String(estimate.value), unit: t(`app.sprints.unit_short_${estimate.unit}`) }),
  );

  function begin() {
    draft = estimateText(estimate);
    error = undefined;
    isEditing = true;
    void tick().then(() => field?.querySelector('input')?.focus());
  }
  function save() {
    if (!isEditing) return;
    const input = estimateOf(draft);
    if (input === undefined) {
      error = t('estimates.value_out_of_range', { maximum: '10000' });
      return;
    }
    isEditing = false;
    if (draft.trim() !== estimateText(estimate)) onsave(input);
  }
</script>

{#if isEditing}
  <span class="field" bind:this={field}>
    <Input
      label={t('app.sprints.estimate_of', { title })}
      isLabelHidden
      size="sm"
      placeholder={t('app.sprints.estimate_placeholder')}
      value={draft}
      {error}
      oninput={(event: Event) => (draft = (event.currentTarget as HTMLInputElement).value)}
      onchange={save}
      onkeydown={(event: KeyboardEvent) => { if (event.key === 'Escape') isEditing = false; }}
    />
  </span>
{:else if isEditable}
  <button type="button" class="chip" data-points-chip data-estimate={estimate ? undefined : 'none'}
    aria-label={estimate ? t('app.sprints.estimate_change', { title, value: word }) : t('app.sprints.estimate_add', { title })}
    onclick={begin}>{estimate ? word : t('app.sprints.estimate_chip')}</button>
{:else if estimate}
  <span class="chip" data-points-chip>{word}</span>
{/if}

<style>
  .chip {
    display: inline-flex;
    flex: none;
    align-items: center;
    min-block-size: var(--sp-300);
    padding: 0 var(--sp-100);
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    white-space: nowrap;
  }
  button.chip { cursor: pointer; }
  button.chip:hover { color: var(--text-primary); }
  button.chip:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .chip[data-estimate='none'] { border-color: var(--border-strong); border-style: dashed; background: transparent; }
  .field :global(input) { inline-size: var(--sp-1000); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target beyond its drawn box (P-2). */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    button.chip { position: relative; }
    button.chip::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
