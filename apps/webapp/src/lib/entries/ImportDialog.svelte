<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Import… on a hub: four steps in one dialog (F6-09, decision 15).
  //
  // **The kind, the file, the mapping, the run.** The kinds are the contract's enum, each with
  // one sentence saying what file it takes and where the source exports it. The file goes through
  // the media upload staged as `IMPORT`, claiming the kind's type rather than the browser's guess.
  // For `CSV` only, the header row is read here and each of the seven fields offers the file's
  // columns, prefilled where a header already names one; what differs is sent as `mapping`. Then
  // `POST /imports`, the job followed the way every job is, and the report rendered by the
  // restore's own component - an import lands through the restore and reports in its shape.
  //
  // **A second import of the same file is the no-op P-08 promised**, and the dialog says so in the
  // report - "left as they are" - rather than warning beforehand: it cannot know, and the server
  // creates nothing the second time.
  //
  // **A kind this build refuses answers by name** (`imports.kind_unsupported`) and that answer is
  // what the dialog shows; the manifest declares no import capability and nothing is hidden here.
  //
  // **Each step is announced** (F5-11): the heading changes and the announcer says it, so a reader
  // who is not looking hears where they are.

  import { Button, Dialog, Inline, ProgressBar, Radio, Select, Stack, UploadField } from '@hubtask/design-system/components';
  import { untrack } from 'svelte';

  import { announcer } from '../announce.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { imports } from '../data/imports.svelte.ts';
  import {
    CSV_FIELDS,
    IMPORT_KINDS,
    acceptFor,
    contentTypeFor,
    importIdOf,
    mappingFor,
    parseHeader,
    prefill,
    type CsvField,
    type ImportKind,
  } from '../data/imports.ts';
  import { jobs, type Watch } from '../data/jobs.svelte.ts';
  import { isTerminal } from '../data/jobs.ts';
  import { media } from '../data/media.svelte.ts';
  import { isWithinUploadLimit, uploadLimitOf } from '../data/media.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import RestoreReport from '../restore/RestoreReport.svelte';

  interface Props {
    isOpen?: boolean;
    /** The hub the imported collections land under. */
    hubId: string;
    /** Where a collection that landed is opened. */
    onnavigate: (path: string) => void;
  }

  let { isOpen = $bindable(false), hubId, onnavigate }: Props = $props();

  type Step = 'kind' | 'file' | 'mapping' | 'run';
  const STEPS: readonly Step[] = ['kind', 'file', 'mapping', 'run'];

  let step = $state<Step>('kind');
  let kind = $state<ImportKind>('CSV');
  let chosen = $state<File | undefined>(undefined);
  let columns = $state<readonly string[]>([]);
  let mapping = $state<Record<CsvField, string>>(prefill([]));
  let sent = $state(0);
  let controller = $state<AbortController | undefined>(undefined);
  let mediaId = $state<string | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let jobId = $state<string | undefined>(undefined);
  let importId = $state<string | undefined>(undefined);
  let isStarting = $state(false);
  /** The hub's collections before the run, so that what landed can be told from what was there. */
  let before = $state<ReadonlySet<string>>(new Set());

  const limit = $derived(uploadLimitOf(manifest.value));
  const isUploading = $derived(controller !== undefined);
  const overLimit = $derived(chosen !== undefined && !isWithinUploadLimit(chosen.size, limit));
  const percent = $derived(chosen && chosen.size > 0 ? Math.floor((sent / chosen.size) * 100) : 0);
  const watch = $derived<Watch | undefined>(jobId ? jobs.of(jobId) : undefined);
  const run = $derived(importId ? imports.of(importId) : undefined);
  /** The step's number for the heading: "Step 2 of 4". The mapping step counts only for CSV. */
  const steps = $derived(kind === 'CSV' ? STEPS : STEPS.filter((each) => each !== 'mapping'));
  const stepNumber = $derived(steps.indexOf(step) + 1);
  const heading = $derived(
    t('app.import.step', { number: String(stepNumber), of: String(steps.length), name: t(`app.import.step_${step}`) }),
  );

  const sizeLabel = $derived(
    chosen === undefined
      ? undefined
      : limit === undefined
        ? t('app.media.size', { size: formatBytes(chosen.size, messages.locale) })
        : t('app.media.size_of_limit', {
            size: formatBytes(chosen.size, messages.locale),
            limit: formatBytes(limit, messages.locale),
          }),
  );

  /** The collections that landed: under the hub now and not before the run. */
  const landed = $derived(
    run?.status === 'SUCCEEDED' ? containers.collectionsOf(hubId).filter((each) => !before.has(each.id)) : [],
  );

  // Opening starts over: the dialog is about one file, and a report left on screen would be a
  // report about an import somebody is no longer composing.
  $effect(() => {
    if (!isOpen) return;
    untrack(() => {
      step = 'kind';
      kind = 'CSV';
      chosen = undefined;
      columns = [];
      mapping = prefill([]);
      sent = 0;
      mediaId = undefined;
      failure = undefined;
      if (jobId) jobs.forget(jobId);
      jobId = undefined;
      importId = undefined;
    });
  });

  // A finished job means the report has arrived. Read it once; the watcher already stopped.
  $effect(() => {
    const current = watch;
    if (!current || current.watching || !isTerminal(current.job.status)) return;
    const id = importId;
    if (!id) return;
    void untrack(async () => {
      await imports.read(id);
      const done = imports.of(id);
      if (done?.status === 'SUCCEEDED') await containers.refreshLevel(hubId);
      announcer.say(done?.status === 'SUCCEEDED' ? t('app.import.done_announced') : t('app.import.failed_announced'));
    });
  });

  function go(next: Step) {
    step = next;
    failure = undefined;
    announcer.say(
      t('app.import.step', { number: String(steps.indexOf(next) + 1), of: String(steps.length), name: t(`app.import.step_${next}`) }),
    );
  }

  async function choose(file: File) {
    chosen = file;
    failure = undefined;
    sent = 0;
    mediaId = undefined;
    columns = [];
    if (!isWithinUploadLimit(file.size, limit)) return;

    if (kind === 'CSV') {
      // The header row only: the first few kilobytes are enough for a line, and a large file is
      // not read twice.
      try {
        columns = parseHeader(await file.slice(0, 64 * 1024).text());
      } catch {
        columns = [];
      }
      mapping = prefill(columns);
    }

    const abort = new AbortController();
    controller = abort;
    try {
      const object = await media.upload(file, 'IMPORT', {
        idempotencyKey: crypto.randomUUID(),
        signal: abort.signal,
        contentType: contentTypeFor(kind),
        onProgress: (moved) => (sent = moved),
      });
      mediaId = object.id;
    } catch (error) {
      failure = abort.signal.aborted ? t('app.media.cancelled') : renderProblem(error as never, messages).message;
      chosen = undefined;
    } finally {
      controller = undefined;
    }
  }

  function cancelUpload() {
    controller?.abort();
  }

  async function start() {
    if (!mediaId) return;
    isStarting = true;
    failure = undefined;
    try {
      const sent = kind === 'CSV' ? mappingFor(mapping) : undefined;
      before = new Set(containers.collectionsOf(hubId).map((each) => each.id));
      const accepted = await imports.start({
        media_id: mediaId,
        kind,
        hub_id: hubId,
        ...(sent ? { mapping: sent } : {}),
      });
      jobId = accepted.job_id;
      importId = importIdOf(accepted.result_url);
      go('run');
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isStarting = false;
    }
  }

  const sentence = (code: string | null | undefined) =>
    code ? (messages.has(code) ? t(code) : code) : undefined;

  const columnOptions = $derived([
    { value: '', label: t('app.import.column_none') },
    ...columns.map((column) => ({ value: column, label: column })),
  ]);
