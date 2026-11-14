<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's alerts (UC-AST-08 checks 6, 7; work-surface §6.10): what its intakes were told, the
  // latest first, as a calm table - each problem once with its object, the entry opened for it,
  // since when, how often and whether it still fires -, narrowed to firing or resolved. Read only - an alert is the monitoring program's to resolve. A
  // pull's own run alert (research C8) carries a message code where a monitoring alert has words.
  // An empty list says where alerts come from and, to whoever may, offers to set an intake up;
  // narrowed to firing and empty, it says nothing is firing (user-experience.md §5.5).

  import { untrack } from 'svelte';
  import { DataTable, Select, Skeleton, StateLozenge, Toolbar, type DataColumn, type SelectOption } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { CiAlert, CiAlertList, CiConnector, CiConnectorList } from '@hubtask/sync-engine';

  import { RUN_ALERT_KEY, alertTone, alertsPath, cmdb, connectorsPath } from '../data/cmdb.svelte.ts';
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
  /** The alert's facts under its title: severity, its last report, resolved, from which intake. */
  const factsOf = (alert: CiAlert) => [
    ...(alert.severity && !isRun(alert) ? [alert.severity] : []),
    t('app.assets.alert_seen', { count: alert.count, first: when(alert.first_at), last: when(alert.last_at) }),
    ...(alert.resolved_at ? [t('app.assets.alert_resolved_at', { when: when(alert.resolved_at) })] : []),
    ...(intakeOf(alert) && !isRun(alert) ? [t('app.assets.alert_from', { intake: intakeOf(alert) })] : []),
  ].join(' · ');
  // The objects' names, read one by one: an alert carries only the object's id.
  let names = $state<Record<string, string>>({});
  $effect(() => {
    const ids = [...new Set((alerts ?? []).map((each) => each.ci_id).filter((id): id is string => Boolean(id)))].slice(0, 50);
    untrack(() => {
      for (const id of ids) {
        if (id in names) continue;
        names = { ...names, [id]: '' };
        void cmdb.read(id).then((ci) => {
          if (ci) names = { ...names, [id]: ci.name };
        });
      }
    });
  });
  const columns = $derived<DataColumn[]>([
    { id: 'alert', label: t('app.assets.alert_col.alert'), priority: 1 },
    { id: 'object', label: t('app.assets.alert_ci'), priority: 2 },
    { id: 'entry', label: t('app.assets.alert_entry'), priority: 3 },
    { id: 'since', label: t('app.assets.alert_col.since'), priority: 3 },
    { id: 'count', label: t('app.assets.alert_col.count'), priority: 3, align: 'end' },
    { id: 'state', label: t('app.assets.alert_col.state'), priority: 1, isStatus: true },
  ]);
  const filters = $derived<SelectOption[]>([
    { value: '', label: t('app.assets.alerts_all') },
    { value: 'FIRING', label: t('app.assets.alert_status.FIRING') },
    { value: 'RESOLVED', label: t('app.assets.alert_status.RESOLVED') },
  ]);
</script>

<div class="alerts">
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !alerts}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if alerts.length === 0 && !narrowed}
    <ProductEmpty product="ASSETS" place={{ type: 'HUB', id: hubId }}
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
      <DataTable label={t('app.assets.section.alerts')} kind="work" {columns} rows={alerts} key={(alert: CiAlert) => alert.id} titleText={titleOf}>
        {#snippet cell(alert: CiAlert, column: DataColumn)}
          {#if column.id === 'alert'}
            <span class="what" data-alert={alert.key} data-status={alert.status}>
              <strong class="title">{titleOf(alert)}</strong>
              {#if isRun(alert) && alert.title && messages.has(alert.title)}<span>{t(alert.title)}</span>{/if}
              <span class="meta">{factsOf(alert)}</span>
            </span>
          {:else if column.id === 'object'}
            {#if alert.ci_id}<a href={`/cis/${alert.ci_id}`}>{names[alert.ci_id] || t('app.assets.alert_ci')}</a>{:else if !isRun(alert)}<span class="meta">{t('app.assets.alert_no_ci')}</span>{:else}—{/if}
          {:else if column.id === 'entry'}
            {#if alert.entry_id}<a href={`/items/${alert.entry_id}`}>{t('app.assets.alert_entry')}</a>{:else}—{/if}
          {:else if column.id === 'since'}{when(alert.first_at)}
          {:else if column.id === 'count'}{alert.count}
          {:else}
            <StateLozenge label={t(`app.assets.alert_status.${alert.status}`)} tone={alertTone(alert)} icon={alert.status === 'FIRING' ? 'bell' : 'circle-check'} />
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  {/if}
</div>

<style>
  .alerts { display: grid; gap: var(--sp-150); }
  /* One line, as a work row is (WB-4): the title, then what it says and its facts, quieter. */
  .what { display: inline; }
  .what > :global(* + *) { margin-inline-start: var(--sp-100); }
  a { color: var(--text-primary); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
