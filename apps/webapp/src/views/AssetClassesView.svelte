<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The assets' *Classes* (M6, UC-AST-02 check 6, the navigation concept §4): the class hierarchy
  // Hubtask ships with the workspace's own classes beneath it, as one table drawn as a tree - each
  // deeper branch folded until opened. A class opens its record page; an administrator adds a class
  // (*New class*, a dialog L) and changes the workspace's own; a shipped class is read, never changed.
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE at the workspace the
  // page lists the classes and offers no control.
  import { DataTable, EmptyState, FilterBar, IconButton, ListPage, Spinner, type DataColumn, type DataRowMenu } from '@hubtask/design-system/components';
  import type { CiClass, CiClassList, CiRelationTypeList } from '@hubtask/sync-engine';
  import { untrack } from 'svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { holdsHere, reachesAdministrationArea } from '../lib/data/capability.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import ProductTabs from '../lib/product/ProductTabs.svelte';
  import { productHead } from '../lib/product/frame.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ClassDialog from '../lib/assets/ClassDialog.svelte';
  import { classPath, classesBelow, classesInOrder, shownClasses } from '../lib/assets/classTree.ts';
  import { classesPath, classWord, relationTypesPath, type Words } from '../lib/data/cmdb.svelte.ts';

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
  const tree = $derived(classesInOrder(classes));
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
  // --- A class ---------------------------------------------------------------------------------

  let classEditing = $state<CiClass | undefined>(undefined);
  let isClassOpen = $state(false);

  function openClass(of?: CiClass) {
    classEditing = of;
    isClassOpen = true;
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
      href={(each: CiClass) => classPath(each.key)}
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

<ClassDialog bind:isOpen={isClassOpen} editing={classEditing} classes={tree} {types} />

<style>
  .named { display: inline-flex; align-items: center; gap: var(--sp-050); }
  .indent[data-depth='1'] { inline-size: var(--sp-150); }
  .indent[data-depth='2'] { inline-size: var(--sp-300); }
  .indent[data-depth='3'] { inline-size: var(--sp-400); }
  .indent[data-depth='4'] { inline-size: var(--sp-500); }
  .indent[data-depth='5'],
  .indent[data-depth='6'] { inline-size: var(--sp-600); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
