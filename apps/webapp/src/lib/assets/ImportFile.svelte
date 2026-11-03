<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A file brought into the inventory (UC-AST-04 check 5): the file read in the browser, its format
  // found, a CSV table's or JSON array's columns mapped to the class's attributes (a source's saved
  // mapping first) - a workbook's first sheet read as such a table (check 6) -, a dry run's decisions shown row by row, and then the import itself - all through
  // the reconciliation of UC-AST-02 as the source chosen. Only a member who may write sees it.

  import { untrack } from 'svelte';
  import { Banner, Button, Select, Stack, UploadField, type SelectOption } from '@hubtask/design-system/components';
  import type { CiClass, CiFileImport, CiFileMapping, CiRun, CiSource, CiSourceList } from '@hubtask/sync-engine';

  import { classWord, cmdb, fileColumns, fileFormat, guessMapping, instantiable, sourcesPath, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { base64Of, isZip, workbookColumns } from '../data/workbook.ts';
  import { toast } from '../frame/toasts.svelte.ts';
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
  }

  const { hubId, classes, words }: Props = $props();

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
  let run = $state<CiRun | undefined>(undefined);
  let isBusy = $state(false);

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
    const bytes = new Uint8Array(await file.arrayBuffer());
    const workbook = /\.xlsx$/iu.test(file.name) || isZip(bytes);
    if (file.size > (workbook ? MAX_WORKBOOK_BYTES : MAX_BYTES)) {
      toast.show({ text: t('app.assets.import_too_large'), tone: 'danger' });
      return;
    }
    chosenFile = file;
    if (workbook) {
      sheetColumns = await workbookColumns(bytes);
      content = base64Of(bytes);
      format = 'XLSX';
    } else {
      sheetColumns = [];
      content = new TextDecoder().decode(bytes);
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

  async function send(dryRun: boolean) {
    isBusy = true;
    try {
      const answer = await cmdb.importFile(hubId, { source, format, content, dry_run: dryRun, ...(mapping ? { mapping } : {}) });
      run = answer;
      if (!dryRun) toast.show({ text: t('app.assets.import_done', { created: answer.counts.created, updated: answer.counts.updated }), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isBusy = false;
    }
  }

  const formats = $derived<SelectOption[]>((format === 'XLSX' ? (['XLSX'] as const) : (['CSV', 'JSON', 'CYCLONEDX', 'SPDX'] as const)).map((each) => ({ value: each, label: t(`app.assets.format.${each}`) })));
  const sourceOptions = $derived<SelectOption[]>(sources.map((each) => ({ value: each.key, label: each.key })));
  const classOptions = $derived<SelectOption[]>(instantiable(classes).map((each) => ({ value: each.key, label: classWord(each.key, classes, words) })));
</script>

<Stack gap="150">
  {#if sources.length === 0}
    <Banner tone="info">{t('app.assets.import_needs_source')}</Banner>
  {:else}
    <Select label={t('app.assets.import_source')} options={sourceOptions} placeholder={t('app.assets.import_source')} bind:value={source} onchange={adopt} />
  {/if}
  <UploadField
    label={t('app.assets.import_file')}
    hint={t('app.assets.import_hint')}
    chooseLabel={t('app.import.choose')}
    dropLabel={t('app.media.drop')}
    accept=".csv,.json,.xlsx,text/csv,application/json,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
    file={chosenFile}
    onChoose={(file) => void readFile(file)}
  />
  {#if content}
    <Select label={t('app.assets.import_format')} options={formats} bind:value={format} />
  {/if}
  {#if content && tabular}
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
  <div class="buttons">
    <Button tone="secondary" disabledReason={reason} isBusy={isBusy} onclick={() => void send(true)}>{t('app.assets.import_dry_run')}</Button>
    <Button tone="primary" disabledReason={reason} isBusy={isBusy} onclick={() => void send(false)}>{t('app.assets.import_run')}</Button>
  </div>

  {#if run}
    <section class="part" aria-labelledby="import-preview">
      <h2 id="import-preview" class="heading">{run.dry_run ? t('app.assets.import_preview') : t('app.assets.decisions')}</h2>
      <p class="meta">{t('app.assets.run_counts', {
        created: run.counts.created, updated: run.counts.updated, unchanged: run.counts.unchanged,
        held: run.counts.held, duplicates: run.counts.duplicates, refused: run.counts.refused,
      })}</p>
      {#if run.skipped_rows?.length}<Banner tone="warning">{t('app.assets.import_skipped', { rows: run.skipped_rows.join(', ') })}</Banner>{/if}
      <Decisions decisions={run.decisions ?? []} {classes} {words} />
    </section>
  {/if}
</Stack>

<style>
  .mapping { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .buttons { display: flex; flex-wrap: wrap; gap: var(--sp-100); justify-content: flex-end; }
  .part { display: grid; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
