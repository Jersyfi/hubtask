<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation at a glance (ADR-0070 §5).
  //
  // **Counts, states and limits — never rows.** The tenant boundary is a database policy rather
  // than a role, and this screen does not go around it: what it reads is five integers and the
  // health report, and there is no control here that reaches into a workspace.
  //
  // The health of the machinery is `/meta/health`, which is where it already lived: a second
  // surface for one fact is how two answers come to disagree.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, PageHeader, Spinner, Stack } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { health } from '../lib/data/health.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    // The digest below reads what the other screens read, which is why they are opened here: the
    // overview shows the *values* — the password minimum, the second factor, the providers offered —
    // rather than a list of links to go and find them.
    const stops = untrack(() => [
      instance.openOverview(),
      instance.openSettings(),
      instance.openOperators(),
      instance.openProviders(),
      instance.openJournal(),
    ]);
    return () => stops.forEach((stop) => stop());
  });

  /** How many of the installation's switches are decided, and how many of those are locked. */
  const signIn = $derived.by(() => {
    const settings = instance.settings.status === 'ready' ? instance.settings.data : undefined;
    const entries = Object.values(settings?.sign_in ?? {});
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

  /** The newest journal entry, as the overview's one line of "what happened". */
  const latest = $derived(instance.journal[0]);

  /**
   * The areas this level will hold and does not yet.
   *
   * Held open rather than left out: a reader who cannot see that a thing is coming cannot tell it
   * apart from a thing nobody thought of (P-11).
   */
  const later = [
    { id: 'ai', code: 'app.instance.later_ai' },
    { id: 'retention', code: 'app.instance.later_retention' },
    { id: 'backup', code: 'app.instance.later_backup' },
    { id: 'plans', code: 'app.instance.later_plans' },
  ];

  const reading = $derived(instance.overview);
  const census = $derived(reading.status === 'ready' ? reading.data : undefined);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  const report = $derived(health.report);

  $effect(() => page.entitle(t('app.instance.overview')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.overview')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="300">
      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else if census}
        <dl class="counts">
          <div class="count">
            <dt>{t('app.instance.workspaces_active')}</dt>
            <dd>{census.workspaces_active}</dd>
          </div>
          <div class="count">
            <dt>{t('app.instance.workspaces_suspended')}</dt>
            <dd>{census.workspaces_suspended}</dd>
          </div>
          <div class="count">
            <dt>{t('app.instance.workspaces_pending_deletion')}</dt>
            <dd>{census.workspaces_pending_deletion}</dd>
          </div>
          <div class="count">
            <dt>{t('app.instance.accounts_active')}</dt>
            <dd>{census.accounts_active}</dd>
          </div>
          <div class="count">
            <dt>{t('app.instance.accounts_total')}</dt>
            <dd>{census.accounts_total}</dd>
          </div>
        </dl>

        <!-- The digest the concept's §5.5 draws: each area with what it currently says, and the
             way to it. Not an index of the column beside it — the values are the point, and
             "Minimum length 12 · second factor for admins 🔒" is a sentence somebody reads instead
             of opening a screen. -->
        <dl class="digest">
          <div class="entry">
            <dt>{t('app.instance.settings')}</dt>
            <dd>
              <span>{t('app.instance.digest_sign_in', { decided: signIn.decided, total: signIn.total, locked: signIn.locked })}</span>
              <Button tone="subtle" size="sm" onclick={() => onnavigate('/instance/settings')}>
                {t('app.instance.digest_change')}
              </Button>
            </dd>
          </div>
          <div class="entry">
            <dt>{t('app.instance.digest_legal')}</dt>
            <dd>
              <span>{t('app.instance.digest_legal_state', { decided: legal.decided, total: legal.total, locked: legal.locked })}</span>
              <Button tone="subtle" size="sm" onclick={() => onnavigate('/instance/settings')}>
                {t('app.instance.digest_change')}
              </Button>
            </dd>
          </div>
          <div class="entry">
            <dt>{t('app.instance.providers')}</dt>
            <dd>
              <span>
                {offered.length === 0
                  ? t('app.instance.digest_providers_none')
                  : t('app.instance.digest_providers', { names: offered.join(', ') })}
              </span>
              <Button tone="subtle" size="sm" onclick={() => onnavigate('/instance/providers')}>
                {t('app.instance.digest_manage')}
              </Button>
            </dd>
          </div>
          <div class="entry">
            <dt>{t('app.instance.operators')}</dt>
            <dd>
              <span>{t('app.instance.digest_operators', { count: instance.operators.length })}</span>
              <Button tone="subtle" size="sm" onclick={() => onnavigate('/instance/operators')}>
                {t('app.instance.digest_manage')}
              </Button>
            </dd>
          </div>
          <div class="entry">
            <dt>{t('app.instance.journal')}</dt>
            <dd>
              <span>
                {latest
                  ? t('app.instance.digest_journal', {
                      action: t(`app.instance.action.${latest.action.replaceAll('.', '_')}`),
                      at: formatDateTime(latest.occurred_at, messages.locale),
                    })
                  : t('app.instance.journal_empty')}
              </span>
              <Button tone="subtle" size="sm" onclick={() => onnavigate('/instance/journal')}>
                {t('app.instance.digest_open')}
              </Button>
            </dd>
          </div>
        </dl>

        <!-- The places this level holds open. The concept defers each of these and says
             the place is kept; a reader who cannot see that a thing is coming
             cannot tell it from a thing nobody thought of. -->
        <Stack gap="100">
          <h2 class="section">{t('app.instance.later_title')}</h2>
          <ul class="later">
            {#each later as row (row.id)}
              <li>
                <Badge tone="neutral">{t('app.instance.later_badge')}</Badge>
                <span>{t(row.code)}</span>
              </li>
            {/each}
          </ul>
        </Stack>

        <!-- The health report, where the reader may read it. No bearer means no request and a
             refusal is silence rather than a message, which is the frame's rule and not this
             screen's to relax. -->
        {#if report}
          <Stack gap="150">
            <h2 class="section">{t('app.instance.health')}</h2>
            <Banner tone={report.status === 'ok' ? 'success' : report.status === 'degraded' ? 'warning' : 'danger'}>
              {t(`app.instance.health_${report.status}`)}
            </Banner>
            {#if report.degraded_features && report.degraded_features.length > 0}
              <ul class="features">
                {#each report.degraded_features as feature (feature.feature)}
                  <li>{feature.feature}</li>
                {/each}
              </ul>
            {/if}
          </Stack>
        {/if}
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  /* A row of numbers takes the region it is given: this is the list case of ADR-0065 decision 2,
     not the prose one. */
  .counts {
    margin: 0;
    display: grid;
    gap: var(--sp-150);
    grid-template-columns: repeat(auto-fit, minmax(14ch, 1fr));
  }

  .count {
    display: grid;
    gap: var(--sp-025);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .count dt { color: var(--text-secondary); font-size: var(--fs-100); }
  .count dd { margin: 0; font-family: var(--font-display); font-size: var(--fs-500); font-weight: var(--fw-semibold); }

  .features { margin: 0; padding-inline-start: var(--sp-200); color: var(--text-secondary); font-size: var(--fs-100); }

  /* Five rows of "what this says, and the way to it". A list, so it takes the region rather than a
     measure (ADR-0065 decision 2). */
  .digest { margin: 0; display: grid; gap: var(--sp-100); }

  .entry {
    display: grid;
    gap: var(--sp-025);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .entry dt { color: var(--text-secondary); font-size: var(--fs-100); }

  .entry dd {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100);
  }

  .later { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .later li { display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); font-size: var(--fs-100); }
</style>
