<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The query language, offered: the conditions, the order, and — on a board — what the columns
  // are. Everything it offers comes from `/meta/capabilities` through `lib/data/query.ts`, and
  // nothing in this file names a field.
  //
  // The layout switch is not here: it is the page head's second row
  // (`LayoutSwitch`), and this panel is what the head's "Filter" button opens - inline from
  // `expanded`, in a drawer below it. The panel stays mounted either way, because the conditions
  // are its state and a closed filter is still a filter; it reports how many parts are set so the
  // button can say so.
  //
  // It builds the **document** as well as the controls, because the two have to agree: a condition
  // the editor allowed and the document dropped would be a filter the reader can see and the
  // server never received. `filterOf` is the one place that decides, and this hands it the rows.
  //
  // The operators are the one thing here that becomes words. `EQ` is an identifier on the wire and
  // a sentence on the screen, so each has a code in the catalogue (ADR-0011) — and an operator the
  // catalogue has never heard of still renders, as `humanise` makes of it, because the grammar can
  // grow without this client.

  import { QueryBuilder, Select, type QueryCondition } from '@hubtask/design-system/components';

  import { manifest } from '../data/capabilities.svelte.ts';
  import type { ItemsQuery } from '../data/items.svelte.ts';
  import type { FilterNode } from '@hubtask/sync-engine';
  import { untrack } from 'svelte';

  import type { FilterField } from '../data/query.ts';
  import {
    conditionsOf,
    filterOf,
    filterableFields,
    offeredByProfile,
    groupOf,
    groupableFields,
    isSendable,
    sortOf,
    takesList,
    takesValue,
  } from '../data/query.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { entryTypes, typeOrder, typesShown } from '../data/entrytypes.svelte.ts';

  interface Props {
    /** Which layout the reader chose, because the grouping belongs to the board. */
    layout: string;
    /** The query, rebuilt whenever a control changes. */
    onquery: (query: ItemsQuery) => void;
    /** How many parts of the query are set - the conditions and an order - for the button that opens this. */
    oncount?: (count: number) => void;
    /**
     * The custom fields in force where these entries live, as fields a condition can name.
     *
     * A second source rather than a second editor: `custom_fields.<key>` is deliberately not in
     * `query_fields` — "which keys exist is `/custom-fields`' answer" — so the caller, which knows
     * which collection is on screen, reads the definitions and hands them in. Empty is the honest
     * default: a screen that spans collections has no one set of definitions in force.
     */
    custom?: readonly FilterField[];
    /**
     * A question applied from outside - a saved view opened from the *Views* menu. The controls
     * take it over, so the next change or a change of layout starts from it rather than replacing
     * it with what the controls held before.
     */
    applied?: ItemsQuery;
    /**
     * Whether any level of the collection carries a capability: the fields its profile gives no
     * meaning to are not offered (`offeredByProfile`). Absent, the installation's answer stands.
     */
    carried?: (capability: string) => boolean;
    /**
     * The order, owned by the toolbar's own *Sort* menu (design-system.md §11.1) and bound here,
     * because the order is part of the one query this panel sends. The manual order is the empty
     * choice: `order_key ASC` is the query's own default.
     */
    sortField?: string;
    sortDir?: 'ASC' | 'DESC';
  }

  let {
    layout,
    onquery,
    oncount,
    custom = [],
    applied,
    carried,
    sortField = $bindable(''),
    sortDir = $bindable('ASC'),
  }: Props = $props();

  /** The query field of an entry's type (`view.FieldEntryType`). */
  const ENTRY_TYPE = 'entry_type';

  let conditions = $state<QueryCondition[]>([]);
  let groupField = $state('bucket_id');
  /**
   * An applied filter the conditions cannot draw (`conditionsOf`), kept whole and combined with
   * whatever the reader adds, until the controls are cleared.
   */
  let kept = $state<FilterNode | undefined>(undefined);

  // The entry type is a value only where the workspace shows types (UC-FLW-01 check 8); there its
  // keys are the workspace's, so the values come from `/entry-types`, named as the list names them.
  const showsTypes = $derived(typesShown(manifest.value));
  $effect(() => {
    if (!showsTypes) return;
    return untrack(() => entryTypes.open());
  });
  const typeValues = $derived(
    typeOrder(entryTypes.all).map((type) => ({ value: type.key, label: type.name ?? t('app.entry_types.level', { level: type.level }) })),
  );
  const shown = <T extends { field: string }>(each: readonly T[]) => (showsTypes ? each : each.filter((field) => field.field !== ENTRY_TYPE));

  const fields = $derived(
    shown(offeredByProfile(filterableFields(manifest.value, custom), carried)).map((field) => ({
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
      values: field.field === ENTRY_TYPE && typeValues.length > 0 ? typeValues : field.values?.map((value) => ({ value, label: value })),
    })),
  );

  const groups = $derived(shown(offeredByProfile(groupableFields(manifest.value, custom), carried)));

  /**
   * The document, rebuilt from the controls.
   *
   * Every part goes through `query.ts`, which drops what the manifest does not report — so a
   * control that somehow offered an unknown field would still not send one.
   */
  function publish() {
    const built = filterOf(manifest.value, conditions, custom);
    onquery({
      filter: kept && built ? { op: 'AND', nodes: [kept, built] } : (kept ?? built),
      sort: sortField === '' ? undefined : sortOf(manifest.value, sortField, sortDir, custom),
      group: layout === 'KANBAN' ? groupOf(manifest.value, groupField, custom) : undefined,
    });
    // The filter's count is its conditions; the order says itself on the *Sort* button.
    oncount?.(conditions.length + (kept ? 1 : 0));
  }

  function clear() {
    kept = undefined;
    conditions = [];
    sortField = '';
    sortDir = 'ASC';
    publish();
  }

  // Before the publishing effect, so that a view opened together with its layout is published
  // once, as itself.
  $effect(() => {
    if (!applied) return;
    const conditionsDrawn = conditionsOf(applied.filter);
    // Drawn only when every condition is one the controls would send again; a view's filter the
    // server validated is never thinned out by this client's manifest.
    const drawn = conditionsDrawn?.every((each) => isSendable(manifest.value, each, custom)) ? conditionsDrawn : undefined;
    untrack(() => {
      conditions = drawn ?? [];
      kept = drawn ? undefined : applied.filter;
      sortField = applied.sort?.[0]?.field ?? '';
      sortDir = applied.sort?.[0]?.dir ?? 'ASC';
      if (applied.group) groupField = applied.group.field;
    });
  });

  // The grouping belongs to the board, so changing the layout changes the document; the order
  // comes from the *Sort* menu, so a change there does too. And the manifest: every part is
  // checked against it, so a document built before it arrived is one without its grouping - a
  // board opened at its own address drew empty columns until a reload.
  $effect(() => {
    void manifest.value;
    void custom;
    void layout;
    void sortField;
    void sortDir;
    untrack(publish);
  });
</script>

<div class="panel">
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
</div>

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-150); }

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
