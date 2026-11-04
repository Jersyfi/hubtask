<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Import… on a hub, or in a collection's add split: one anatomy of four steps in one dialog
  // (user-experience.md §7.11). On a hub the file becomes collections under it; in a collection its
  // entries land there, and the dialog says so before the source is chosen.
  //
  // **Where from, the file, the check, the run.** The sources are cards in families, each with one
  // sentence; one press chooses and moves on. The file goes through the media upload staged as
  // `IMPORT`, claiming the kind's type rather than the browser's guess, its size limit said before
  // choosing. The check is the mapping where the source has columns (`CSV`: the header row read
  // here, each field prefilled where a header names one) and a dry run of the same file
  // (`dry_run`), said in words through `PlanSummary` - never a key or a path. Changing the mapping
  // makes the plan stale until it is checked again. Then the import itself, its job followed the
  // way every job is, and its report said in the same words.
  //
  // **A second import of the same file is a no-op**, and the check already says so: everything is
  // left as it is. The dry run keeps the file, so the import names the same object.
  //
  // **A kind this build refuses answers by name** (`imports.kind_unsupported`) and that answer is
  // what the dialog shows; the manifest declares no import capability and nothing is hidden here.
  //
  // **Each step is announced**: the heading changes and the announcer says it.

  import {
    Button,
    Inline,
    PlanSummary,
    ProgressBar,
    Select,
    Stack,
    TemplateCard,
    TemplateGallery,
    UploadField,
    VisuallyHidden,
  } from '@hubtask/design-system/components';
  import PanelOrDialog from '../workspace/PanelOrDialog.svelte';
  import { untrack } from 'svelte';

  import { announcer } from '../announce.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { imports } from '../data/imports.svelte.ts';
  import {
    CSV_FIELDS,
    SOURCE_FAMILIES,
    acceptFor,
    contentTypeFor,
    importIdOf,
    kindsFor,
    mappingFor,
    markOf,
    parseHeader,
    planOf,
    prefill,
    type CsvField,
    type ImportKind,
  } from '../data/imports.ts';
  import { jobs, type Watch } from '../data/jobs.svelte.ts';
  import { isTerminal } from '../data/jobs.ts';
  import { namesOf } from '../data/kudos.ts';
  import { media } from '../data/media.svelte.ts';
  import { isWithinUploadLimit, uploadLimitOf } from '../data/media.ts';
  import { TOUCHES_ANY_ENTRY } from '../data/touches.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { focusFirst } from '../forms/missing.ts';

  interface Props {
    /** Drawn inside capture's *Import a file* mode rather than as a dialog of its own. */
    isEmbedded?: boolean;
    isOpen?: boolean;
    /** The hub the imported collections land under, or the hub of the collection. */
    hubId: string;
    /** The collection the entries land in, where the import starts from one. */
    collection?: { readonly id: string; readonly name: string };
    /** Where a collection that landed is opened. */
    onnavigate: (path: string) => void;
  }

  let { isEmbedded = false, isOpen = $bindable(false), hubId, collection, onnavigate }: Props = $props();

  type Step = 'kind' | 'file' | 'check' | 'run';
  const STEPS: readonly Step[] = ['kind', 'file', 'check', 'run'];

  let step = $state<Step>('kind');
  let kind = $state<ImportKind>('CSV');
  let chosen = $state<File | undefined>(undefined);
  let columns = $state<readonly string[]>([]);
  let mapping = $state<Record<CsvField, string>>(prefill([]));
  let sent = $state(0);
  let controller = $state<AbortController | undefined>(undefined);
  let mediaId = $state<string | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  /** The dry run: its job, its run, and the mapping it was asked with. */
  let checkJobId = $state<string | undefined>(undefined);
  let checkId = $state<string | undefined>(undefined);
  let checkedMapping = $state<string | undefined>(undefined);
  let isChecking = $state(false);
  /** The import itself. */
  let jobId = $state<string | undefined>(undefined);
  let importId = $state<string | undefined>(undefined);
  let isStarting = $state(false);
  /**
   * What *Next* or *Import* found missing when it was pressed - the file, or the column that
   * carries the title - said at that field (design-system.md §4, case 3). A file still on its way
   * is not missing: the button is busy until it has landed.
   */
  let missing = $state<'file' | 'title' | undefined>(undefined);
  let stepBody = $state<HTMLElement | null>(null);

  /** Moves on only with what the step needs; otherwise marks it and takes focus there. */
  function needs(what: 'file' | 'title', isThere: boolean, then: () => void) {
    if (isThere) {
      missing = undefined;
      then();
      return;
    }
    missing = what;
    queueMicrotask(() => focusFirst(stepBody));
  }
  /** The hub's collections before the run, so that what landed can be told from what was there. */
  let before = $state<ReadonlySet<string>>(new Set());

  const limit = $derived(uploadLimitOf(manifest.value));
  const isUploading = $derived(controller !== undefined);
  const overLimit = $derived(chosen !== undefined && !isWithinUploadLimit(chosen.size, limit));
  const percent = $derived(chosen && chosen.size > 0 ? Math.floor((sent / chosen.size) * 100) : 0);
  const checkWatch = $derived<Watch | undefined>(checkJobId ? jobs.of(checkJobId) : undefined);
  const check = $derived(checkId ? imports.of(checkId) : undefined);
  const watch = $derived<Watch | undefined>(jobId ? jobs.of(jobId) : undefined);
  const run = $derived(importId ? imports.of(importId) : undefined);
  const heading = $derived(
    t('app.import.step', { number: String(STEPS.indexOf(step) + 1), of: String(STEPS.length), name: t(`app.import.step_${step}`) }),
  );
  const families = $derived(
    SOURCE_FAMILIES.map((family) => ({
      family: family.family,
      kinds: family.kinds.filter((each) => kindsFor(collection !== undefined).includes(each)),
    })).filter((family) => family.kinds.length > 0),
  );

  const mappingKey = $derived(kind === 'CSV' ? JSON.stringify(mappingFor(mapping) ?? {}) : '');
  /** The plan no longer says what the import would do: the mapping changed since it was checked. */
  const isStale = $derived(checkedMapping !== undefined && checkedMapping !== mappingKey);
  const isCheckRunning = $derived(isChecking || checkWatch?.watching === true || (checkId !== undefined && check === undefined));
  const fileHint = $derived(
    limit === undefined
      ? t(`app.import.kind_${kind}_hint`)
      : `${t(`app.import.kind_${kind}_hint`)} ${t('app.import.file_limit', { limit: formatBytes(limit, messages.locale) })}`,
  );

  const words = $derived({
    t: (code: string, params?: Record<string, string>) => t(code, params),
    list: (parts: readonly string[]) => namesOf(parts, messages.locale),
  });
  const checkPlan = $derived(check?.status === 'SUCCEEDED' ? planOf(check, true, words) : []);
  const runPlan = $derived(run?.status === 'SUCCEEDED' ? planOf(run, false, words) : []);
  const where = $derived(
    collection ? t('app.import.reach_collection', { name: collection.name }) : t('app.import.reach_hub', { name: containers.find(hubId)?.name ?? '' }),
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

  /** The collections that landed: under the hub now and not before the run. None into a collection. */
  const landed = $derived(
    run?.status === 'SUCCEEDED' && collection === undefined
      ? containers.collectionsOf(hubId).filter((each) => !before.has(each.id))
      : [],
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
      forgetCheck();
      if (jobId) jobs.forget(jobId);
      jobId = undefined;
      importId = undefined;
    });
  });

  // A finished dry run: its plan has arrived. Read once; nothing was written, so nothing is read again.
  $effect(() => {
    const current = checkWatch;
    if (!current || current.watching || !isTerminal(current.job.status)) return;
    const id = checkId;
    if (!id) return;
    void untrack(async () => {
      await imports.read(id);
      const done = imports.of(id);
      if (done !== undefined && done.dry_run !== true) {
        // An instance from before the dry run, met during a rolling update, imported instead of
        // checking: what it did is said as the import, never as a plan.
        before = new Set(containers.collectionsOf(hubId).map((each) => each.id));
        if (collection === undefined) await containers.refreshLevel(hubId);
        else engine.landed([...TOUCHES_ANY_ENTRY, `/containers/${collection.id}`]);
        importId = id;
        go('run');
        return;
      }
      announcer.say(done?.status === 'SUCCEEDED' ? t('app.import.checked_announced') : t('app.import.check_failed'));
    });
  });

  // A finished import means the report has arrived. Read it once; the watcher already stopped.
  $effect(() => {
    const current = watch;
    if (!current || current.watching || !isTerminal(current.job.status)) return;
    const id = importId;
    if (!id) return;
    void untrack(async () => {
      await imports.read(id);
      const done = imports.of(id);
      if (done?.status === 'SUCCEEDED') {
        // The job wrote without a change log entry, so what shows what landed is read again here:
        // the hub's level, or the collection's entries, columns and labels.
        if (collection === undefined) await containers.refreshLevel(hubId);
        else engine.landed([...TOUCHES_ANY_ENTRY, `/containers/${collection.id}`]);
      }
      announcer.say(done?.status === 'SUCCEEDED' ? t('app.import.done_announced') : t('app.import.failed_announced'));
    });
  });

  function forgetCheck() {
    if (checkJobId) jobs.forget(checkJobId);
    checkJobId = undefined;
    checkId = undefined;
    checkedMapping = undefined;
  }

  function go(next: Step) {
    step = next;
    failure = undefined;
    announcer.say(
      t('app.import.step', { number: String(STEPS.indexOf(next) + 1), of: String(STEPS.length), name: t(`app.import.step_${next}`) }),
    );
  }

  function pick(next: ImportKind) {
    if (next !== kind) {
      // Another source is another file: what was uploaded for the first is not offered to it.
      chosen = undefined;
      mediaId = undefined;
      columns = [];
      mapping = prefill([]);
      forgetCheck();
    }
    kind = next;
    go('file');
  }

  async function choose(file: File) {
    chosen = file;
    failure = undefined;
    sent = 0;
    mediaId = undefined;
    columns = [];
    forgetCheck();
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
        contentType: contentTypeFor(kind, file.name),
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

  function request(isDry: boolean) {
    const sent = kind === 'CSV' ? mappingFor(mapping) : undefined;
    return {
      media_id: mediaId ?? '',
      kind,
      hub_id: hubId,
      ...(collection ? { collection_id: collection.id } : {}),
      ...(sent ? { mapping: sent } : {}),
      dry_run: isDry,
    };
  }

  /** The check: the same import as a dry run, so the plan is the server's, not a guess here. */
  async function startCheck() {
    if (!mediaId) return;
    if (kind === 'CSV' && mapping.title === '') return;
    forgetCheck();
    isChecking = true;
    failure = undefined;
    try {
      const asked = mappingKey;
      const accepted = await imports.start(request(true));
      checkJobId = accepted.job_id;
      checkId = importIdOf(accepted.result_url);
      checkedMapping = asked;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isChecking = false;
    }
  }

  function toCheck() {
    go('check');
    // A source without columns has nothing to set first: its check starts at once. A CSV's
    // starts too when the header already names the title.
    if (kind !== 'CSV' || mapping.title !== '') void startCheck();
  }

  async function start() {
    if (!mediaId) return;
    isStarting = true;
    failure = undefined;
    try {
      before = new Set(containers.collectionsOf(hubId).map((each) => each.id));
      const accepted = await imports.start(request(false));
      jobId = accepted.job_id;
      importId = importIdOf(accepted.result_url);
      go('run');
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isStarting = false;
    }
  }

  /** A code in words; one without words of its own is said generically, never shown as a key. */
  const sentence = (code: string | null | undefined, otherwise = 'app.import.failed') =>
    code ? (messages.has(code) ? t(code) : t(otherwise)) : undefined;

  const columnOptions = $derived([
    { value: '', label: t('app.import.column_none') },
    ...columns.map((column) => ({ value: column, label: column })),
  ]);
</script>

{#snippet refusedRows(rows: NonNullable<typeof run>['refused'])}
  {#if rows && rows.length > 0}
    <!-- The rows that did not land, by number: unreadable, or refused by their type's form with
         the entries under them - so a row may appear twice. -->
    <ul class="refused">
      {#each rows as refusal, index (index)}
        <li class="quiet">
          {#if refusal.fields && refusal.fields.length > 0}
            {t('app.import.refused_row_fields', {
              row: String(refusal.row),
              reason: sentence(refusal.code, 'app.import.refused_other') ?? '',
              fields: refusal.fields.join(', '),
            })}
          {:else}
            {t('app.import.refused_row', { row: String(refusal.row), reason: sentence(refusal.code, 'app.import.refused_other') ?? '' })}
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
{/snippet}

<PanelOrDialog {isEmbedded} bind:isOpen title={t('app.import.title')} dismissLabel={t('app.workspace.cancel')}>
  <div bind:this={stepBody} data-import-step={step}>
  <Stack gap="150">
    <h3 class="step" aria-live="polite">{heading}</h3>

    {#if step === 'kind'}
      {#if collection}
        <p class="quiet">{t('app.import.into', { name: collection.name })}</p>
      {/if}
      {#each families as family (family.family)}
        <TemplateGallery label={t(`app.import.family_${family.family}`)}>
          {#each family.kinds as each (each)}
            <TemplateCard name={t(`app.import.kind_${each}`)} summary={t(`app.import.kind_${each}_card`)} mark={markOf(each)} isCompact data-import-kind={each}>
              {#snippet action()}
                <Button size="sm" tone={each === kind && mediaId ? 'primary' : 'secondary'} onclick={() => pick(each)}>
                  {t('app.import.choose_source')}<VisuallyHidden>{` ${t(`app.import.kind_${each}`)}`}</VisuallyHidden>
                </Button>
              {/snippet}
            </TemplateCard>
          {/each}
        </TemplateGallery>
      {/each}
    {:else if step === 'file'}
      <UploadField
        label={t('app.import.file_of', { source: t(`app.import.kind_${kind}`) })}
        hint={fileHint}
        chooseLabel={t('app.import.choose')}
        dropLabel={t('app.media.drop')}
        cancelLabel={t('app.media.cancel')}
        file={chosen}
        {sizeLabel}
        overLimitLabel={overLimit ? t('app.media.over_limit') : undefined}
        progress={isUploading ? percent : undefined}
        progressLabel={isUploading ? t('app.media.uploading', { percent: String(percent) }) : undefined}
        accept={acceptFor(kind)}
        error={missing === 'file' && !chosen ? t('app.import.file_first') : undefined}
        onChoose={(file) => {
          missing = undefined;
          void choose(file);
        }}
        onCancel={cancelUpload}
      />
      {#if mediaId}
        <p class="quiet">{t('app.import.file_ready', { name: chosen?.name ?? '' })}</p>
      {/if}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <Inline gap="100">
        <Button tone="secondary" onclick={() => go('kind')}>{t('app.import.back')}</Button>
        <Button
          tone="primary"
          isBusy={isUploading}
          busyLabel={t('app.media.uploading', { percent: String(percent) })}
          onclick={() => needs('file', mediaId !== undefined, toCheck)}
        >
          {t('app.import.next')}
        </Button>
      </Inline>
    {:else if step === 'check'}
      {#if kind === 'CSV'}
        <p class="quiet">
          {columns.length > 0 ? t('app.import.mapping_hint') : t('app.import.no_header')}
        </p>
        <div class="mapping">
          {#each CSV_FIELDS as field (field)}
            <Select
              label={t(`app.import.field_${field}`)}
              hint={t(`app.import.field_${field}_hint`)}
              bind:value={mapping[field]}
              error={field === 'title' && missing === 'title' && mapping.title === '' ? t('app.import.title_needed') : undefined}
              options={columnOptions}
            />
          {/each}
        </div>
      {/if}

      {#if isCheckRunning}
        <ProgressBar label={t('app.import.checking')} valueLabel={t('app.import.checking')} />
      {:else if check?.status === 'SUCCEEDED'}
        <PlanSummary
          label={t('app.import.plan_label_dry')}
          lines={checkPlan}
          emptyLabel={t('app.import.plan_empty')}
          reach={isStale ? t('app.import.plan_stale') : `${where} ${t('app.import.plan_nothing_yet')}`}
        />
        {@render refusedRows(check.refused)}
      {:else if check?.status === 'FAILED'}
        <p class="failure" role="alert">{sentence(check.error_code) ?? t('app.import.check_failed')}</p>
      {:else if checkWatch?.unreachable}
        <p class="failure" role="alert">{sentence(checkWatch.unreachable)}</p>
      {/if}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

      <Inline gap="100">
        <Button tone="secondary" onclick={() => go('file')}>{t('app.import.back')}</Button>
        {#if kind === 'CSV' && (check === undefined || isStale) && !isCheckRunning}
          <Button
            tone="secondary"
            onclick={() => needs('title', mapping.title !== '', () => void startCheck())}
          >
            {check === undefined ? t('app.import.check') : t('app.import.check_again')}
          </Button>
        {/if}
        {#if check?.status !== 'FAILED'}
          <Button
            tone="primary"
            isBusy={isStarting}
            busyLabel={t('app.import.starting')}
            onclick={() => needs('title', kind !== 'CSV' || mapping.title !== '', () => void start())}
          >
            {t('app.import.go')}
          </Button>
        {/if}
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
          <p class="quiet">{t('app.import.done_announced')}</p>
          <PlanSummary label={t('app.import.plan_label_done')} lines={runPlan} emptyLabel={t('app.import.plan_empty_done')} reach={where} />
        {:else if run.status === 'FAILED'}
          <p class="failure" role="alert">{sentence(run.error_code) ?? t('app.import.failed')}</p>
        {/if}
        {@render refusedRows(run.refused)}
      {/if}

      {#if landed.length > 0}
        <!-- Open what arrived: the collections under the hub that were not there before the run. -->
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
  </div>
</PanelOrDialog>

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

  /* The seven fields two by two where there is room, one under the other on a phone. */
  .mapping { display: grid; gap: var(--sp-150); grid-template-columns: repeat(auto-fit, minmax(min(100%, 28ch), 1fr)); }

  .refused,
  .landed { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }

  .landed a { color: var(--text-brand); }

  .landed a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-sm);
  }
</style>
