<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The goals an entry serves, beside its links (UC-GOL-01 check 4): each goal the reader may read
  // with its progress, opening its page. Nothing is drawn where the entry serves none. Mounted by
  // the entry's page only where the goals module is on; linking happens on the goal's page.

  import { ProgressBar } from '@hubtask/design-system/components';
  import type { GoalList } from '@hubtask/sync-engine';

  import { entryGoalsPath, percentOf, percentText } from '../data/goals.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    itemId: string;
  }

  const { itemId }: Props = $props();

  // svelte-ignore state_referenced_locally
  const served = resource<GoalList>(entryGoalsPath(itemId));
  const list = $derived(served.state.status === 'ready' ? served.state.data.data : []);
</script>

{#if list.length > 0}
  <section class="goals" aria-labelledby={`goals-${itemId}`}>
    <h2 id={`goals-${itemId}`} class="heading">{t('app.goals.served', { count: list.length })}</h2>
    <ul>
      {#each list as goal (goal.id)}
        <li>
          <a href={`/goals/${goal.id}`}>{goal.title}</a>
          {#if goal.progress !== null}
            <ProgressBar size="sm" label={t('app.goals.progress_of', { title: goal.title })} value={percentOf(goal.progress)} valueLabel={t('app.goals.percent', { percent: percentText(goal.progress) })} />
          {/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .goals { display: flex; flex-direction: column; gap: var(--sp-050); }
  .heading { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-secondary); }
  ul { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }
</style>
