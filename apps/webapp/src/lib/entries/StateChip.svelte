<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where the entry stands at L1 and L2 (build-plan §5): a chip with the entry's column or status,
  // its menu the columns or the steps; beside the mark, the same on the list row, the board card and
  // the phone sheet. At L0 there is none - the mark alone completes. A paused workflow's chip only
  // reads and says why. Every choice goes through `takeChoice`.
  import { Menu, StateLozenge, VisuallyHidden, type MenuItem } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import type { Chip } from './statechip.ts';
  import { takeChoice } from './statetake.ts';

  interface Props {
    item: WorkItem;
    chip: Chip;
    /** Present: the reader may not change the entry now, and this says why; the chip only reads. */
    disabledReason?: string;
    /** A refusal, said where the caller says failures. */
    onfailure?: (message: string) => void;
  }

  const { item, chip, disabledReason, onfailure }: Props = $props();

  const menuItems = $derived<MenuItem[]>(
    chip.choices.map((choice) => ({ id: choice.id, label: choice.label, icon: choice.isCurrent ? ('check' as const) : undefined })),
  );

  async function choose(id: string) {
    try {
      await takeChoice(item, { id, label: chip.choices.find((each) => each.id === id)?.label ?? '' });
    } catch (error) {
      onfailure?.(renderProblem(error as never, messages).message);
    }
  }
</script>

{#if chip.pausedReason}
  <span class="chip" data-state-chip data-paused title={chip.pausedReason}><StateLozenge label={chip.label} tone="warning" icon="pause" size="sm" /><VisuallyHidden>{chip.pausedReason}</VisuallyHidden></span>
{:else if disabledReason || menuItems.length === 0}
  <span class="chip" data-state-chip title={disabledReason}><StateLozenge label={chip.label} vocabulary="category" state={chip.category} size="sm" /></span>
{:else}
  <Menu label={t('app.entries.state_menu', { title: item.title })} items={menuItems} onselect={(id) => void choose(id)}>
    {#snippet trigger(props)}
      <button type="button" class="chip" data-state-chip aria-label={t('app.entries.state_chip', { title: item.title, state: chip.label })} {...props}>
        <StateLozenge label={chip.label} vocabulary="category" state={chip.category} size="sm" />
      </button>
    {/snippet}
  </Menu>
{/if}

<style>
  .chip {
    display: inline-flex;
    flex: none;
    align-items: center;
    padding: 0;
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    font: inherit;
    cursor: pointer;
  }

  span.chip { cursor: default; }

  .chip:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target beyond its drawn box (P-2). */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    button.chip { position: relative; z-index: var(--z-raised); }
    button.chip::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
