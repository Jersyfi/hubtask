<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's SLA goals as chips (UC-SVC-02 check 6): each goal's standing and its moment - due,
  // met, breached or standing still - toned as the requester's portal tones them. Absent where no
  // policy measures the entry.

  import { untrack } from 'svelte';
  import { Badge } from '@hubtask/design-system/components';
  import type { EntrySlaList } from '@hubtask/sync-engine';

  import { entrySlaPath } from '../data/desk.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { goalTone } from '../portal/portal.ts';

  const { itemId }: { itemId: string } = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<EntrySlaList>(untrack(() => entrySlaPath(itemId)));
  const goals = $derived(read.state.status === 'ready' ? read.state.data.data : []);
  const at = (iso: string | null) => (iso ? formatDateTime(iso, messages.locale) : '');
  const now = new Date();
</script>

{#if goals.length > 0}
  <ul class="goals" aria-label={t('app.desk.sla_label')} data-entry-sla>
    {#each goals as goal (goal.goal)}
      <li>
        <Badge tone={goalTone(goal, now)} size="sm">
          {t(`app.portal.goal.${goal.goal}`)} ·
          {#if goal.standing === 'MET'}{t('app.portal.goal_met', { at: at(goal.met_at) })}
          {:else if goal.standing === 'BREACHED'}{t('app.portal.goal_breached', { at: at(goal.breached_at) })}
          {:else if goal.standing === 'PAUSED'}{t('app.desk.goal_paused')}
          {:else}{t('app.portal.goal_due', { at: at(goal.due_at) })}{/if}
        </Badge>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .goals { margin: 0; padding: 0; list-style: none; display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
