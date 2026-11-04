<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A queue row's SLA (user-experience.md §7.4): a bar of the time used, the words and a mark,
  // so the state never rests on colour alone (UX-7). The goal it stands for is `queueSla`'s.

  import { untrack } from 'svelte';
  import { Icon, type IconName } from '@hubtask/design-system/components';
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
  const MARK: Readonly<Record<SlaState, IconName>> = { breached: 'triangle-alert', at_risk: 'triangle-alert', running: 'clock', paused: 'pause', met: 'circle-check' };
</script>

<span class="sla" data-sla={sla?.state ?? 'none'} title={sla ? t(`app.portal.goal.${sla.goal}`) : undefined}>
  {#if sla}
    {#if sla.used !== undefined}<span class="bar" aria-hidden="true"><span class="used" data-used={Math.round(sla.used / 10) * 10}></span></span>{/if}
    <Icon name={MARK[sla.state]} size="sm" />
  {/if}
  <span class="words">{words}</span>
</span>

<style>
  .sla { display: inline-flex; align-items: center; gap: var(--sp-050); padding: var(--sp-025) var(--sp-100); border-radius: var(--r-full); white-space: nowrap; color: var(--text-secondary); font-size: var(--fs-075); font-variant-numeric: tabular-nums; }
  .bar { flex: none; inline-size: var(--sp-500); block-size: var(--sp-050); border-radius: var(--r-full); background: var(--bg-surface-sunken); overflow: hidden; }
  .used { display: block; block-size: 100%; inline-size: 0; border-radius: inherit; background: var(--accent-primary); }
  /* Width by tenths through an attribute: an inline style is refused by the CSP (ADR-0028). */
  .used[data-used='10'] { inline-size: 10%; }
  .used[data-used='20'] { inline-size: 20%; }
  .used[data-used='30'] { inline-size: 30%; }
  .used[data-used='40'] { inline-size: 40%; }
  .used[data-used='50'] { inline-size: 50%; }
  .used[data-used='60'] { inline-size: 60%; }
  .used[data-used='70'] { inline-size: 70%; }
  .used[data-used='80'] { inline-size: 80%; }
  .used[data-used='90'] { inline-size: 90%; }
  .used[data-used='100'] { inline-size: 100%; }
  /* A state that asks for the handler is drawn on its surface, as a badge is (design-system.md §6 rule 3). */
  [data-sla='at_risk'] { color: var(--status-warning-text); background: var(--status-warning-surface); }
  [data-sla='at_risk'] .used { background: var(--status-warning-accent); }
  [data-sla='breached'] { color: var(--status-danger-text); background: var(--status-danger-surface); font-weight: var(--fw-semibold); }
  [data-sla='breached'] .used { background: var(--status-danger-accent); }
  [data-sla='met'] { color: var(--status-success-text); background: var(--status-success-surface); }
</style>
