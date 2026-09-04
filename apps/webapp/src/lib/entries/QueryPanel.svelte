<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The query language, offered: the conditions, the order, and — on a board — what the columns
  // are. Everything it offers comes from `/meta/capabilities` through `lib/data/query.ts`, and
  // nothing in this file names a field.
  //
  // It builds the **document** as well as the controls, because the two have to agree: a condition
  // the editor allowed and the document dropped would be a filter the reader can see and the
  // server never received. `filterOf` is the one place that decides, and this hands it the rows.
  //
  // The operators are the one thing here that becomes words. `EQ` is an identifier on the wire and
  // a sentence on the screen, so each has a code in the catalogue (ADR-0011) — and an operator the
  // catalogue has never heard of still renders, as `humanise` makes of it, because the grammar can
  // grow without this client.

  import { QueryBuilder, Select, ViewSwitcher, type QueryCondition, type View } from '@hubtask/design-system/components';

  import { manifest } from '../data/capabilities.svelte.ts';
  import type { ItemsQuery } from '../data/items.svelte.ts';
  import {
    filterOf,
    filterableFields,
    groupOf,
    groupableFields,
    sortOf,
    sortableFields,
    takesList,
    takesValue,
  } from '../data/query.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Which layout the reader chose. Kept on the device by the caller; saved views are F3's. */
    layout: string;
    /** The layouts this client can actually draw, in the order it offers them. */
    drawable: readonly string[];
    onlayout: (id: string) => void;
    /** The query, rebuilt whenever a control changes. */
    onquery: (query: ItemsQuery) => void;
  }

  const { layout, drawable, onlayout, onquery }: Props = $props();

  /**
   * The layouts, as the installation reports them.
   *
   * Reported and drawable is offered; reported and not drawable is offered **with the reason**,
   * because leaving it out would make the switcher disagree with the manifest. A layout this
   * client draws and the installation does not report is not shown at all — that is the
   * installation saying it does not have it, which is not this client's to override.
   */
  const views = $derived<View[]>(
    (manifest.value?.view_layouts ?? []).map((id) => ({
      id,
      label: t(`app.view.${id}`),
      unavailableReason: drawable.includes(id)
        ? undefined
        : t('app.view.not_built', { layout: t(`app.view.${id}`) }),
    })),
  );

  let isOpen = $state(false);
  let conditions = $state<QueryCondition[]>([]);
  /** The manual order is the empty choice: `order_key ASC` is the query's own default. */
  let sortField = $state('');
  let sortDir = $state<'ASC' | 'DESC'>('ASC');
  let groupField = $state('bucket_id');

  const fields = $derived(
    filterableFields(manifest.value).map((field) => ({
      // The field's own name, drawn as the installation reports it. There is no code for it and
      // there cannot be: the set grows with the installation, so a catalogue entry per field would
      // be a catalogue that is wrong on the one that has another (the same reasoning the entry
      // type chooser records).
      id: field.field,
      label: field.field,
      operators: (field.operators ?? [])
        // `IS_NULL` on a field that cannot be absent means nothing, and the contract says so on
        // `nullable`. Offering it would be offering a row that is never sent.
        .filter((op) => op !== 'IS_NULL' || field.nullable)
        .map((op) => ({
          id: op,
          label: t(`app.query.op.${op}`),
          takesValue: takesValue(op),
          hint: takesList(op) ? t('app.query.list_hint') : t('app.query.placeholder_hint'),
        })),
      values: field.values?.map((value) => ({ value, label: value })),
    })),
  );

  const sorts = $derived(sortableFields(manifest.value));
  const groups = $derived(groupableFields(manifest.value));

  /**
   * The document, rebuilt from the controls.
   *
   * Every part goes through `query.ts`, which drops what the manifest does not report — so a
   * control that somehow offered an unknown field would still not send one.
   */
  function publish() {
    onquery({
      filter: filterOf(manifest.value, conditions),
      sort: sortField === '' ? undefined : sortOf(manifest.value, sortField, sortDir),
      group: layout === 'KANBAN' ? groupOf(manifest.value, groupField) : undefined,
    });
  }

  function clear() {
    conditions = [];
    sortField = '';
    sortDir = 'ASC';
    publish();
  }

  // The grouping belongs to the board, so changing the layout changes the document.
  $effect(() => {
    void layout;
    publish();
  });
</script>

<div class="panel">
  <div class="bar">
    <ViewSwitcher label={t('app.view.label')} {views} selected={layout} onselect={onlayout} />
    <button type="button" class="toggle" aria-expanded={isOpen} onclick={() => (isOpen = !isOpen)}>
      {isOpen ? t('app.query.hide') : t('app.query.show')}
    </button>
  </div>

  {#if isOpen}
    <QueryBuilder
      label={t('app.query.title')}
      {fields}
      bind:conditions
      fieldLabel={t('app.query.field')}
      operatorLabel={t('app.query.operator')}
      valueLabel={t('app.query.value')}
      addLabel={t('app.query.add')}
      removeLabel={t('app.query.remove')}
      emptyLabel={t('app.query.none_filterable')}
      onchange={publish}
    />

    <div class="ordering">
      <Select
        label={t('app.query.sort_by')}
        size="sm"
        bind:value={sortField}
        placeholder={t('app.query.sort_manual')}
        options={sorts.map((field) => ({ value: field.field, label: field.field }))}
        onchange={publish}
      />
      <Select
        label={t('app.query.direction')}
        size="sm"
        bind:value={sortDir}
        options={[
          { value: 'ASC', label: t('app.query.ascending') },
          { value: 'DESC', label: t('app.query.descending') },
        ]}
        onchange={publish}
      />
      <!-- Only where the columns are a question. On a list there is no grouping to choose. -->
      {#if layout === 'KANBAN' && groups.length > 1}
        <Select
          label={t('app.query.group_by')}
          size="sm"
          bind:value={groupField}
          options={groups.map((field) => ({ value: field.field, label: field.field }))}
          onchange={publish}
        />
      {/if}
      <button type="button" class="toggle" onclick={clear}>{t('app.query.clear')}</button>
    </div>
  {/if}
</div>

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-150); }

  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); }

  .ordering { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .toggle {
    padding: var(--density-row-block) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .toggle:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  .toggle:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }
</style>
