<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A file brought into the inventory (UC-AST-04 check 5): the file read in the browser, its format
  // found, a CSV table's or JSON array's columns mapped to the class's attributes (a source's saved
  // mapping first), a dry run's decisions shown row by row, and then the import itself - all through
  // the reconciliation of UC-AST-02 as the source chosen. Only a member who may write sees it.

  import { untrack } from 'svelte';
  import { Banner, Button, Select, Stack, type SelectOption } from '@hubtask/design-system/components';
  import type { CiClass, CiFileImport, CiFileMapping, CiRun, CiSource, CiSourceList } from '@hubtask/sync-engine';

  import { classWord, cmdb, fileColumns, fileFormat, guessMapping, instantiable, sourcesPath, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import Decisions from './Decisions.svelte';

  /** What the contract takes (`CiFileImport.content`); a larger file is refused before it is sent. */
  const MAX_BYTES = 5 * 1024 * 1024;

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

  let fileName = $state('');
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

  const tabular = $derived(format === 'CSV' || format === 'JSON');
  const columns = $derived(content && tabular ? fileColumns(format, content) : []);
  const chosen = $derived(classes.find((each) => each.key === mapClass));
  const columnOptions = $derived<SelectOption[]>([{ value: '', label: t('app.assets.import_none') }, ...columns.map((each) => ({ value: each, label: each }))]);
  const attributeOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.assets.import_ignore') },
    ...(chosen?.all_attributes ?? []).map((each) => ({
      value: each.key,
      label: messages.has(`app.assets.attribute.${each.key}`) ? t(`app.assets.attribute.${each.key}`) : each.key,
    })),
  ]);

  async function readFile(event: Event) {
    const file = (event.currentTarget as HTMLInputElement).files?.[0];
    run = undefined;
    if (!file) return;
    if (file.size > MAX_BYTES) {
      toast.show({ text: t('app.assets.import_too_large'), tone: 'danger' });
      return;
    }
    fileName = file.name;
    content = await file.text();
    format = fileFormat(file.name, content);
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

  const formats = $derived<SelectOption[]>((['CSV', 'JSON', 'CYCLONEDX', 'SPDX'] as const).map((each) => ({ value: each, label: t(`app.assets.format.${each}`) })));
  const sourceOptions = $derived<SelectOption[]>(sources.map((each) => ({ value: each.key, label: each.key })));
  const classOptions = $derived<SelectOption[]>(instantiable(classes).map((each) => ({ value: each.key, label: classWord(each.key, classes, words) })));
</script>

<Stack gap="150">
  {#if sources.length === 0}
    <Banner tone="info">{t('app.assets.import_needs_source')}</Banner>
  {:else}
    <Select label={t('app.assets.import_source')} options={sourceOptions} placeholder={t('app.assets.import_source')} bind:value={source} onchange={adopt} />
  {/if}
  <label class="file">
    <span>{t('app.assets.import_file')}</span>
    <input type="file" accept=".csv,.json,text/csv,application/json" onchange={readFile} />
    <span class="meta">{fileName || t('app.assets.import_hint')}</span>
  </label>
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
        <Select label={t('app.assets.import_column_to', { column })} options={attributeOptions} bind:value={columnTo[column]} />
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
  .file { display: grid; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); }
  .mapping { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .buttons { display: flex; flex-wrap: wrap; gap: var(--sp-100); justify-content: flex-end; }
  .part { display: grid; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
