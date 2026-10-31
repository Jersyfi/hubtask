<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The administration's front door (UC-ID-24 checks 1, 2, 5): what needs attention first - limits
  // close to their ceiling, rules failing, the last backup and whether it opened, privacy requests
  // owed soon, degraded features - each card linked to the page that acts on it.
  //
  // **Only reads the pages behind it already make**, decided by `lib/administration/overview.ts`,
  // so a card and its page never disagree. A card whose read this reader's role is refused is
  // absent, not empty (P-05): an auditor is not shown the privacy register's card, and is not sent
  // the read the server would refuse and record.
  //
  // **Nothing here writes.** Every card is a reading and a link, so an auditor sees the same
  // overview an administrator does, read-only by construction.

  import { untrack } from 'svelte';

  import { Badge, PageHeader, Spinner, Stack } from '@hubtask/design-system/components';

  import type { ConfigCheck } from '@hubtask/sync-engine';

  import { backupState, limitLines, newestArchive, owedSoon, pageOfFeature, pageOfFinding, ruleLines, type ArchiveReading } from '../lib/administration/overview.ts';
  import { backup } from '../lib/data/backup.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { health } from '../lib/data/health.svelte.ts';
  import { privacy, standingOf, deadlinePhrase } from '../lib/data/privacy.svelte.ts';
  import { quotas } from '../lib/data/quotas.svelte.ts';
  import { rules } from '../lib/data/rules.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  // Each read where the server answers this reader - the condition the read itself asks.
  const mayReadConfiguration = $derived(holdsHere('READ_CONFIGURATION') === true);
  const mayReadRules = $derived(holdsHere('AUTOMATION') === true || mayReadConfiguration);
  const mayReadBackup = $derived(holdsHere('STRUCTURE') === true || mayReadConfiguration);
  const mayReadRequests = $derived(holdsHere('MANAGE_MEMBERS') === true);

  $effect(() => untrack(() => quotas.open()));

  // The configuration check (configuration.md §11, UC-CFG-01 check 10): what the configuration
  // in force would strand people on, found before they meet it.
  const mayCheck = $derived(holdsHere('STRUCTURE') === true || mayReadConfiguration);
  let check = $state<{ status: string; data?: ConfigCheck }>({ status: 'idle' });
  $effect(() => {
    if (!mayCheck) return;
    return untrack(() => engine.subscribe<ConfigCheck>({ path: '/config/check' }, (next) => {
      check = next.status === 'ready' ? { status: 'ready', data: next.data } : { status: next.status };
    }));
  });
  const findings = $derived(check.data?.findings ?? []);
  $effect(() => {
    if (mayReadRules) return untrack(() => rules.open());
  });
  $effect(() => {
    if (mayReadRequests) return untrack(() => privacy.open(false));
  });
  $effect(() => {
    if (mayReadBackup) return untrack(() => backup.open());
  });

  // The archives of every enabled target, read once each, and the newest one's run - the half that
  // says whether it was opened since. Not every run: the overview asks about one backup.
  const targets = $derived(backup.targets.status === 'ready' ? backup.targets.data.filter((target) => target.enabled !== false) : []);
  let asked = new Set<string>();
  $effect(() => {
    for (const target of targets) {
      if (asked.has(target.id)) continue;
      asked = new Set([...asked, target.id]);
      void untrack(() => backup.readArchives(target.id, false, false));
    }
  });
  const archives = $derived(Object.fromEntries(
    targets.map((target) => {
      const state = backup.archivesAt(target.id);
      return [target.id, state.status === 'ready' ? (state.data as readonly ArchiveReading[]) : []];
    }),
  ));
  const archivesSettled = $derived(targets.every((target) => {
    const status = backup.archivesAt(target.id).status;
    return status === 'ready' || status === 'failed';
  }));
  const newest = $derived(newestArchive(archives));
  $effect(() => {
    const id = newest?.archive.archive_id;
    if (id && !backup.runOf(id)) void untrack(() => backup.readRun(id));
  });
  const lastBackup = $derived(newest ? { ...newest, state: backupState(newest.archive, backup.runOf(newest.archive.archive_id)) } : undefined);

  const limits = $derived(limitLines(quotas.standings));
  const failing = $derived(ruleLines(rules.all));
  const now = Date.now();
  const owed = $derived(owedSoon(privacy.of(false), (request) => standingOf(request, now)));
  const degraded = $derived(health.degradations);

  const KNOWN_LIMITS = new Set([
    'api_requests_per_minute', 'items', 'media_bytes', 'automation_runs_per_hour', 'webhook_targets',
    'export_jobs', 'ai_tokens_per_day', 'ai_own_tokens_per_day', 'ai_arrival_proposals_per_day', 'accounts',
  ]);
  const limitName = (quota: string) => (KNOWN_LIMITS.has(quota) ? t(`app.quotas.name.${quota}`) : quota);
  const amount = (quota: string, value: number) => (quota === 'media_bytes' ? formatBytes(value, messages.locale) : String(value));
  const at = (iso: string) => formatDateTime(iso, messages.locale);

  const LIMIT_TONE = { reached: 'danger', near: 'warning', room: 'neutral' } as const;
  const RULE_TONE = { failing: 'danger', broken: 'danger', attention: 'warning' } as const;
  const BACKUP_TONE = { opened: 'success', broken: 'danger', unchecked: 'warning', incomplete: 'danger' } as const;

  const reading = (status: string) => status === 'idle' || status === 'loading';

  /**
   * The cards in the order they need somebody: one that has something to say first, then the
   * quiet ones - the question the overview answers is "what needs me", not "where is everything".
   */
  const cards = $derived(
    [
      { id: 'limits', shown: true, needs: limits.some((line) => line.state !== 'room') },
      { id: 'rules', shown: mayReadRules, needs: failing.length > 0 },
      { id: 'backup', shown: mayReadBackup, needs: lastBackup === undefined ? targets.length > 0 : lastBackup.state !== 'opened' },
      { id: 'requests', shown: mayReadRequests, needs: owed.length > 0 },
      { id: 'health', shown: true, needs: degraded.length > 0 },
      { id: 'check', shown: mayCheck, needs: findings.length > 0 },
    ]
      .filter((card) => card.shown)
      .sort((a, b) => Number(b.needs) - Number(a.needs)),
  );

  $effect(() => page.entitle(t('app.admin_overview.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.admin_overview.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [{ id: 'administration', label: t('app.admin.title') }],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <div class="cards" data-testid="admin-overview">
    {#each cards as card (card.id)}
      <section class="card" data-card={card.id} data-needs={card.needs ? '' : undefined} aria-labelledby={`card-${card.id}`}>
        {#if card.id === 'limits'}
          <h2 id="card-limits">{t('app.admin_overview.limits')}</h2>
          {#if reading(quotas.state.status)}
            <p class="quiet"><Spinner label={t('app.admin_overview.reading')} /></p>
          {:else if limits.length === 0}
            <p class="quiet">{t('app.admin_overview.limits_none')}</p>
          {:else}
            <ul class="lines">
              {#each limits as line (line.quota)}
                <li>
                  <span>{limitName(line.quota)}</span>
                  <Badge tone={LIMIT_TONE[line.state]}>{t('app.admin_overview.limit_use', { used: amount(line.quota, line.used), limit: amount(line.quota, line.limit) })}</Badge>
                </li>
              {/each}
            </ul>
          {/if}
          <a class="go" href="/administration/quotas">{t('app.admin_overview.open_limits')}</a>
        {:else if card.id === 'rules'}
          <h2 id="card-rules">{t('app.admin_overview.rules')}</h2>
          {#if reading(rules.state.status)}
            <p class="quiet"><Spinner label={t('app.admin_overview.reading')} /></p>
          {:else if failing.length === 0}
            <p class="quiet">{t('app.admin_overview.rules_none')}</p>
          {:else}
            <ul class="lines">
              {#each failing.slice(0, 5) as line (line.id)}
                <li>
                  <a href={`/administration/rules/${line.id}`}>{line.name}</a>
                  <Badge tone={RULE_TONE[line.state]}>
                    {line.state === 'failing'
                      ? t('app.admin_overview.rule_failing', { count: line.failures })
                      : t(`app.admin_overview.rule_${line.state}`)}
                  </Badge>
                </li>
              {/each}
            </ul>
          {/if}
          <a class="go" href="/administration/rules">{t('app.admin_overview.open_rules')}</a>
        {:else if card.id === 'backup'}
          <h2 id="card-backup">{t('app.admin_overview.backup')}</h2>
          {#if reading(backup.targets.status) || !archivesSettled}
            <p class="quiet"><Spinner label={t('app.admin_overview.reading')} /></p>
          {:else if targets.length === 0}
            <p class="quiet">{t('app.admin_overview.backup_no_target')}</p>
          {:else if !lastBackup}
            <p>{t('app.admin_overview.backup_none')}</p>
          {:else}
            <p class="line">
              <span>{t('app.admin_overview.backup_last', { at: at(lastBackup.archive.created_at) })}</span>
              <Badge tone={BACKUP_TONE[lastBackup.state]}>{t(`app.admin_overview.backup_${lastBackup.state}`)}</Badge>
            </p>
          {/if}
          <a class="go" href="/administration/backup">{t('app.admin_overview.open_backup')}</a>
        {:else if card.id === 'requests'}
          <h2 id="card-requests">{t('app.admin_overview.requests')}</h2>
          {#if reading(privacy.stateOf(false).status)}
            <p class="quiet"><Spinner label={t('app.admin_overview.reading')} /></p>
          {:else if owed.length === 0}
            <p class="quiet">{t('app.admin_overview.requests_none')}</p>
          {:else}
            <ul class="lines">
              {#each owed.slice(0, 5) as request (request.id)}
                {@const phrase = deadlinePhrase(request, at)}
                <li>
                  <span>{t(`app.privacy.kind_${request.kind.toLowerCase()}`)}</span>
                  <Badge tone={standingOf(request, now) === 'overdue' ? 'danger' : 'warning'}>
                    {standingOf(request, now) === 'overdue' ? t('app.privacy.overdue') : t(phrase.code, phrase.params)}
                  </Badge>
                </li>
              {/each}
            </ul>
          {/if}
          <a class="go" href="/administration/privacy">{t('app.admin_overview.open_requests')}</a>
        {:else if card.id === 'check'}
          <h2 id="card-check">{t('app.admin.check_title')}</h2>
          {#if reading(check.status)}
            <p class="quiet"><Spinner label={t('app.admin_overview.reading')} /></p>
          {:else if findings.length === 0}
            <p class="quiet">{t('app.admin.check_none')}</p>
          {:else}
            <ul class="lines" data-testid="config-findings">
              {#each findings.slice(0, 8) as finding, index (index)}
                {@const where = pageOfFinding(finding.path)}
                <li>
                  {#if where}
                    <a href={where} title={t('app.admin.check_open', { path: finding.path })}>{t(finding.code, finding.params ?? {})}</a>
                  {:else}
                    <span>{t(finding.code, finding.params ?? {})}</span>
                  {/if}
                  <Badge tone={finding.severity === 'ERROR' ? 'danger' : 'warning'}>
                    {t(finding.severity === 'ERROR' ? 'app.admin.check_error' : 'app.admin.check_warning')}
                  </Badge>
                </li>
              {/each}
            </ul>
          {/if}
        {:else if card.id === 'health'}
          <h2 id="card-health">{t('app.admin_overview.health')}</h2>
          {#if degraded.length === 0}
            <p class="quiet">{t('app.admin_overview.health_fine')}</p>
          {:else}
            <ul class="lines">
              {#each degraded as degradation (degradation.feature)}
                {@const where = pageOfFeature(degradation.feature)}
                <li>
                  <span>{t('app.health.feature', { feature: degradation.feature, reason: t(degradation.reasonCode) })}</span>
                  {#if where}<a href={where}>{t('app.admin_overview.open_feature')}</a>{/if}
                </li>
              {/each}
            </ul>
          {/if}
        {/if}
      </section>
    {/each}
  </div>
</Stack>

<style>
  /* Cards side by side where there is room, one above the other on a phone. A list of cards takes
     the region rather than a measure (ADR-0065 decision 2). */
  .cards {
    display: grid;
    gap: var(--sp-200);
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 32ch), 1fr));
  }

  .card {
    display: grid;
    align-content: start;
    gap: var(--sp-100);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .card[data-needs] { border-color: var(--border-default); }

  h2 { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .line { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .lines { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }

  /* Two columns rather than a wrapping row: a long name wraps inside its own column, and every
     line's badge stays beside its name at the end - a row that wrapped put one badge beside its
     label and the next one under its own. */
  .lines li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: start;
    gap: var(--sp-100);
  }

  a { color: var(--text-brand); }

  .go { justify-self: start; font-size: var(--fs-100); }
</style>
