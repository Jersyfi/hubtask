<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation at a glance (ADR-0070 §5), as an overview page of summary cards (page-system
  // §7.5, §9.4): what is wrong first, then the counts, then each area with what it says today.
  //
  // **Counts, states and limits — never rows.** The tenant boundary is a database policy rather
  // than a role, and this screen does not go around it: what it reads is five integers and the
  // health report, and there is no control here that reaches into a workspace.
  //
  // The health of the machinery is `/meta/health`, which is where it already lived: a second
  // surface for one fact is how two answers come to disagree.
  //
  // The one row it names is an own domain that fell back to its workspace's subdomain (UC-INS-15
  // check 9): the domain and the workspace's name, which the operator's *Own domains* lists anyway,
  // and nothing from inside the workspace. It stays until the owner makes the domain main again or
  // removes it.

  import { untrack } from 'svelte';

  import { Badge, Button, OverviewPage, SummaryCard, Spinner } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { health } from '../lib/data/health.svelte.ts';
  import { formatDateTime, formatLongDay, formatRelative, formatTime } from '../lib/i18n/datetime.ts';
  import { backlogTiles, featureNameCode, instancePageOfFeature, schemaState } from '../lib/instance/health.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { reachesAdministrationArea } from '../lib/data/capability.svelte.ts';
  import { rowsOf } from '../lib/instance/values.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    if (!instance.isElevated) return;
    // The digest below reads what the other screens read, which is why they are opened here: the
    // overview shows the *values* — the password minimum, the second factor, the providers offered —
    // rather than a list of links to go and find them.
    const stops = untrack(() => [
      instance.openOverview(),
      instance.openNearQuota(),
      instance.openSettings(),
      instance.openOperators(),
      instance.openProviders(),
      instance.openJournal(),
      instance.openOwnDomains(),
    ]);
    return () => stops.forEach((stop) => stop());
  });

  /**
   * How many of the installation's settings are decided, and how many of those are locked: every
   * area, as *Defaults* counts them, so the two screens give one number.
   */
  const decided = $derived.by(() => {
    const settings = instance.settings.status === 'ready' ? instance.settings.data : undefined;
    const entries = rowsOf(settings).map((row) => row.setting);
    return {
      decided: entries.filter((entry) => entry.set).length,
      total: entries.length,
      locked: entries.filter((entry) => entry.locked).length,
    };
  });

  const legal = $derived.by(() => {
    const settings = instance.settings.status === 'ready' ? instance.settings.data : undefined;
    const entries = Object.values(settings?.legal ?? {});
    return {
      decided: entries.filter((entry) => entry.set).length,
      total: entries.length,
      locked: entries.filter((entry) => entry.locked).length,
    };
  });

  /** The installation's providers, by name, as the overview prints them. */
  const offered = $derived(instance.providers.map((provider) => provider.display_name));

  /**
   * Who may run the installation, as one line (UC-INS-12 check 2). An empty register is not "nobody":
   * this reader reached the overview because the installation has one workspace and they own it,
   * which is said as such. Nothing is said while the register is still being read.
   */
  const operatorsLine = $derived.by(() => {
    if (instance.operatorsState.status !== 'ready') return '';
    const count = instance.operators.length;
    return count === 0 ? t('app.instance.digest_operators_sole') : t('app.instance.digest_operators', { count });
  });

  /** The newest journal entry, as the overview's one line of "what happened". */
  const latest = $derived(instance.journal[0]);

  const KNOWN_LIMITS = new Set([
    'api_requests_per_minute', 'items', 'media_bytes', 'automation_runs_per_hour', 'webhook_targets',
    'export_jobs', 'ai_tokens_per_day', 'ai_own_tokens_per_day', 'ai_arrival_proposals_per_day', 'accounts',
    'mailboxes',
  ]);
  const limitName = (quota: string) => (KNOWN_LIMITS.has(quota) ? t(`app.quotas.name.${quota}`) : quota);

  const reading = $derived(instance.overview);
  const census = $derived(reading.status === 'ready' ? reading.data : undefined);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  // The report is the bell's read; on a phone below a tab's root the bar draws no bell (the
  // navigation concept §9), so the screen keeps the read open itself, under the bell's condition.
  $effect(() => health.start(reachesAdministrationArea() === true || instance.isElevated));
  const report = $derived(health.report);
  const schema = $derived(schemaState(report));
  const backlogs = $derived(backlogTiles(report));
  const formatNumber = (value: number) => new Intl.NumberFormat(messages.locale, { maximumFractionDigits: 0 }).format(value);
  const fellBack = $derived(instance.ownDomains.filter((row) => row.fell_back_at));
  /** The domain, then what happened to it and where its workspace answers now. */
  const fellBackLine = (row: (typeof fellBack)[number]): string =>
    `${row.domain} ${t('app.instance.fell_back', {
      workspace: row.tenant_name,
      day: formatLongDay(row.fell_back_at ?? '', messages.locale),
      time: formatTime(row.fell_back_at ?? '', messages.locale),
      host: row.address_host ?? '',
    })}`;

  $effect(() => page.entitle(t('app.instance.overview')));

  const HEALTH_TONE = { ok: 'success', degraded: 'warning', down: 'danger', disabled: 'neutral' } as const;
  const go = (path: string) => () => onnavigate(path);
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <OverviewPage head={{ title: t('app.instance.overview'), breadcrumb: installationTrail() }} refused={door ?? (unreadable ? failed : undefined)}>
      <!-- The health report, where the reader may read it, first: what is wrong outranks the
           counts. No bearer means no request and a refusal is silence rather than a message,
           which is the frame's rule and not this screen's to relax. -->
      {#if report}
        <div class="tile" data-card="health">
          <SummaryCard title={t('app.instance.health')} state={{ label: t(`app.instance.health_state_${report.status}`), tone: HEALTH_TONE[report.status] }} line={t(`app.instance.health_${report.status}`)}>
            {#if report.degraded_features && report.degraded_features.length > 0}
              <ul class="lines" data-degraded>
                {#each report.degraded_features as feature (feature.feature)}
                  {@const where = instancePageOfFeature(feature.feature)}
                  <li>
                    <span class="feature">{featureNameCode(feature.feature) ? t(featureNameCode(feature.feature)!) : feature.feature}</span>
                    <span>{messages.has(feature.reason_code) ? t(feature.reason_code) : feature.reason_code}</span>
                    {#if feature.since}
                      <span class="quiet small">{t('app.instance.health_since', { since: formatRelative(feature.since, messages.locale) })}</span>
                    {/if}
                    {#if where}
                      <Button tone="subtle" size="sm" onclick={go(where)}>{t('app.instance.health_open')}</Button>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
            <!-- What the installation is missing without anything having failed - an empty register,
                 a statement nobody set - in the catalogue's words, which the operator acts on. -->
            {#if report.warnings && report.warnings.length > 0}
              <ul class="lines" data-warnings>
                {#each report.warnings as warning (warning.code + JSON.stringify(warning.params ?? {}))}
                  <li>{t(warning.code, (warning.params ?? {}) as Record<string, string>)}</li>
                {/each}
              </ul>
            {/if}
            <!-- The schema the instances agree on, from the report's `migration`. Whether instances
                 run different versions is not in the report, so it is not said here. -->
            <p class="line" data-upgrade>
              <span>{t('app.instance.upgrade_title')} · {t('app.instance.upgrade_version', { version: report.version })}</span>
              {#if schema}
                <Badge tone={schema === 'up_to_date' ? 'success' : schema === 'waiting' ? 'warning' : 'danger'}>{t(`app.instance.schema_${schema}`)}</Badge>
              {/if}
            </p>
          </SummaryCard>
        </div>
        {#if backlogs.length > 0}
          <div class="tile" data-card="backlogs">
            <SummaryCard title={t('app.instance.backlogs_title')} line={report.backlogs?.sampled_at ? t('app.instance.backlogs_counted', { at: formatRelative(report.backlogs.sampled_at, messages.locale) }) : undefined}>
              <ul class="lines">
                {#each backlogs as tile (tile.id)}
                  <li data-backlog={tile.id} data-tone={tile.isAbove ? 'warning' : undefined}>
                    <span>{t(`app.instance.backlog.${tile.id}`)}</span>
                    <Badge tone={tile.isAbove ? 'warning' : 'neutral'}>{formatNumber(tile.value)}</Badge>
                    {#if tile.href}
                      <Button tone="subtle" size="sm" onclick={go(tile.href)}>{t('app.instance.backlog_open')}</Button>
                    {/if}
                  </li>
                {/each}
              </ul>
            </SummaryCard>
          </div>
        {/if}
      {/if}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else if census}
        {#each fellBack as row (row.domain)}
          <div class="tile" data-fell-back={row.domain}>
            <SummaryCard
              title={t('app.instance.fell_back_title')}
              line={fellBackLine(row)}
              link={{ label: t('app.instance.own_domains'), href: '/instance/workspaces', onnavigate: go('/instance/workspaces') }}
            />
          </div>
        {/each}
        <div class="tile" data-card="workspaces">
          <SummaryCard
            title={t('app.instance.workspaces_active')}
            figure={formatNumber(census.workspaces_active)}
            line={census.workspaces_service_only > 0 ? t('app.instance.workspaces_service_only', { count: census.workspaces_service_only }) : undefined}
            link={{ label: t('app.instance.workspaces_open'), href: '/instance/workspaces', onnavigate: go('/instance/workspaces') }}
          >
            {#if !manifest.isSingleTenant}
              <!-- A single installation's one workspace is neither suspended nor deleted from here
                   (the server refuses both), so it is shown no counter for either. -->
              <ul class="lines">
                <li><span>{t('app.instance.workspaces_suspended')}</span><Badge tone={census.workspaces_suspended > 0 ? 'warning' : 'neutral'}>{formatNumber(census.workspaces_suspended)}</Badge></li>
                <li><span>{t('app.instance.workspaces_pending_deletion')}</span><Badge tone="neutral">{formatNumber(census.workspaces_pending_deletion)}</Badge></li>
              </ul>
            {/if}
          </SummaryCard>
        </div>
        <div class="tile" data-card="accounts">
          <SummaryCard title={t('app.instance.accounts_active')} figure={formatNumber(census.accounts_active)} line={`${t('app.instance.accounts_total')}: ${formatNumber(census.accounts_total)}`} />
        </div>

        <!-- Who A-18 counts: the alert names no workspace, so this says which one and at which
             wall, before somebody there meets the refusal. -->
        {#if instance.nearQuota.length > 0}
          <div class="tile" data-card="near-quota">
            <SummaryCard title={t('app.instance.near_quota_title')} state={{ label: String(instance.nearQuota.length), tone: 'warning' }} link={{ label: t('app.instance.near_quota_open'), href: '/instance/workspaces', onnavigate: go('/instance/workspaces') }}>
              <ul class="lines" data-near-quota>
                {#each instance.nearQuota as workspace (workspace.tenant_id)}
                  <li>
                    <strong>{workspace.slug}</strong>
                    <span class="quiet small">
                      {workspace.quotas
                        .map((standing) =>
                          t('app.instance.near_quota_standing', {
                            name: limitName(standing.quota),
                            used: standing.used ?? 0,
                            limit: standing.limit,
                            percent: Math.round((standing.ratio ?? 0) * 100),
                          }),
                        )
                        .join(' · ')}
                    </span>
                  </li>
                {/each}
              </ul>
            </SummaryCard>
          </div>
        {/if}

        <!-- Each area with what it currently says, and the way to it: the values are the point, and
             "Minimum length 12 · second factor for admins" is a sentence somebody reads instead of
             opening a screen. -->
        <div class="tile" data-card="settings">
          <SummaryCard title={t('app.instance.settings')} line={t('app.instance.digest_settings', { decided: decided.decided, total: decided.total, locked: decided.locked })} link={{ label: t('app.instance.digest_change'), href: '/instance/settings', onnavigate: go('/instance/settings') }} />
        </div>
        <div class="tile" data-card="legal">
          <SummaryCard title={t('app.instance.digest_legal')} line={t('app.instance.digest_legal_state', { decided: legal.decided, total: legal.total, locked: legal.locked })} link={{ label: t('app.instance.digest_change'), href: '/instance/settings', onnavigate: go('/instance/settings') }} />
        </div>
        <div class="tile" data-card="providers">
          <SummaryCard title={t('app.instance.providers')} line={offered.length === 0 ? t('app.instance.digest_providers_none') : t('app.instance.digest_providers', { names: offered.join(', ') })} link={{ label: t('app.instance.digest_manage'), href: '/instance/providers', onnavigate: go('/instance/providers') }} />
        </div>
        <div class="tile" data-card="operators">
          <SummaryCard title={t('app.instance.operators')} line={operatorsLine || undefined} link={{ label: t('app.instance.digest_manage'), href: '/instance/operators', onnavigate: go('/instance/operators') }} />
        </div>
        <div class="tile" data-card="journal">
          <SummaryCard
            title={t('app.instance.journal')}
            line={latest
              ? t('app.instance.digest_journal', { action: t(`app.instance.action.${latest.action.replaceAll('.', '_')}`), at: formatDateTime(latest.occurred_at, messages.locale) })
              : t('app.instance.journal_empty')}
            link={{ label: t('app.instance.digest_open'), href: '/instance/journal', onnavigate: go('/instance/journal') }}
          />
        </div>
      {/if}
    </OverviewPage>
  {/snippet}
</InstanceGate>

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<style>
  .tile { display: grid; min-inline-size: 0; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .small { font-size: var(--fs-100); }
  .feature { font-weight: var(--fw-semibold); }
  .lines { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .lines li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-050) var(--sp-100); }
  .line { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
</style>
