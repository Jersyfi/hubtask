<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's alerts (UC-AST-08 checks 6, 7): what its intakes were told, the latest first, each
  // problem once with how often it fired, the CI its host names and the entry opened for it;
  // narrowed to firing or resolved. Read only - an alert is the monitoring program's to resolve. A
  // pull's own run alert (research C8) carries a message code where a monitoring alert has words.
  // An empty list says where alerts come from and, to whoever may, offers to set an intake up;
  // narrowed to firing and empty, it says nothing is firing (user-experience.md §5.5).

  import { untrack } from 'svelte';
  import { Badge, Banner, Select, Skeleton, Toolbar, type SelectOption } from '@hubtask/design-system/components';
  import type { CiAlert, CiAlertList, CiConnector, CiConnectorList } from '@hubtask/sync-engine';

  import { RUN_ALERT_KEY, alertTone, alertsPath, connectorsPath } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import ProductEmpty from '../product/ProductEmpty.svelte';

  interface Props {
    hubId: string;
    /** Whether the reader may define connectors, so an empty list offers to set an intake up. */
    mayConnect?: boolean;
    onconnect?: () => void;
  }

  const { hubId, mayConnect = false, onconnect }: Props = $props();

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
  const isRun = (alert: CiAlert) => alert.key === RUN_ALERT_KEY;
  const titleOf = (alert: CiAlert) => (isRun(alert) ? t('app.assets.alert_run', { connector: intakeOf(alert) }) : alert.title || alert.key);
  /** The alert's facts as one line: severity, how often and when, resolved, from which intake. */
  const factsOf = (alert: CiAlert) => [
    ...(alert.severity && !isRun(alert) ? [alert.severity] : []),
    t('app.assets.alert_seen', { count: alert.count, first: when(alert.first_at), last: when(alert.last_at) }),
    ...(alert.resolved_at ? [t('app.assets.alert_resolved_at', { when: when(alert.resolved_at) })] : []),
    ...(intakeOf(alert) && !isRun(alert) ? [t('app.assets.alert_from', { intake: intakeOf(alert) })] : []),
  ].join(' · ');
  const filters = $derived<SelectOption[]>([
    { value: '', label: t('app.assets.alerts_all') },
    { value: 'FIRING', label: t('app.assets.alert_status.FIRING') },
    { value: 'RESOLVED', label: t('app.assets.alert_status.RESOLVED') },
  ]);
</script>

<div class="alerts">
  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !alerts}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if alerts.length === 0 && !narrowed}
    <ProductEmpty
      kind="unused"
      icon="bell"
      title={t('app.assets.alerts_empty')}
      description={mayConnect && onconnect ? t('app.assets.alerts_empty_body') : t('app.assets.alerts_empty_reader')}
      create={mayConnect && onconnect ? { label: t('app.assets.alerts_connect'), onclick: onconnect } : undefined}
    />
  {:else}
    <Toolbar label={t('app.assets.alerts_tools')}>
      <Select label={t('app.assets.alerts_narrow')} size="sm" options={filters} bind:value={narrowed} />
    </Toolbar>
    {#if alerts.length === 0 && narrowed === 'FIRING'}
      <ProductEmpty kind="settled" icon="circle-check" title={t('app.assets.alerts_none_firing')} />
    {:else if alerts.length === 0}
      <ProductEmpty kind="filtered" icon="funnel" title={t('app.assets.alerts_none_resolved')} clear={{ label: t('app.assets.alerts_all'), onclick: () => (narrowed = '') }} />
    {:else}
      <ul class="rows">
        {#each alerts as alert (alert.id)}
          <li class="row" data-alert={alert.key} data-status={alert.status}>
            <span class="what">
              <span class="line">
                <Badge size="sm" tone={alertTone(alert)} icon={alert.status === 'FIRING' ? 'bell' : 'circle-check'}>{t(`app.assets.alert_status.${alert.status}`)}</Badge>
                <strong class="title">{titleOf(alert)}</strong>
              </span>
              {#if isRun(alert) && alert.title && messages.has(alert.title)}<span>{t(alert.title)}</span>{/if}
              <span class="meta">{factsOf(alert)}</span>
            </span>
            <span class="links">
              {#if alert.ci_id}<a href={`/cis/${alert.ci_id}`}>{t('app.assets.alert_ci')}</a>{:else if !isRun(alert)}<span class="meta">{t('app.assets.alert_no_ci')}</span>{/if}
              {#if alert.entry_id}<a href={`/items/${alert.entry_id}`}>{t('app.assets.alert_entry')}</a>{/if}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<style>
  .alerts { display: grid; gap: var(--sp-150); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .what, .links { display: grid; gap: var(--sp-025); min-inline-size: 0; }
  .title { overflow-wrap: break-word; }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
  .links { justify-items: end; font-size: var(--fs-075); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