</script>

<Dialog bind:isOpen title={t('app.import.title')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="150">
    <h3 class="step" aria-live="polite">{heading}</h3>

    {#if step === 'kind'}
      <Radio
        label={t('app.import.kind')}
        bind:value={kind}
        options={IMPORT_KINDS.map((each) => ({
          value: each,
          label: t(`app.import.kind_${each}`),
          hint: t(`app.import.kind_${each}_hint`),
        }))}
      />
      <Inline gap="100">
        <Button onclick={() => go('file')}>{t('app.import.next')}</Button>
      </Inline>
    {:else if step === 'file'}
      <UploadField
        label={t('app.import.file')}
        hint={t(`app.import.kind_${kind}_hint`)}
        chooseLabel={t('app.import.choose')}
        dropLabel={t('app.media.drop')}
        cancelLabel={t('app.media.cancel')}
        file={chosen}
        {sizeLabel}
        overLimitLabel={overLimit ? t('app.media.over_limit') : undefined}
        progress={isUploading ? percent : undefined}
        progressLabel={isUploading ? t('app.media.uploading', { percent: String(percent) }) : undefined}
        accept={acceptFor(kind)}
        onChoose={(file) => void choose(file)}
        onCancel={cancelUpload}
      />
      {#if mediaId}
        <p class="quiet">{t('app.import.file_ready', { name: chosen?.name ?? '' })}</p>
      {/if}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <Inline gap="100">
        <Button tone="secondary" onclick={() => go('kind')}>{t('app.import.back')}</Button>
        {#if kind === 'CSV'}
          <Button
            disabledReason={mediaId ? undefined : t('app.import.file_first')}
            onclick={() => go('mapping')}
          >
            {t('app.import.next')}
          </Button>
        {:else}
          <Button
            isBusy={isStarting}
            busyLabel={t('app.import.starting')}
            disabledReason={mediaId ? undefined : t('app.import.file_first')}
            onclick={() => void start()}
          >
            {t('app.import.go')}
          </Button>
        {/if}
      </Inline>
    {:else if step === 'mapping'}
      <p class="quiet">
        {columns.length > 0 ? t('app.import.mapping_hint') : t('app.import.no_header')}
      </p>
      {#each CSV_FIELDS as field (field)}
        <Select
          label={t(`app.import.field_${field}`)}
          hint={t(`app.import.field_${field}_hint`)}
          bind:value={mapping[field]}
          options={columnOptions}
        />
      {/each}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <Inline gap="100">
        <Button tone="secondary" onclick={() => go('file')}>{t('app.import.back')}</Button>
        <Button
          isBusy={isStarting}
          busyLabel={t('app.import.starting')}
          disabledReason={mapping.title === '' ? t('app.import.title_needed') : undefined}
          onclick={() => void start()}
        >
          {t('app.import.go')}
        </Button>
      </Inline>
    {:else}
      {#if watch?.watching}
        <ProgressBar
          label={t('app.import.running')}
          value={watch.job.progress === null || watch.job.progress === undefined
            ? undefined
            : Math.round(watch.job.progress * 100)}
          valueLabel={watch.job.progress === null || watch.job.progress === undefined
            ? t('app.import.running')
            : t('app.import.progress', { percent: String(Math.round(watch.job.progress * 100)) })}
        />
      {:else if watch?.unreachable}
        <p class="failure" role="alert">{sentence(watch.unreachable)}</p>
      {/if}

      {#if run}
        {#if run.status === 'SUCCEEDED'}
          <p class="quiet">{t('app.import.done')}</p>
          <RestoreReport report={run.report} />
        {:else if run.status === 'FAILED'}
          <p class="failure" role="alert">{sentence(run.error_code) ?? t('app.import.failed')}</p>
        {/if}

        {#if run.refused && run.refused.length > 0}
          <!-- The rows the converter could not read, by number: the rest of the file landed. -->
          <p class="quiet">{t('app.import.refused_title', { count: String(run.refused.length) })}</p>
          <ul class="refused">
            {#each run.refused as refusal (refusal.row)}
              <li class="quiet">
                {t('app.import.refused_row', { row: String(refusal.row), reason: sentence(refusal.code) ?? refusal.code })}
              </li>
            {/each}
          </ul>
        {/if}
      {/if}

      {#if landed.length > 0}
        <!-- Where it went: the collections under the hub that were not there before the run. -->
        <ul class="landed">
          {#each landed as collection (collection.id)}
            <li>
              <a
                href={`/collections/${collection.id}`}
                onclick={(event) => {
                  event.preventDefault();
                  isOpen = false;
                  onnavigate(`/collections/${collection.id}`);
                }}
              >
                {t('app.import.open_collection', { name: collection.name })}
              </a>
            </li>
          {/each}
        </ul>
      {/if}

      <Inline gap="100">
        <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.import.close')}</Button>
      </Inline>
    {/if}
  </Stack>
</Dialog>

<style>
  .step {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-100);
    font-weight: var(--fw-semibold);
    color: var(--text-secondary);
  }

  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  .refused,
  .landed { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }

  .landed a { color: var(--text-brand); }

  .landed a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-sm);
  }
</style>
