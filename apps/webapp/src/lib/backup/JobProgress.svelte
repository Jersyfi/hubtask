<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one backup or check running from this page, wherever it was started: one place for it
  // rather than one per button, since three watchers would be three loops disagreeing about which
  // is current. `progress: null` is the indeterminate case rather than zero - a bar at zero would
  // claim nothing has happened on a job that is halfway through.
  import { Button, ProgressBar, Stack } from '@hubtask/design-system/components';

  import type { Watch } from '../data/jobs.svelte.ts';
  import { mayCancel } from '../data/jobs.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { sentence } from './words.svelte.ts';

  interface Props {
    watch: Watch;
    /** A check of an archive rather than a backup. */
    isVerifying: boolean;
    /** Whether the reader may stop it; absent otherwise (P-05). */
    mayStop: boolean;
    onstop: () => void;
  }

  const { watch, isVerifying, mayStop, onstop }: Props = $props();

  const title = $derived(isVerifying ? t('app.backup.verifying_title') : t('app.backup.running_title'));
  const share = $derived(watch.job.progress === null || watch.job.progress === undefined ? undefined : Math.round(watch.job.progress * 100));
</script>

<section class="job" aria-label={title}>
  <Stack gap="100">
    <h2 class="title">{title}</h2>
    {#if watch.watching}
      <ProgressBar
        label={title}
        value={share}
        valueLabel={share === undefined ? t('app.backup.progress_unknown') : t('app.backup.progress_share', { percent: String(share) })}
      />
      {#if mayCancel(watch.job.status) && mayStop}
        <div><Button size="sm" tone="secondary" onclick={onstop}>{t('app.backup.cancel')}</Button></div>
        <!-- What a cancellation cannot take back, said where the button is. -->
        <p class="quiet">{t('app.backup.cancel_note')}</p>
      {/if}
    {:else if watch.job.status === 'SUCCEEDED'}
      <p class="good">{isVerifying ? t('app.backup.verify_done') : t('app.backup.run_done')}</p>
    {:else if watch.job.status === 'CANCELLED'}
      <p class="quiet">{t('app.backup.run_cancelled')}</p>
    {:else if watch.unreachable}
      <!-- A job whose status could not be read is not a job that failed. -->
      <p class="failure" role="alert">{sentence(watch.unreachable)}</p>
    {:else}
      <p class="failure" role="alert">{sentence(watch.job.error_code) ?? t('app.backup.run_failed')}</p>
    {/if}
  </Stack>
</section>

<style>
  .job {
    padding: var(--sp-200) var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .title { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .good { margin: 0; color: var(--text-success); font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
