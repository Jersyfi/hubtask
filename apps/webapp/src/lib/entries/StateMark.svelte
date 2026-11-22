<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one press of the state control in a row and on a card (UC-WRK-05 check 11, E5): a checkbox
  // where the press completes or reopens - at L0, L1, while a workflow is paused, and at L2 where the
  // next step lands in done -, an arrow button named by its step where it moves on without
  // completing. Where no step leads on it is drawn, not pressable, with the reason. It goes through
  // `takeMark`, the same door the swipe and the menu's *advance* take.
  import { Checkbox, IconButton } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import type { Mark } from './statechip.ts';
  import { takeMark } from './statetake.ts';

  interface Props {
    item: WorkItem;
    /** Undefined: no step leads on from where the entry stands. */
    mark: Mark | undefined;
    /** Present: the reader may not change the entry now, and this says why. */
    disabledReason?: string;
    /** Where the entry stands, for the reason a mark that leads nowhere gives. */
    stateLabel: string;
    onfailure?: (message: string) => void;
  }

  const { item, mark, disabledReason, stateLabel, onfailure }: Props = $props();

  const isCompleted = $derived(item.completion?.is_completed ?? false);
  let isBusy = $state(false);

  async function press() {
    if (!mark || isBusy) return;
    isBusy = true;
    try {
      await takeMark(item, mark);
    } catch (error) {
      onfailure?.(renderProblem(error as never, messages).message);
    } finally {
      isBusy = false;
    }
  }
</script>

<span class="mark" data-state-mark={mark?.kind ?? 'none'}>
  {#if mark?.kind === 'step' && !mark.completes && !mark.reopens}
    <IconButton
      icon="arrow-right"
      size="sm"
      tone="secondary"
      label={t('app.entries.move_on', { title: item.title, step: mark.choice.label })}
      {disabledReason}
      {isBusy}
      onclick={() => void press()}
    />
  {:else}
    <Checkbox
      label={t(isCompleted ? 'app.entries.reopen' : 'app.entries.complete', { title: item.title })}
      isLabelHidden
      checked={isCompleted}
      disabledReason={disabledReason ?? (mark ? undefined : t('app.entries.no_step_on', { status: stateLabel }))}
      onchange={() => void press()}
    />
  {/if}
</span>

<style>
  .mark { display: inline-flex; flex: none; align-items: center; }
</style>
