<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Export…* as a Dialog M (page-system §9.3): a period of its own, the format and the backup
  // target, then the job's progress and where the archive went.
  //
  // **The export goes to a backup target and the browser never sees the archive** (ADR-0047): what
  // the dialog shows when the job ends is *where* the archive was written - not a download. And
  // **there is no filter on an export**, said where somebody would look for the filters they just
  // used: an export narrowed before it was signed would be evidence about a selection.
  import { tick, untrack } from 'svelte';

  import { Banner, Button, Dialog, Input, ProgressBar, Select, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { audit } from '../data/audit.svelte.ts';
  import { backup } from '../data/backup.svelte.ts';
  import { jobs, type Watch } from '../data/jobs.svelte.ts';
  import { isTerminal } from '../data/jobs.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
  }

  let { isOpen = $bindable(false) }: Props = $props();

  const FORMATS = ['JSONL', 'CSV'];

  let exportFrom = $state('');
  let exportTo = $state('');
  let exportFormat = $state('JSONL');
  let exportTarget = $state('');
  let exportJob = $state('');
  let exportedTo = $state<{ target: string } | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let missing = $state<Missing>({});

  $effect(() => untrack(() => backup.open()));

  const watch = $derived<Watch | undefined>(exportJob ? jobs.of(exportJob) : undefined);
  const markOf = (key: string, value: string): string | undefined => (missing[key] && !value ? t(missing[key]) : undefined);
  const sentence = (code: string | null | undefined) => (code ? (messages.has(code) ? t(code) : code) : undefined);

  // The report has arrived once the job is over. Read once, rather than a second polling loop.
  $effect(() => {
    const current = watch;
    if (!current || current.watching || !isTerminal(current.job.status) || current.job.status !== 'SUCCEEDED') return;
    untrack(() => {
      exportedTo = { target: backup.all.find((each) => each.id === exportTarget)?.name ?? exportTarget };
    });
  });

  function close(): void {
    isOpen = false;
  }

  const guard = new DiscardGuard(() => !exportJob && (exportFrom !== '' || exportTo !== '' || exportTarget !== ''), close);

  function reset(): void {
    exportFrom = '';
    exportTo = '';
    exportFormat = 'JSONL';
    exportTarget = '';
    exportJob = '';
    exportedTo = undefined;
    failure = undefined;
    missing = {};
  }

  async function start(): Promise<void> {
    missing = check([
      { kind: 'field', key: 'exportFrom', code: 'app.audit.period_first', isFilled: exportFrom !== '' },
      { kind: 'field', key: 'exportTo', code: 'app.audit.period_first', isFilled: exportTo !== '' },
      { kind: 'field', key: 'target', code: 'app.audit.target_first', isFilled: exportTarget !== '' },
    ]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(document.querySelector('dialog[open]') ?? document));
      return;
    }
    failure = undefined;
    isWorking = true;
    try {
      exportedTo = undefined;
      const accepted = await audit.export({
        from: new Date(exportFrom).toISOString(),
        to: new Date(exportTo).toISOString(),
        format: exportFormat as 'JSONL' | 'CSV',
        target_id: exportTarget,
      });
      exportJob = accepted.job_id;
      announcer.say(t('app.audit.export_started_announced'));
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.audit.export_title')}
  bind:isOpen
  dismissLabel={t('app.page.cancel')}
  onDismiss={guard.dismiss}
  onClose={reset}
  primary={exportJob ? undefined : { label: t('app.audit.export'), isBusy: isWorking, busyLabel: t('app.audit.exporting'), onclick: () => void start() }}
>
  <Stack gap="200">
    <p class="quiet">{t('app.audit.export_intro')} {t('app.audit.export_has_no_filter')}</p>
    {#if failure}<FailureNote problem={failure} />{/if}
    <div class="fields">
      <Input label={t('app.audit.from')} type="datetime-local" bind:value={exportFrom} error={markOf('exportFrom', exportFrom)} />
      <Input label={t('app.audit.to')} type="datetime-local" bind:value={exportTo} error={markOf('exportTo', exportTo)} />
    </div>
    <Select
      label={t('app.audit.format')}
      bind:value={exportFormat}
      options={FORMATS.map((value) => ({ value, label: t(`app.audit.format_${value.toLowerCase()}`) }))}
    />
    <Select
      label={t('app.audit.export_target')}
      bind:value={exportTarget}
      placeholder={t('app.audit.choose_target')}
      options={backup.all.map((target) => ({ value: target.id, label: target.name }))}
      error={markOf('target', exportTarget)}
    />
    <p class="quiet small">{t('app.audit.export_is_audited')}</p>

    {#if watch}
      {#if watch.watching}
        <ProgressBar
          label={t('app.audit.export_title')}
          value={watch.job.progress === null || watch.job.progress === undefined ? undefined : Math.round(watch.job.progress * 100)}
          valueLabel={watch.job.progress === null || watch.job.progress === undefined
            ? t('app.audit.progress_unknown')
            : t('app.audit.progress_share', { percent: String(Math.round(watch.job.progress * 100)) })}
        />
      {:else if watch.job.status === 'SUCCEEDED' && exportedTo}
        <!-- Where it went, and no download (ADR-0047). -->
        <Banner tone="success" title={t('app.audit.export_done')}>
          <Stack gap="050">
            <span>{t('app.audit.export_written_to', { target: exportedTo.target })}</span>
            <span>{t('app.audit.export_no_download')}</span>
          </Stack>
        </Banner>
      {:else if watch.unreachable}
        <p class="failure" role="alert">{sentence(watch.unreachable)}</p>
      {:else if watch.job.status !== 'SUCCEEDED'}
        <p class="failure" role="alert">{sentence(watch.job.error_code) ?? t('app.audit.export_failed')}</p>
      {/if}
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{exportJob ? t('app.page.close') : t('app.page.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(20ch, 1fr)); gap: var(--sp-150); }
</style>
