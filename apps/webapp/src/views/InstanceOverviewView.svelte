<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation at a glance (SI-17, ADR-0070 §5).
  //
  // **Counts, states and limits — never rows.** The tenant boundary is a database policy rather
  // than a role, and this screen does not go around it: what it reads is five integers and the
  // health report, and there is no control here that reaches into a workspace.
  //
  // The health of the machinery is `/meta/health`, which is where it already lived: a second
  // surface for one fact is how two answers come to disagree.

  import { untrack } from 'svelte';

  import { Banner, PageHeader, Spinner, Stack } from '@hubtask/design-system/components';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { health } from '../lib/data/health.svelte.ts';
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
    const stop = untrack(() => instance.openOverview());
    return stop;
  });

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
</style>
