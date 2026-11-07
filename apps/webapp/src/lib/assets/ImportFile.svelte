<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A file brought into the inventory (UC-AST-04 check 5), in the import's four steps
  // (user-experience.md §7.11): where from - the file sources an administrator defined, as cards -,
  // the file with its limit said before choosing, the check - a CSV table's or JSON array's columns
  // mapped to the class's attributes (a source's saved mapping first; a workbook's first sheet read
  // as such a table, check 6) and a dry run said in words (`PlanSummary`) with its decisions row by
  // row -, and the import with its report. All through the reconciliation of UC-AST-02 as the
  // source chosen. Only a member who may write sees it.
  //
  // **A changed mapping makes the check stale**: the plan no longer says what the import would do
  // until it is checked again.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Inline, PlanSummary, Select, Stack, TemplateCard, TemplateGallery, UploadField, VisuallyHidden,
    type PlanLine, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { CiClass, CiFileImport, CiFileMapping, CiRun, CiSource, CiSourceList } from '@hubtask/sync-engine';

  import { classWord, cmdb, fileColumns, fileFormat, fileText, guessMapping, instantiable, sourcesPath, type Words } from '../data/cmdb.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { base64Of, isZip, workbookColumns } from '../data/workbook.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import Decisions from './Decisions.svelte';

  /** What the contract takes (`CiFileImport.content`); a larger file is refused before it is sent. */
  const MAX_BYTES = 5 * 1024 * 1024;
  /** A workbook travels as base64, a third larger than its bytes. */
  const MAX_WORKBOOK_BYTES = Math.floor((MAX_BYTES * 3) / 4);

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    /** Where what arrived is seen: the inventory. */
    onopen?: () => void;
  }

  const { hubId, classes, words, onopen }: Props = $props();

  type Step = 'kind' | 'file' | 'check' | 'run';
  const STEPS: readonly Step[] = ['kind', 'file', 'check', 'run'];
  let step = $state<Step>('kind');
  const heading = $derived(
    t('app.import.step', { number: String(STEPS.indexOf(step) + 1), of: String(STEPS.length), name: t(`app.import.step_${step}`) }),
  );
  function go(next: Step) {
    step = next;
    failure = undefined;
    announcer.say(t('app.import.step', { number: String(STEPS.indexOf(next) + 1), of: String(STEPS.length), name: t(`app.import.step_${next}`) }));
  }

  let sources = $state<readonly CiSource[]>([]);
  $effect(() =>
    untrack(() =>
      engine.subscribe<CiSourceList>({ path: sourcesPath }, (state) => {
        if (state.status === 'ready') sources = state.data.data.filter((each) => each.kind !== 'MANUAL');
      }),
    ),
  );

  let chosenFile = $state<File | null>(null);
  let content = $state('');
  let format = $state<CiFileImport['format']>('CSV');
  let source = $state('');
  let mapClass = $state('');
  let classColumn = $state('');
  let nameColumn = $state('');
  let keyColumn = $state('');
  let columnTo = $state<Record<string, string>>({});
  /** The dry run, and the request it answered: a mapping changed since makes it stale. */
  let check = $state<CiRun | undefined>(undefined);
  let checkedKey = $state<string | undefined>(undefined);
  let run = $state<CiRun | undefined>(undefined);
  let isBusy = $state(false);
  let isTooLarge = $state(false);
  let missing = $state(false);
  let failure = $state<string | undefined>(undefined);

  const tabular = $derived(format === 'CSV' || format === 'JSON' || format === 'XLSX');
  // A workbook's columns are read once from its bytes; a text's from the text as it stands.
  let sheetColumns = $state<string[]>([]);
  const columns = $derived(!content || !tabular ? [] : format === 'XLSX' ? sheetColumns : fileColumns(format, content));
  const chosen = $derived(classes.find((each) => each.key === mapClass));
  const columnOptions = $derived<SelectOption[]>([{ value: '', label: t('app.assets.import_none') }, ...columns.map((each) => ({ value: each, label: each }))]);
  const attributeOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.assets.import_ignore') },
    ...(chosen?.all_attributes ?? []).map((each) => ({
      value: each.key,
      label: messages.has(`app.assets.attribute.${each.key}`) ? t(`app.assets.attribute.${each.key}`) : each.key,
    })),
  ]);

  async function readFile(file: File) {
    run = undefined;
    check = undefined;
    checkedKey = undefined;
    missing = false;
    chosenFile = file;
    content = '';
    const bytes = new Uint8Array(await file.arrayBuffer());
    const workbook = /\.xlsx$/iu.test(file.name) || isZip(bytes);
    isTooLarge = file.size > (workbook ? MAX_WORKBOOK_BYTES : MAX_BYTES);
    if (isTooLarge) return;
    if (workbook) {
      sheetColumns = await workbookColumns(bytes);
      content = base64Of(bytes);
      format = 'XLSX';
    } else {
      sheetColumns = [];
      content = fileText(bytes);
      format = fileFormat(file.name, content);
    }
    adopt();
  }

  /** The source's saved mapping, else one guessed from the columns. */
  function adopt() {
    const saved = sources.find((each) => each.key === source)?.mapping;
    if (saved) {
      mapClass = saved.class;
      classColumn = saved.class_column ?? '';
      nameColumn = saved.name_column;
      keyColumn = saved.native_key_column ?? '';
      columnTo = { ...(saved.columns ?? {}) };
      return;
    }
    const guess = guessMapping(columns, chosen?.all_attributes ?? []);
    nameColumn = guess.name ?? nameColumn;
    keyColumn = guess.nativeKey ?? keyColumn;
    columnTo = { ...guess.columns, ...columnTo };
  }

  const mapping = $derived<CiFileMapping | undefined>(
    tabular && mapClass && nameColumn
      ? {
          class: mapClass,
          name_column: nameColumn,
          ...(classColumn ? { class_column: classColumn } : {}),
          ...(keyColumn ? { native_key_column: keyColumn } : {}),
          columns: Object.fromEntries(Object.entries(columnTo).filter(([column, key]) => key && columns.includes(column))),
        }
      : undefined,
  );

  const reason = $derived(
    !source ? t('app.assets.import_needs_source')
    : !content ? t('app.assets.import_needs_file')
    : tabular && !mapClass ? t('app.assets.needs_class')
    : tabular && !nameColumn ? t('app.assets.import_needs_name_column')
    : undefined,
  );

  const requestKey = $derived(JSON.stringify({ source, format, mapping: mapping ?? null }));
  const isStale = $derived(checkedKey !== undefined && checkedKey !== requestKey);

  async function send(dryRun: boolean) {
    isBusy = true;
    failure = undefined;
    const asked = requestKey;
    try {
      const answer = await cmdb.importFile(hubId, { source, format, content, dry_run: dryRun, ...(mapping ? { mapping } : {}) });
      if (dryRun) {
        check = answer;
        checkedKey = asked;
        announcer.say(t('app.import.checked_announced'));
      } else {
        run = answer;
        go('run');
      }
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }

  function pick(key: string) {
    if (key !== source) {
      chosenFile = null;
      content = '';
      check = undefined;
      checkedKey = undefined;
    }
    source = key;
    adopt();
    go('file');
  }

  /** The file being read: *Next* pressed meanwhile waits for it rather than calling it missing. */
  let reading: Promise<void> | undefined;

  async function toCheck() {
    await reading;
    if (!content) {
      missing = true;
      return;
    }
    adopt();
    go('check');
    // What needs no mapping, or whose mapping is already whole, is checked at once.
    if (reason === undefined) void send(true);
  }

  function again() {
    chosenFile = null;
    content = '';
    check = undefined;
    checkedKey = undefined;
    run = undefined;
    go('kind');
  }

  const formats = $derived<SelectOption[]>((format === 'XLSX' ? (['XLSX'] as const) : (['CSV', 'JSON', 'CYCLONEDX', 'SPDX'] as const)).map((each) => ({ value: each, label: t(`app.assets.format.${each}`) })));
  /** A source's card sentence: what it reads the columns by. */
  function sourceSummary(each: CiSource): string {
    return each.mapping
      ? t('app.assets.import_source_mapped', { class: classWord(each.mapping.class, classes, words) })
      : t('app.assets.import_source_unmapped');
  }
  const classOptions = $derived<SelectOption[]>(instantiable(classes).map((each) => ({ value: each.key, label: classWord(each.key, classes, words) })));

  /** A run's counts as a plan in words (user-experience.md §7.11), a line for each that is not zero. */
  const PLAN_TONES = { created: 'add', updated: 'change', unchanged: 'keep', held: 'refuse', duplicates: 'refuse', refused: 'refuse' } as const;
  function planOf(answer: NonNullable<typeof run>): PlanLine[] {
    const tense = answer.dry_run ? 'dry' : 'done';
    return (Object.keys(PLAN_TONES) as (keyof typeof PLAN_TONES)[])
      .filter((outcome) => Number(answer.counts[outcome] ?? 0) > 0)
      .map((outcome) => ({ tone: PLAN_TONES[outcome], text: t(`app.assets.plan_${outcome}_${tense}`, { count: String(answer.counts[outcome]) }) }));
  }
</script>

<Stack gap="150">
  <h2 class="step" aria-live="polite">{heading}</h2>

  {#if step === 'kind'}
    {#if sources.length === 0}
      <Banner tone="info">{t('app.assets.import_needs_source')}</Banner>
    {:else}
      <TemplateGallery label={t('app.assets.import_source')} isLabelHidden>
        {#each sources as each (each.key)}
          <TemplateCard name={each.key} summary={sourceSummary(each)} mark="file-text" isCompact data-import-source={each.key}>
            {#snippet action()}
              <Button size="sm" tone={each.key === source && content ? 'primary' : 'secondary'} onclick={() => pick(each.key)}>
                {t('app.import.choose_source')}<VisuallyHidden>{` ${each.key}`}</VisuallyHidden>
              </Button>
            {/snippet}
          </TemplateCard>
        {/each}
      </TemplateGallery>
    {/if}
  {:else if step === 'file'}
    <UploadField
      label={t('app.assets.import_file_of', { source })}
      hint={t('app.assets.import_hint')}
      chooseLabel={t('app.import.choose')}
      dropLabel={t('app.media.drop')}
      accept=".csv,.json,.xlsx,text/csv,application/json,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
      file={chosenFile}
      sizeLabel={chosenFile ? t('app.media.size', { size: formatBytes(chosenFile.size, messages.locale) }) : undefined}
      overLimitLabel={isTooLarge ? t('app.assets.import_too_large') : undefined}
      error={missing && !content ? t('app.import.file_first') : undefined}
      onChoose={(file) => (reading = readFile(file))}
    />
    <Inline gap="100">
      <Button tone="secondary" onclick={() => go('kind')}>{t('app.import.back')}</Button>
      <Button tone="primary" onclick={() => void toCheck()}>{t('app.import.next')}</Button>
    </Inline>
  {:else if step === 'check'}
    <Select label={t('app.assets.import_format')} options={formats} bind:value={format} />
    {#if tabular}
      <fieldset class="mapping">
        <legend>{t('app.assets.class_label')}</legend>
        <Select label={t('app.assets.class_label')} options={classOptions} placeholder={t('app.assets.needs_class')} bind:value={mapClass} onchange={adopt} />
        <Select label={t('app.assets.import_class_column')} options={columnOptions} bind:value={classColumn} />
        <Select label={t('app.assets.import_name_column')} options={columnOptions} bind:value={nameColumn} />
        <Select label={t('app.assets.import_key_column')} options={columnOptions} bind:value={keyColumn} />
        {#each columns.filter((each) => each !== nameColumn && each !== keyColumn && each !== classColumn) as column (column)}
          <Select label={t('app.assets.import_column_to', { column })} options={attributeOptions} bind:value={() => columnTo[column] ?? '', (key: string) => (columnTo[column] = key)} />
        {/each}
      </fieldset>
    {/if}
    {#if isBusy && !check}
      <p class="quiet" role="status">{t('app.import.checking')}</p>
    {:else if check}
      <section class="part" aria-labelledby="import-preview">
        <h3 id="import-preview" class="heading">{t('app.assets.import_preview')}</h3>
        <PlanSummary label={t('app.assets.import_preview')} lines={planOf(check)} emptyLabel={t('app.assets.plan_nothing')} reach={isStale ? t('app.import.plan_stale') : t('app.import.plan_nothing_yet')} />
        {#if check.skipped_rows?.length}<Banner tone="warning">{t('app.assets.import_skipped', { rows: check.skipped_rows.join(', ') })}</Banner>{/if}
        <Decisions decisions={check.decisions ?? []} {classes} {words} />
      </section>
    {/if}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
    <Inline gap="100">
      <Button tone="secondary" onclick={() => go('file')}>{t('app.import.back')}</Button>
      {#if check === undefined || isStale}
        <Button tone="secondary" disabledReason={reason} isBusy={isBusy} onclick={() => void send(true)}>{check === undefined ? t('app.import.check') : t('app.import.check_again')}</Button>
      {/if}
      <Button tone="primary" disabledReason={reason} isBusy={isBusy} onclick={() => void send(false)}>{t('app.assets.import_run')}</Button>
    </Inline>
  {:else}
    {#if run}
      <section class="part" aria-labelledby="import-report">
        <h3 id="import-report" class="heading">{t('app.assets.import_did')}</h3>
        <PlanSummary label={t('app.assets.import_did')} lines={planOf(run)} emptyLabel={t('app.assets.plan_nothing')} />
        {#if run.skipped_rows?.length}<Banner tone="warning">{t('app.assets.import_skipped', { rows: run.skipped_rows.join(', ') })}</Banner>{/if}
        <Decisions decisions={run.decisions ?? []} {classes} {words} />
      </section>
    {/if}
    <Inline gap="100">
      <Button tone="secondary" onclick={again}>{t('app.assets.import_another')}</Button>
      {#if onopen}<Button tone="primary" onclick={onopen}>{t('app.assets.import_open_inventory')}</Button>{/if}
    </Inline>
  {/if}
</Stack>

<style>
  .mapping { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .part { display: grid; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .step { margin: 0; font-family: var(--font-display); font-size: var(--fs-100); font-weight: var(--fw-semibold); color: var(--text-secondary); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }
</style>
