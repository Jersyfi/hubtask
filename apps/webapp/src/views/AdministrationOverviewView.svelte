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
  //
  // An `OverviewPage` (page-system §7.5): *Getting started* is its one raised focal card, every
  // other card a `SummaryCard` region. First among them, the warning after the main own domain was lost (UC-INS-15 check 9): it broke
  // and the subdomain became the main address by itself, or the operator blocked it. It stays until
  // the owner makes the domain main again or removes it, so it is not dismissed here.

  import { featureNameCode } from '../lib/instance/health.ts';
  import { untrack } from 'svelte';

  import { Badge, OverviewPage, Spinner, SummaryCard } from '@hubtask/design-system/components';

  import { fallBackOf } from '../lib/address/owndomain.ts';

  import type { ConfigCheck, ConfigProposalList, ConfigTemplateList, ResourceState } from '@hubtask/sync-engine';

  import { backupState, limitAmount, limitLines, mailboxLines, newestArchive, owedSoon, pageOfFeature, pageOfFinding, ruleLines, waitingLines, type ArchiveReading } from '../lib/administration/overview.ts';
  import { gapsOf } from '../lib/administration/gaps.ts';
  import FirstStepsCard from '../lib/administration/FirstSteps.svelte';
  import { firstSteps, isOwed } from '../lib/administration/firststeps.ts';
  import { identityProvider } from '../lib/data/identityprovider.svelte.ts';
  import { signInPolicy } from '../lib/data/signinpolicy.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { config } from '../lib/data/config.svelte.ts';
  import { readMailboxes, type Mailbox } from '../lib/data/mailintake.svelte.ts';
  import { backup } from '../lib/data/backup.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { health } from '../lib/data/health.svelte.ts';
  import { privacy, standingOf, deadlinePhrase } from '../lib/data/privacy.svelte.ts';
  import { quotas } from '../lib/data/quotas.svelte.ts';
  import { rules } from '../lib/data/rules.svelte.ts';
  import { formatDateTime, formatLongDay, formatTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';

  // Each read where the server answers this reader - the condition the read itself asks.
  const mayReadConfiguration = $derived(holdsHere('READ_CONFIGURATION') === true);
  const mayReadRules = $derived(holdsHere('AUTOMATION') === true || mayReadConfiguration);
  const mayReadBackup = $derived(holdsHere('STRUCTURE') === true || mayReadConfiguration);
  const mayReadRequests = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const mayReadMail = $derived(holdsHere('AUTOMATION') === true);

  interface Props {
    onnavigate?: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => untrack(() => workspace.open()));
  const address = $derived(workspace.workspace?.address);
  const fallBack = $derived(fallBackOf(address?.own_domain ?? undefined));

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

  // What waits for a person's word (configuration.md §3, §8): plans an agent proposed and the
  // templates the workspace took that have a newer version - the same reads *Changes* and
  // *Templates* make.
  let proposals = $state<ResourceState<ConfigProposalList> | undefined>(undefined);
  let offered = $state<ResourceState<ConfigTemplateList> | undefined>(undefined);
  $effect(() => {
    if (!mayCheck) return;
    return untrack(() => {
      const stopProposals = config.waiting((next) => (proposals = next));
      const stopTemplates = config.templates((next) => (offered = next));
      return () => {
        stopProposals();
        stopTemplates();
      };
    });
  });
  const waiting = $derived(waitingLines(
    proposals?.status === 'ready' ? proposals.data.data : [],
    offered?.status === 'ready' ? offered.data.data : [],
  ));
  $effect(() => {
    if (mayReadRules) return untrack(() => rules.open());
  });
  $effect(() => {
    if (mayReadRequests) return untrack(() => privacy.open(false));
  });
  $effect(() => {
    if (mayReadBackup) return untrack(() => backup.open());
  });

  // The connected mailboxes (ADR-0094): one refusing for over an hour is mail that stops reaching
  // the desk, said here rather than only on the mail page nobody opens while it works.
  let mail = $state<{ status: string; boxes: readonly Mailbox[] }>({ status: 'idle', boxes: [] });
  $effect(() => {
    if (!mayReadMail) return;
    untrack(() => {
      mail = { status: 'loading', boxes: [] };
      void readMailboxes()
        .then((read) => (mail = { status: 'ready', boxes: read.mailboxes }))
        .catch(() => (mail = { status: 'failed', boxes: [] }));
    });
  });
  const mailFailing = $derived(mailboxLines(mail.boxes));

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

  // The first steps (user-experience.md §8.5). Sign-in is read only for whoever may set it up, the
  // step's own permission; the other facts are reads this page or the frame already makes.
  const maySetUpSignIn = $derived(holdsHere('STRUCTURE') === true);
  $effect(() => {
    if (!maySetUpSignIn) return;
    return untrack(() => {
      const stopPolicy = signInPolicy.open();
      const stopProviders = identityProvider.open();
      return () => {
        stopPolicy();
        stopProviders();
      };
    });
  });
  const factOf = (status: string | undefined, value: () => boolean): boolean | null | undefined =>
    status === 'ready' ? value() : status === 'failed' ? null : undefined;
  const steps = $derived(firstSteps({
    holds: (permission) => holdsHere(permission) === true,
    isDeskOn: (manifest.value?.modules_on ?? []).includes('SERVICE_DESK'),
    hasSeveralPeople: workspace.workspace === undefined ? undefined : workspace.workspace.several_people === true,
    hasModuleOn: manifest.value === undefined ? undefined : (manifest.value.modules_on ?? []).length > 0,
    hasMailbox: factOf(mail.status, () => mail.boxes.length > 0),
    hasSignIn: factOf(
      signInPolicy.state.status === 'failed' || identityProvider.state.status === 'failed'
        ? 'failed'
        : signInPolicy.policy !== undefined && identityProvider.state.status === 'ready' ? 'ready' : undefined,
      () => identityProvider.own.length > 0 || signInPolicy.policy?.mfa_required_for.value !== 'NONE',
    ),
    hasNightlyBackup: factOf(backup.schedules.status, () =>
      backup.schedules.status === 'ready' && backup.schedules.data.some((schedule) => schedule.enabled !== false)),
    hasBackupTarget: factOf(backup.targets.status, () => targets.length > 0),
    mayAddBackupTarget: manifest.value?.features?.backup_targets === true,
    hasSupportContact: manifest.value === undefined ? undefined : Boolean(manifest.value.legal?.support_url),
  }));
  // Put away by one reader in one browser - a convenience, so kept there and not on the server.
  const HIDDEN_KEY = 'hubtask.first-steps.hidden';
  const readHidden = (): boolean => {
    try {
      return globalThis.localStorage?.getItem(HIDDEN_KEY) === '1';
    } catch {
      return false;
    }
  };
  let isHidden = $state(readHidden());
  const hide = (hidden: boolean) => {
    isHidden = hidden;
    try {
      if (hidden) globalThis.localStorage?.setItem(HIDDEN_KEY, '1');
      else globalThis.localStorage?.removeItem(HIDDEN_KEY);
    } catch {
      // A browser that keeps nothing hides the card for this visit only.
    }
  };
  const isStepsOwed = $derived(isOwed(steps));

  const limits = $derived(limitLines(quotas.standings, { aiOn: manifest.value?.features?.ai_suggestions === true }));
  const failing = $derived(ruleLines(rules.all));
  const now = Date.now();
  const owed = $derived(owedSoon(privacy.of(false), (request) => standingOf(request, now)));
  const degraded = $derived(health.degradations);
  // What the installation is not set up for, which the health report counts as no fault.
  const gaps = $derived(gapsOf(manifest.value?.features));

  const KNOWN_LIMITS = new Set([
    'api_requests_per_minute', 'items', 'media_bytes', 'automation_runs_per_hour', 'webhook_targets',
    'export_jobs', 'ai_tokens_per_day', 'ai_own_tokens_per_day', 'ai_arrival_proposals_per_day', 'accounts',
    'mailboxes',
  ]);
  const limitName = (quota: string) => (KNOWN_LIMITS.has(quota) ? t(`app.quotas.name.${quota}`) : quota);
  const amount = (quota: string, value: number) => limitAmount(quota, value, messages.locale);
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
      { id: 'mail', shown: mayReadMail && (mail.boxes.length > 0 || mail.status !== 'ready'), needs: mailFailing.length > 0 },
      { id: 'backup', shown: mayReadBackup, needs: lastBackup === undefined ? targets.length > 0 : lastBackup.state !== 'opened' },
      { id: 'requests', shown: mayReadRequests, needs: owed.length > 0 },
      { id: 'health', shown: true, needs: degraded.length > 0 || gaps.length > 0 },
      { id: 'check', shown: mayCheck, needs: findings.length > 0 },
      { id: 'waiting', shown: mayCheck, needs: waiting.needs },
    ]
      .filter((card) => card.shown)
      .sort((a, b) => Number(b.needs) - Number(a.needs)),
  );

  $effect(() => page.entitle(t('app.admin_overview.title')));
