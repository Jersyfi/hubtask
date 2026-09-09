<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What this workspace may use, and how close it is (`multi-tenancy.md` §4, H-08).
  //
  // **There is no control to raise a limit, and the screen says who to ask.**
  // `/admin/tenants/{id}/quotas` is the installation operator's (0.6.0 decision 6). A button the
  // server would refuse is worse than no button — it teaches somebody that the product is broken
  // rather than that the decision is somebody else's.
  //
  // **`limit: 0` is unlimited, not zero.** Rendering it as a number would say the opposite of what
  // it means, and "0 of 0 used" is the kind of screen somebody opens a support thread about.
  //
  // **`configured: false` is the mode's default rather than this workspace's own ceiling**, and
  // the difference matters to whoever is deciding whether to ask for more: one is a number
  // somebody chose for this workspace, the other is what every workspace on this installation
  // gets.
  //
  // **A quota this build has never heard of still renders.** Tolerance towards unknown fields is
  // binding, and a limit the client dropped would be a limit nobody can see they are near. Its
  // name is shown as the server's own token, because a made-up sentence would be worse.

  import { untrack } from 'svelte';

  import { Banner, ProgressBar, Spinner, Stack, Table } from '@hubtask/design-system/components';

  import { quotas } from '../lib/data/quotas.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  $effect(() => untrack(() => quotas.open()));

  const reading = $derived(quotas.state);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const columns = [
    { id: 'quota', label: t('app.quotas.limit_name') },
    { id: 'limit', label: t('app.quotas.ceiling') },
    { id: 'used', label: t('app.quotas.used') },
    { id: 'ratio', label: t('app.quotas.approach') },
    { id: 'source', label: t('app.quotas.source') },
  ];

  /** The wording for a limit this build knows. One it does not is shown as the server's token. */
  const KNOWN = new Set([
    'api_requests_per_minute',
    'items',
    'media_bytes',
    'automation_runs_per_hour',
    'webhook_targets',
    'export_jobs',
    'ai_tokens_per_day',
  ]);

  const nameOf = (quota: string) => (KNOWN.has(quota) ? t(`app.quotas.name.${quota}`) : quota);

  /** The same number `hubtask_tenant_quota_usage_ratio` reports, as a percentage a person reads. */
  const percentOf = (ratio: number) => Math.round(ratio * 100);
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.quotas.title')}</h1>
    <p class="quiet">{t('app.quotas.intro')}</p>

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting">
        <Spinner label={t('app.quotas.reading')} />
        <span>{t('app.quotas.reading')}</span>
      </p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else}
      <Table label={t('app.quotas.title')} isLabelHidden {columns}>
        {#each quotas.standings as standing (standing.quota)}
          <tr>
            <th scope="row" class="name">{nameOf(standing.quota)}</th>
            <td>
              {#if standing.limit === 0}
                <!-- Zero is the contract's word for "no ceiling". Printing the digit would say the
                     opposite of what it means. -->
                {t('app.quotas.unlimited')}
              {:else}
                {standing.limit}
              {/if}
            </td>
            <td>
              {#if standing.used === undefined || standing.used === null}
                <!-- No live count: the request rate is enforced by the limiter and reported in its
                     own headers, so this screen would be inventing a number. -->
                <span class="quiet">{t('app.quotas.no_count')}</span>
              {:else}
                {standing.used}
              {/if}
            </td>
            <td class="approach">
              {#if standing.ratio === undefined || standing.ratio === null}
                <span class="quiet">—</span>
              {:else}
                <ProgressBar
                  size="sm"
                  label={nameOf(standing.quota)}
                  value={Math.min(100, percentOf(standing.ratio))}
                  valueLabel={t('app.quotas.percent', { percent: String(percentOf(standing.ratio)) })}
                />
              {/if}
            </td>
            <td>
              {standing.configured ? t('app.quotas.workspace') : t('app.quotas.installation')}
            </td>
          </tr>
        {/each}
      </Table>

      <p class="quiet">{t('app.quotas.raising')}</p>
    {/if}
  </Stack>
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); white-space: nowrap; }

  /* Wide enough for the bar to mean something, and it is the only column that wants width. */
  .approach { min-inline-size: 12ch; }
</style>
