<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The ✦ beside one subject of an entry: the questions AI can be asked about it (UC-AI-04 check 6).
  //
  // A menu, also for one question, because its head is where a nearly spent budget is said before
  // anything is asked (check 9) - and a question that cannot be asked now stands in it with its
  // reason, inside the overlay, so nothing on the page moves (design-system.md §4). Absent when
  // nothing may be asked here: AI off, a reader at a subject that only writing asks about.

  import { untrack } from 'svelte';

  import { IconButton, Menu } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { ASK_CODE, isBudgetNearlySpent, offersAt, type Ask, type BudgetState, type Place } from '../data/aiplaces.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { entryAsks } from '../data/entryasks.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderRefusal } from '../problem.ts';

  interface Props {
    item: WorkItem;
    place: Place;
    mayChange: boolean;
    hasComments?: boolean;
    choices?: boolean | 'reading';
    /** The questions whose answer already stands here, and which proposal it is. */
    standing?: Readonly<Partial<Record<Ask, string>>>;
    /** Read in another language: the caller opens the translation under the head. */
    ontranslate?: () => void;
  }

  const { item, place, mayChange, hasComments = false, choices = true, standing = {}, ontranslate }: Props = $props();

  const BUDGET_PATH = '/ai-budget-state';
  // One read for every ✦ of the page: the engine holds one state per path.
  const budget = resource<BudgetState>(untrack(() => BUDGET_PATH));

  const offers = $derived(
    offersAt(place, {
      features: manifest.value?.features,
      item,
      mayChange,
      hasComments,
      choices,
      budget: budget.state.status === 'ready' ? budget.state.data : undefined,
      standing,
    }),
  );

  const items = $derived(
    offers.map((offer) => ({
      id: offer.ask,
      label: offer.standing ? t('app.ai.show_standing') : t(ASK_CODE[offer.ask]),
      disabledReason: offer.reason ? renderRefusal(offer.reason, messages) : undefined,
    })),
  );

  const nearlySpent = $derived(isBudgetNearlySpent(budget.state.status === 'ready' ? budget.state.data : undefined));

  const LABEL_CODE: Readonly<Record<Place, string>> = {
    title: 'app.ai.menu_title',
    notes: 'app.ai.menu_notes',
    due: 'app.ai.menu_due',
    labels: 'app.ai.menu_labels',
    steps: 'app.ai.menu_steps',
    comments: 'app.ai.menu_comments',
  };

  function select(id: string) {
    const offer = offers.find((each) => each.ask === id);
    if (!offer) return;
    if (offer.standing) {
      // Where the answer already stands, the ✦ leads to it rather than asking again.
      document.getElementById(`proposal-${offer.standing}`)?.focus();
      return;
    }
    if (offer.ask === 'translate') {
      ontranslate?.();
      return;
    }
    void entryAsks.ask(item.id, offer.ask).finally(() => {
      // What the question spent is part of the next answer about the budget.
      void engine.refresh({ path: BUDGET_PATH }).catch(() => undefined);
    });
  }
</script>

{#if items.length > 0}
  <!-- Marked as AI, as every AI element is, so that an installation without it can be checked for
       none (apps/webapp/build/residue.test.js). -->
  <span class="ai-ask" data-ai data-place={place}>
    <Menu label={t(LABEL_CODE[place])} {items} onselect={select}>
      {#snippet trigger(props)}
        <IconButton icon="sparkles" size="sm" label={t(LABEL_CODE[place])} {...props} />
      {/snippet}
      {#snippet head()}
        {#if nearlySpent}<p class="budget">{t('app.ai.budget_nearly_spent')}</p>{/if}
      {/snippet}
    </Menu>
  </span>
{/if}

<style>
  .budget { margin: 0; padding: var(--sp-050) var(--sp-100); color: var(--text-secondary); font-size: var(--fs-075); max-width: 40ch; }
</style>