</script>

<!-- The overview's one name for tests and readers: the page, its focal card and its tiles. -->
<div class="overview" data-testid="admin-overview">
<OverviewPage
  head={{
    title: t('app.admin_overview.title'),
    breadcrumb: {
      trail: [{ id: 'administration', label: t('app.admin.title') }],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
    menu: isStepsOwed && isHidden
      ? { label: t('app.admin_overview.menu'), items: [{ id: 'show-first-steps', label: t('app.first_steps.show') }], onselect: () => hide(false) }
      : undefined,
    figure: workspace.workspace ? figure : undefined,
  }}
  focal={isStepsOwed && !isHidden ? focal : undefined}
>
  {#if fallBack && address?.own_domain}
    <!-- The lost main domain first, as the tile that needs somebody most (UC-INS-15 check 9). -->
    <div class="tile" data-fell-back={fallBack.cause} data-needs>
      <SummaryCard
        title={t('app.admin_overview.fell_back_title', { domain: address.own_domain.domain })}
        state={{ label: t(fallBack.cause === 'blocked' ? 'app.address.own.state_blocked' : 'app.address.own.state_broken'), tone: 'danger' }}
        line={fallBack.cause === 'blocked'
          ? t('app.admin_overview.fell_back_blocked', { day: formatLongDay(fallBack.at, messages.locale), host: address.host })
          : t('app.admin_overview.fell_back_broken', {
              day: formatLongDay(fallBack.at, messages.locale),
              time: formatTime(fallBack.at, messages.locale),
              host: address.host,
            })}
        link={{ label: t('app.address.title'), href: '/administration/address', onnavigate: () => onnavigate?.('/administration/address') }}
      />
    </div>
  {/if}

  {#each cards as card (card.id)}
    <div class="tile" data-card={card.id} data-needs={card.needs ? '' : undefined}>
      {#if card.id === 'limits'}
        <SummaryCard title={t('app.admin_overview.limits')} line={reading(quotas.state.status) ? undefined : limits.length === 0 ? t('app.admin_overview.limits_none') : undefined} link={{ label: t('app.admin_overview.open_limits'), href: '/administration/quotas' }}>
          {#if reading(quotas.state.status)}
            {@render waitingFor()}
          {:else if limits.length > 0}
            <ul class="lines">
              {#each limits as line (line.quota)}
                <li>
                  <span>{limitName(line.quota)}</span>
                  <Badge tone={LIMIT_TONE[line.state]}>{t('app.admin_overview.limit_use', { used: amount(line.quota, line.used), limit: amount(line.quota, line.limit) })}</Badge>
                </li>
              {/each}
            </ul>
          {/if}
        </SummaryCard>
      {:else if card.id === 'rules'}
        <SummaryCard title={t('app.admin_overview.rules')} line={!reading(rules.state.status) && failing.length === 0 ? t('app.admin_overview.rules_none') : undefined} link={{ label: t('app.admin_overview.open_rules'), href: '/administration/rules' }}>
          {#if reading(rules.state.status)}
            {@render waitingFor()}
          {:else if failing.length > 0}
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
        </SummaryCard>
      {:else if card.id === 'mail'}
        <SummaryCard
          title={t('app.admin_overview.mail')}
          line={reading(mail.status) ? undefined : mail.status === 'failed' ? t('app.admin_overview.mail_unread') : mailFailing.length === 0 ? t('app.admin_overview.mail_none') : undefined}
          link={{ label: t('app.admin_overview.open_mail'), href: '/administration/mail' }}
        >
          {#if reading(mail.status)}
            {@render waitingFor()}
          {:else if mail.status !== 'failed' && mailFailing.length > 0}
            <ul class="lines">
              {#each mailFailing.slice(0, 5) as box (box.id)}
                <li>
                  <span>{box.address}</span>
                  <Badge tone="danger">{t('app.admin_overview.mail_failing')}</Badge>
                </li>
              {/each}
            </ul>
          {/if}
        </SummaryCard>
      {:else if card.id === 'backup'}
        {@const isReading = reading(backup.targets.status) || !archivesSettled}
        <SummaryCard
          title={t('app.admin_overview.backup')}
          state={!isReading && targets.length === 0 ? { label: t('app.admin_overview.backup_no_target_state'), tone: 'danger' } : undefined}
          line={isReading ? undefined : targets.length === 0 ? t('app.admin_overview.backup_no_target') : !lastBackup ? t('app.admin_overview.backup_none') : undefined}
          link={{ label: t('app.admin_overview.open_backup'), href: '/administration/backup' }}
        >
          {#if isReading}
            {@render waitingFor()}
          {:else if targets.length > 0 && lastBackup}
            <p class="line">
              <span>{t('app.admin_overview.backup_last', { at: at(lastBackup.archive.created_at) })}</span>
              <Badge tone={BACKUP_TONE[lastBackup.state]}>{t(`app.admin_overview.backup_${lastBackup.state}`)}</Badge>
            </p>
          {/if}
        </SummaryCard>
      {:else if card.id === 'requests'}
        <SummaryCard title={t('app.admin_overview.requests')} line={!reading(privacy.stateOf(false).status) && owed.length === 0 ? t('app.admin_overview.requests_none') : undefined} link={{ label: t('app.admin_overview.open_requests'), href: '/administration/privacy' }}>
          {#if reading(privacy.stateOf(false).status)}
            {@render waitingFor()}
          {:else if owed.length > 0}
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
        </SummaryCard>
      {:else if card.id === 'check'}
        <SummaryCard title={t('app.admin.check_title')} line={!reading(check.status) && findings.length === 0 ? t('app.admin.check_none') : undefined}>
          {#if reading(check.status)}
            {@render waitingFor()}
          {:else if findings.length > 0}
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
        </SummaryCard>
      {:else if card.id === 'waiting'}
        {@const isReading = reading(proposals?.status ?? 'idle') || reading(offered?.status ?? 'idle')}
        <SummaryCard title={t('app.admin_overview.waiting')} line={!isReading && !waiting.needs ? t('app.admin_overview.waiting_none') : undefined} link={{ label: t('app.admin_overview.open_changes'), href: '/administration/changes' }}>
          {#if isReading}
            {@render waitingFor()}
          {:else if waiting.needs}
            <ul class="lines" data-testid="admin-waiting">
              {#if waiting.proposals > 0}
                <li><a href="/administration/changes">{t('app.admin_overview.waiting_proposals', { count: waiting.proposals })}</a></li>
              {/if}
              {#each waiting.updates.slice(0, 5) as update (update.key)}
                <li><a href="/administration/templates?tab=in-use">{t('app.admin_overview.waiting_update', { name: update.name })}</a></li>
              {/each}
            </ul>
          {/if}
        </SummaryCard>
      {:else if card.id === 'health'}
        <SummaryCard title={t('app.admin_overview.health')} line={degraded.length === 0 && gaps.length === 0 ? t('app.admin_overview.health_fine') : undefined}>
          {#if degraded.length > 0 || gaps.length > 0}
            <ul class="lines" data-testid="admin-health">
              {#each gaps as gap (gap.feature)}
                <li>
                  <span>{t(gap.messageCode)}</span>
                  {#if gap.page}<a href={gap.page}>{t('app.admin_overview.open_feature')}</a>{/if}
                </li>
              {/each}
              {#each degraded as degradation (degradation.feature)}
                {@const where = pageOfFeature(degradation.feature)}
                <li>
                  <span>{t('app.health.feature', { feature: featureNameCode(degradation.feature) ? t(featureNameCode(degradation.feature)!) : degradation.feature, reason: t(degradation.reasonCode) })}</span>
                  {#if where}<a href={where}>{t('app.admin_overview.open_feature')}</a>{/if}
                </li>
              {/each}
            </ul>
          {/if}
        </SummaryCard>
      {/if}
    </div>
  {/each}
</OverviewPage>
</div>

{#snippet figure()}<span>{workspace.workspace?.display_name}</span>{/snippet}

{#snippet focal()}<FirstStepsCard {steps} onhide={() => hide(true)} />{/snippet}

{#snippet waitingFor()}<p class="quiet"><Spinner label={t('app.admin_overview.reading')} /></p>{/snippet}

<style>
  /* A tile fills its cell, so the tiles of one row end on one line. */
  .tile { display: grid; min-width: 0; }

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
</style>
