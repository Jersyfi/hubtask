<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The assets' *Classes* (M6, UC-AST-02 check 6, the navigation concept §4): the class hierarchy
  // Hubtask ships with the workspace's own classes beneath it, as one table drawn as a tree - each
  // deeper branch folded until opened. An administrator adds a class (*New class*, a dialog L) and
  // changes the workspace's own; a shipped class is read, never changed.
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE at the workspace the
  // page lists the classes and offers no control.
  import {
    Button, Checkbox, CheckboxGroup, DataTable, Dialog, EmptyState, FilterBar, IconButton, Input, ListPage, Select, Spinner, Stack, Switch, Textarea,
    type DataColumn, type DataRowMenu, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { CiAttribute, CiClass, CiClassList, CiRelationTypeList } from '@hubtask/sync-engine';
  import { untrack } from 'svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { holdsHere, reachesAdministrationArea } from '../lib/data/capability.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import ProductTabs from '../lib/product/ProductTabs.svelte';
  import { productHead } from '../lib/product/frame.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { classesBelow, shownClasses } from '../lib/assets/classTree.ts';
  import { classDefine, classDraft, classesPath, classWord, cmdb, keyIsWellFormed, relationTypesPath, relationWord, type ClassDraft, type Words } from '../lib/data/cmdb.svelte.ts';

  interface Props {
    onnavigate?: (path: string) => void;
  }
  const { onnavigate }: Props = $props();
  $effect(() => page.entitle(t('app.cmdb_admin.classes')));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const words: Words = (code) => (messages.has(code) ? t(code) : undefined);

  const classesRead = untrack(() => resource<CiClassList>(classesPath));
  const typesRead = untrack(() => resource<CiRelationTypeList>(relationTypesPath));
  const classes = $derived(classesRead.state.status === 'ready' ? classesRead.state.data.data : []);
  const types = $derived(typesRead.state.status === 'ready' ? typesRead.state.data.data : []);
  const refusal = $derived(classesRead.state.status === 'failed' ? renderProblem(classesRead.state.error, messages).message : undefined);
  const nameOf = (key: string) => classWord(key, classes, words);
  const depthOf = (each: CiClass) => Math.max(0, each.lineage.length - 1);
  /** The hierarchy in reading order: each class after its parent, siblings by their order in the list. */
  const tree = $derived.by(() => {
    const out: CiClass[] = [];
    const visit = (parent: string) => {
      for (const each of classes.filter((one) => one.parent === parent)) {
        out.push(each);
        visit(each.key);
      }
    };
    for (const root of classes.filter((each) => !each.parent)) {
      out.push(root);
      visit(root.key);
    }
    return out;
  });
  // The families open, each deeper branch folded until opened (§5.4); a search shows every match.
  let opened = $state<ReadonlySet<string>>(new Set());
  const below = $derived(classesBelow(tree));
  let search = $state('');
  const shownTree = $derived(search.trim() === '' ? shownClasses(tree, opened) : tree.filter((each) => matchesSearch(search, [nameOf(each.key), each.key])));
  function toggle(key: string) {
    const next = new Set(opened);
    if (!next.delete(key)) next.add(key);
    opened = next;
  }
  const classOptions = $derived<SelectOption[]>(tree.map((each) => ({ value: each.key, label: nameOf(each.key) })));
  const kinds: readonly CiAttribute['kind'][] = ['TEXT', 'NUMBER', 'BOOLEAN', 'DATE', 'CHOICE', 'URL', 'GROUP', 'ACCOUNT'];
  const kindOptions = $derived<SelectOption[]>(kinds.map((each) => ({ value: each, label: t(`app.cmdb_admin.kind.${each}`) })));
  const needOptions = $derived<SelectOption[]>((['OPTIONAL', 'RECOMMENDED', 'REQUIRED'] as const).map((each) => ({ value: each, label: t(`app.cmdb_admin.need.${each}`) })));
  const typeKeys = $derived<SelectOption[]>(types.map((each) => ({ value: each.key, label: relationWord(each, each.key, true, words) })));

  // One dialog is open at a time; its refusal is said inside it, which keeps what was typed
  // (user-experience.md §5.10). An action outside a dialog says it in a toast.
  let dialogFailure = $state<string | undefined>(undefined);
  async function act<T>(run: () => Promise<T>, done: string, inDialog = false): Promise<T | undefined> {
    if (inDialog) dialogFailure = undefined;
    try {
      const out = await run();
      toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      const said = renderProblem(error as never, messages).message;
      if (inDialog) dialogFailure = said;
      else toast.show({ text: said, tone: 'danger' });
      return undefined;
    }
  }

  // --- A class ---------------------------------------------------------------------------------

  let classEditing = $state<CiClass | undefined>(undefined);
  let classDraftState = $state<ClassDraft | undefined>(undefined);
  let isClassOpen = $state(false);

  function openClass(of?: CiClass) {
    classEditing = of;
    classDraftState = classDraft(of);
    dialogFailure = undefined;
    isClassOpen = true;
  }

  const classReason = $derived(
    !classDraftState ? undefined
    : !classEditing && !keyIsWellFormed(classDraftState.key) ? t('app.cmdb_admin.key_hint')
    : !classDraftState.name.trim() ? t('app.cmdb_admin.needs_name')
    : undefined,
  );

  async function saveClass() {
    const draft = classDraftState;
    if (!draft) return;
    const done = await act(() => cmdb.defineClass(classEditing?.key ?? draft.key, classDefine(draft), classEditing?.version), t('app.cmdb_admin.saved'), true);
    if (done) isClassOpen = false;
  }

  const columns = $derived<DataColumn[]>([
    { id: 'class', label: t('app.cmdb_admin.col_class'), priority: 1 },
    { id: 'attributes', label: t('app.assets.attributes'), priority: 2 },
    { id: 'stale', label: t('app.cmdb_admin.col_stale'), priority: 3 },
    { id: 'below', label: t('app.cmdb_admin.col_below'), priority: 3 },
  ]);
  const rowMenu = (each: CiClass): DataRowMenu | undefined =>
    mayShape && !each.system ? { label: t('app.page.more_for', { name: nameOf(each.key) }), items: [{ id: 'edit', label: t('app.cmdb_admin.edit_named', { name: nameOf(each.key) }) }] } : undefined;
  const primary = $derived(mayShape ? { label: t('app.cmdb_admin.new_class'), icon: 'plus' as const, onclick: () => openClass() } : undefined);
</script>

{#snippet strip()}<ProductTabs module="ASSETS" current="classes" {onnavigate} />{/snippet}

<ListPage head={{ ...productHead('ASSETS'), primary }} {strip} refused={reachesAdministrationArea() === false ? notYours : undefined}>
  {#snippet intro()}<p class="meta">{t('app.cmdb_admin.classes_hint')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar searchLabel={t('app.cmdb_admin.search_classes')} clearSearchLabel={t('app.page.clear_search')} bind:search
      countLabel={classesRead.state.status === 'ready' ? t('app.page.found', { shown: String(shownTree.length), count: String(tree.length) }) : undefined} />
  {/snippet}
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if classesRead.state.status !== 'ready'}
    <Spinner label={t('app.assets.loading')} />
  {:else}
    <DataTable label={t('app.cmdb_admin.classes')} {columns} rows={shownTree} key={(each: CiClass) => each.key} rowLabel={(each: CiClass) => nameOf(each.key)}
      onopen={mayShape ? (each: CiClass) => { if (!each.system) openClass(each); } : undefined}
      menu={rowMenu} onmenu={(each: CiClass) => openClass(each)}
      actionsLabel={t('app.page.row_actions')} sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')}>
      <!-- The fold stands before the title, outside the press that opens the row: inside it, it
           was a button within a button, under the press's stretched box. -->
      {#snippet lead(each: CiClass)}
        {@const holds = depthOf(each) > 0 ? (below.get(each.key) ?? 0) : 0}
        <span class="indent" data-depth={String(Math.min(depthOf(each), 6))}></span>
        {#if holds > 0 && search.trim() === ''}
          <IconButton icon={opened.has(each.key) ? 'chevron-down' : 'chevron-right'} size="sm" tone="subtle"
            label={t(opened.has(each.key) ? 'app.cmdb_admin.fold_named' : 'app.cmdb_admin.open_named', { name: nameOf(each.key), count: holds })}
            aria-expanded={opened.has(each.key)} data-class-toggle onclick={() => toggle(each.key)} />
        {/if}
      {/snippet}
      {#snippet cell(each: CiClass, column: DataColumn)}
        {@const holds = depthOf(each) > 0 ? (below.get(each.key) ?? 0) : 0}
        {#if column.id === 'class'}
          <span class="named" data-class-row={each.key}>
            <span>{nameOf(each.key)}</span>
            <span class="meta" data-class-meta>{[
              each.abstract ? t('app.cmdb_admin.abstract') : '',
              each.system ? t('app.cmdb_admin.shipped') : '',
              each.archived ? t('app.cmdb_admin.archived') : '',
            ].filter((part) => part !== '').join(' · ')}</span>
          </span>
        {:else if column.id === 'attributes'}{t('app.cmdb_admin.attribute_count', { count: each.all_attributes.length })}
        {:else if column.id === 'stale'}{t('app.cmdb_admin.staleness', { days: each.staleness_days })}
        {:else}{holds > 0 ? t('app.cmdb_admin.classes_below', { count: holds }) : '—'}{/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

<!-- Offered to whoever reaches the administration, as the door before them was (P-05). -->
{#snippet notYours()}<EmptyState kind="filtered" title={t('app.settings.not_yours')} />{/snippet}

{#if isClassOpen && classDraftState}
  <Dialog bind:isOpen={isClassOpen} title={classEditing ? t('app.cmdb_admin.edit_named', { name: nameOf(classEditing.key) }) : t('app.cmdb_admin.new_class')} dismissLabel={t('app.assets.cancel')} size="l">
    <Stack gap="150">
      {#if !classEditing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={classDraftState.key} maxlength={63} />{/if}
      <Input label={t('app.assets.name_label')} bind:value={classDraftState.name} maxlength={120} />
      <Select label={t('app.cmdb_admin.parent')} options={classOptions.filter((each) => each.value !== classEditing?.key)} bind:value={classDraftState.parent} />
      <Switch label={t('app.cmdb_admin.abstract')} bind:checked={classDraftState.abstract} />
      <fieldset class="group">
        <legend>{t('app.assets.attributes')}</legend>
        {#each classDraftState.attributes as attribute, index (index)}
          <div class="line">
            <Input label={t('app.cmdb_admin.key')} size="sm" bind:value={attribute.key} maxlength={63} />
            <Select label={t('app.cmdb_admin.kind_label')} size="sm" options={kindOptions} bind:value={attribute.kind} />
            <Select label={t('app.cmdb_admin.need_label')} size="sm" options={needOptions} bind:value={attribute.need} />
            <Checkbox label={t('app.cmdb_admin.identifying')} bind:checked={attribute.identifier} />
            {#if attribute.kind === 'CHOICE'}<Textarea label={t('app.cmdb_admin.options')} rows={2} bind:value={attribute.options} />{/if}
            <IconButton icon="x" size="sm" tone="subtle" label={t('app.cmdb_admin.remove')} onclick={() => classDraftState?.attributes.splice(index, 1)} />
          </div>
        {/each}
        <div><Button size="sm" tone="subtle" icon="plus" onclick={() => classDraftState?.attributes.push({ key: '', kind: 'TEXT', need: 'OPTIONAL', identifier: false, options: '' })}>{t('app.cmdb_admin.add_attribute')}</Button></div>
      </fieldset>
      <Textarea label={t('app.cmdb_admin.rules')} hint={t('app.cmdb_admin.rules_hint')} rows={3} bind:value={classDraftState.rules} />
      <fieldset class="group">
        <legend>{t('app.cmdb_admin.required')}</legend>
        {#each classDraftState.required as needed, index (index)}
          <div class="line">
            <CheckboxGroup label={t('app.cmdb_admin.required_types')} options={typeKeys.map((each) => ({ value: each.value, label: each.label }))} selected={needed.types} onChange={(next) => (needed.types = [...next])} />
            <Checkbox label={t('app.cmdb_admin.outgoing')} bind:checked={needed.outgoing} />
            <IconButton icon="x" size="sm" tone="subtle" label={t('app.cmdb_admin.remove')} onclick={() => classDraftState?.required.splice(index, 1)} />
          </div>
        {/each}
        <div><Button size="sm" tone="subtle" icon="plus" onclick={() => classDraftState?.required.push({ types: [], outgoing: true })}>{t('app.cmdb_admin.add_required')}</Button></div>
      </fieldset>
      <Input label={t('app.cmdb_admin.staleness_label')} hint={t('app.cmdb_admin.staleness_hint')} inputmode="numeric" bind:value={classDraftState.staleness} />
      {#if classEditing}<Switch label={t('app.cmdb_admin.archived')} bind:checked={classDraftState.archived} />{/if}
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isClassOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={classReason} onclick={() => void saveClass()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .named { display: inline-flex; align-items: center; gap: var(--sp-050); }
  .indent[data-depth='1'] { inline-size: var(--sp-150); }
  .indent[data-depth='2'] { inline-size: var(--sp-300); }
  .indent[data-depth='3'] { inline-size: var(--sp-400); }
  .indent[data-depth='4'] { inline-size: var(--sp-500); }
  .indent[data-depth='5'],
  .indent[data-depth='6'] { inline-size: var(--sp-600); }
  .group { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .line { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
