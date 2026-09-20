<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The rule's runs, in short (F8-06, decision 5): the health word with its reason, the last runs
  // as a row of bars and as rows, the link to the runs page prefiltered on this rule. A run row
  // draws its recorded path onto the canvas; the whole record is the runs page's.

  import { RunStatusBadge } from '@hubtask/design-system/components';

  import { healthOf, type Health } from './probe.ts';
  import type { Run } from '../data/runs.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    ruleId: string;
    enabled: boolean;
    findings: readonly { level: string }[];
    runs: readonly Run[];
    ondraw: (run: Run) => void;
  }

  const { ruleId, enabled, findings, runs, ondraw }: Props = $props();

  const health = $derived<Health>(healthOf({ enabled, findings, runs }));
  const counted = $derived(runs.filter((run) => run.status !== 'RUNNING' && run.status !== 'WAITING'));
  const failed = $derived(counted.filter((run) => run.status === 'FAILED' || run.status === 'ABORTED_LOOP').length);

  const why = $derived.by(() => {
    switch (health) {
      case 'works':
        return t('app.flow.health_works_why', { count: counted.length });
      case 'sometimes':
        return t('app.flow.health_sometimes_why', { failed, count: counted.length });
      case 'failing':
        return t('app.flow.health_failing_why', { failed, count: counted.length });
      case 'off':
        return t('app.flow.health_off_why');
      case 'unknown':
        return t('app.flow.health_unknown_why');
      default: {
        const first = findings[0] as { code?: string; params?: Record<string, string> } | undefined;
        return first?.code && messages.has(first.code) ? t(first.code, first.params) : '';
      }
    }
  });

  const barOf = (run: Run): string =>
    run.status === 'FAILED' || run.status === 'ABORTED_LOOP' ? 'f' : run.status === 'THROTTLED' ? 't' : run.status === 'SKIPPED' ? 's' : 'ok';
</script>

<div class="panel">
  <h3>{t('app.flow.tab_runs')}</h3>
  <p class="quiet">{t('app.flow.runs_intro')}</p>

  <div class="health" data-health={health}>
    <span class="dot"></span>
    <div>
      <b>{t(`app.flow.health_${health}`)}</b>
      {#if why}<span class="why">{why}</span>{/if}
    </div>
  </div>

  {#if runs.length > 0}
    <div>
      <span class="label">{t('app.flow.runs_last')}</span>
      <div class="bars" aria-hidden="true">
        {#each [...runs].reverse().slice(-20) as run (run.id)}
          <i class={barOf(run)}></i>
        {/each}
      </div>
      <span class="hint">{t('app.flow.runs_legend')}</span>
    </div>
    <ul class="rows">
      {#each runs.slice(0, 5) as run (run.id)}
        <li>
          <button class="row" type="button" onclick={() => ondraw(run)} title={t('app.flow.runs_draw')}>
            <RunStatusBadge status={run.status as never} label={t(`app.runs.status_${run.status.toLowerCase()}`)} />
            <span class="when">{formatDateTime(run.started_at, messages.locale)}</span>
          </button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="quiet">{t('app.flow.runs_none')}</p>
  {/if}

  <a class="link" href={`/administration/runs?rule_id=${encodeURIComponent(ruleId)}`}>{t('app.flow.runs_all')} →</a>
</div>

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-200); padding: var(--sp-200); }

  h3 { margin: 0; font-family: var(--font-ui); font-size: var(--fs-200); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }

  .label { display: block; margin-block-end: var(--sp-050); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .hint { font-size: var(--fs-075); color: var(--text-subtle); }

  /* Rule 3: the word carries the state; the dot repeats it in colour and nothing depends on the dot. */
  .health { display: flex; align-items: center; gap: var(--sp-150); padding: var(--sp-150); border-radius: var(--r-md); background: var(--bg-surface-sunken); }

  .health b { display: block; font-weight: var(--fw-medium); }

  .health .why { font-size: var(--fs-075); color: var(--text-secondary); }

  .dot { flex: 0 0 auto; width: var(--sp-150); height: var(--sp-150); border-radius: var(--r-full); background: var(--border-default); }

  .health[data-health='works'] .dot { background: var(--success-500); }

  .health[data-health='sometimes'] .dot, .health[data-health='attention'] .dot { background: var(--warning-500); }

  .health[data-health='failing'] .dot, .health[data-health='broken'] .dot { background: var(--danger-500); }

  .bars { display: flex; gap: var(--sp-025); align-items: flex-end; height: var(--sp-300); }

  .bars i { flex: 1 1 0; height: 100%; border-radius: var(--sp-025) var(--sp-025) 0 0; background: var(--success-500); }

  .bars i.s { height: 40%; background: var(--border-default); }

  .bars i.t { background: var(--warning-500); }

  .bars i.f { background: var(--danger-500); }

  .rows { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); }

  .row { width: 100%; display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); padding: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: transparent; text-align: start; }

  .row:hover { background: var(--bg-surface-hover); }

  .row:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .when { font-size: var(--fs-075); color: var(--text-subtle); font-variant-numeric: tabular-nums; white-space: nowrap; }

  .link { font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-brand); }
</style>
