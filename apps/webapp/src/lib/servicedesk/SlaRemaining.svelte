<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A queue row's SLA (user-experience.md §7.4): a bar of the time used with its words. The goal
  // it stands for is `queueSla`'s.

  import { untrack } from 'svelte';
  import { DeadlineBar } from '@hubtask/design-system/components';
  import type { EntrySlaList } from '@hubtask/sync-engine';

  import { entrySlaPath } from '../data/desk.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { queueSla, spanOf, type SlaState } from './sla.ts';

  const { itemId, now }: { itemId: string; now: Date } = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<EntrySlaList>(untrack(() => entrySlaPath(itemId)));
  const sla = $derived(read.state.status === 'ready' ? queueSla(read.state.data.data, now) : undefined);
  const words = $derived.by(() => {
    if (!sla) return read.state.status === 'ready' ? t('app.desk_queues.sla_none') : '';
    const span = sla.span === undefined ? undefined : spanOf(sla.span);
    return t(sla.code, { span: span ? t(span.code, span.params) : '' });
  });
  const TONE: Readonly<Record<SlaState, 'neutral' | 'warning' | 'danger' | 'success'>> = { breached: 'danger', at_risk: 'warning', running: 'neutral', paused: 'neutral', met: 'success' };
</script>

<!-- The design system's bar with its words (scene queue): how much of the time is used, what is
     left, a mark where it asks for the handler - never a colour alone (UX-7). -->
<span class="sla" data-sla={sla?.state ?? 'none'} title={sla ? t(`app.portal.goal.${sla.goal}`) : undefined}>
  {#if sla}
    <DeadlineBar share={sla.used ?? (sla.state === 'met' ? 100 : 0)} {words} tone={TONE[sla.state]} isPaused={sla.state === 'paused'} />
  {:else}
    <span class="words">{words}</span>
  {/if}
</span>

<style>
  .sla { display: inline-flex; min-inline-size: 0; max-inline-size: 100%; font-variant-numeric: tabular-nums; }
  .words { color: var(--text-subtle); font-size: var(--fs-075); white-space: nowrap; }
</style>