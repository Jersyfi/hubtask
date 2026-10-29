<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's alerts (UC-AST-08 checks 6, 7): what its intakes were told, the latest first, each
  // problem once with how often it fired, the CI its host names and the entry opened for it;
  // narrowed to firing or resolved. Read only - an alert is the monitoring program's to resolve.

  import { untrack } from 'svelte';
  import { Badge, Banner, EmptyState, Select, Spinner, type SelectOption } from '@hubtask/design-system/components';
  import type { CiAlert, CiAlertList, CiConnector, CiConnectorList } from '@hubtask/sync-engine';

  import { alertTone, alertsPath, connectorsPath } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
  }

  const { hubId }: Props = $props();

  let narrowed = $state<'' | CiAlert['status']>('');
  let alerts = $state<readonly CiAlert[] | undefined>(undefined);
  let connectors = $state<readonly CiConnector[]>([]);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    const status = narrowed || undefined;
    return untrack(() => {
      alerts = undefined;
      const stops = [
        engine.subscribe<CiAlertList>({ path: alertsPath(at, status) }, (state) => {
          if (state.status === 'ready') alerts = state.data.data;
          if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
        }),
        engine.subscribe<CiConnectorList>({ path: connectorsPath(at) }, (state) => {
          if (state.status === 'ready') connectors = state.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  const when = (at: string) => formatDateTime(at, messages.locale);
  const intakeOf = (alert: CiAlert) => connectors.find((each) => each.id === alert.connector_id)?.key ?? '';
  const filters = $derived<SelectOption[]>([
    { value: '', label: t('app.assets.alerts_all') },
    { value: 'FIRING', label: t('app.assets.alert_status.FIRING') },
    { value: 'RESOLVED', label: t('app.assets.alert_status.RESOLVED') },
  ]);
</script>

<div class="alerts">
  <Select label={t('app.assets.alerts_narrow')} size="sm" options={filters} bind:value={narrowed} />
  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !alerts}
    <Spinner label={t('app.assets.loading')} />
  {:else if alerts.length === 0}
    <EmptyState kind={narrowed ? 'filtered' : 'unused'} title={t('app.assets.alerts_empty')} icon="bell" />
  {:else}
    <ul class="rows">
      {#each alerts as alert (alert.id)}
        <li class="row" data-alert={alert.key}>
          <span class="what">
            <span class="line">
              <Badge size="sm" tone={alertTone(alert)}>{t(`app.assets.alert_status.${alert.status}`)}</Badge>
              <strong>{alert.title || alert.key}</strong>
            </span>
            <span class="meta">
              {#if alert.severity}{alert.severity} · {/if}{t('app.assets.alert_seen', { count: alert.count, first: when(alert.first_at), last: when(alert.last_at) })}
              {#if alert.resolved_at} · {t('app.assets.alert_resolved_at', { when: when(alert.resolved_at) })}{/if}
              {#if intakeOf(alert)} · {t('app.assets.alert_from', { intake: intakeOf(alert) })}{/if}
            </span>
          </span>
          <span class="links">
            {#if alert.ci_id}<a href={`/cis/${alert.ci_id}`}>{t('app.assets.alert_ci')}</a>{:else}<span class="meta">{t('app.assets.alert_no_ci')}</span>{/if}
            {#if alert.entry_id}<a href={`/items/${alert.entry_id}`}>{t('app.assets.alert_entry')}</a>{/if}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .alerts { display: grid; gap: var(--sp-150); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .what, .links { display: grid; gap: var(--sp-025); }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
  .links { justify-items: end; font-size: var(--fs-075); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
