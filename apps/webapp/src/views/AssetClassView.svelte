<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One class of the assets as its record page (page-system §7.2): its attributes, own and
  // inherited, the classes directly beneath it, and the relation types that join it - all from the
  // two lists the classes' screen reads. *Change* opens the class's Dialog L from the head;
  // archiving, the one act that takes a class out of use (the contract has no delete), stands in the
  // danger zone behind a Dialog S. A shipped class is read, never changed (P-05).
  import { untrack } from 'svelte';
  import {
    Button, DataTable, Dialog, EmptyState, RecordPage, Spinner,
    type DataColumn, type MenuItem, type RecordList,
  } from '@hubtask/design-system/components';
  import type { CiAttribute, CiClass, CiClassList, CiRelationType, CiRelationTypeList } from '@hubtask/sync-engine';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import ClassDialog from '../lib/assets/ClassDialog.svelte';
  import { classPath, classesInOrder, classesUnder, typesJoining } from '../lib/assets/classTree.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { classDefine, classDraft, classesPath, classWord, cmdb, relationTypesPath, relationWord, type Words } from '../lib/data/cmdb.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    classKey: string;
    onnavigate: (path: string) => void;
  }
  const { classKey, onnavigate }: Props = $props();
  const LIST = '/assets/classes';
  const words: Words = (code) => (messages.has(code) ? t(code) : undefined);
  const mayShape = $derived(holdsHere('STRUCTURE') === true);

  const classesRead = untrack(() => resource<CiClassList>(classesPath));
  const typesRead = untrack(() => resource<CiRelationTypeList>(relationTypesPath));
  const classes = $derived(classesRead.state.status === 'ready' ? classesInOrder(classesRead.state.data.data) : []);
  const types = $derived(typesRead.state.status === 'ready' ? typesRead.state.data.data : []);
  const refusal = $derived(classesRead.state.status === 'failed' ? renderProblem(classesRead.state.error, messages).message : undefined);
  const isRead = $derived(classesRead.state.status === 'ready');
  const shown = $derived(classes.find((each) => each.key === classKey));
  const nameOf = (key: string) => classWord(key, classes, words);
  const name = $derived(shown ? nameOf(shown.key) : '');
  $effect(() => page.entitle(name || t('app.cmdb_admin.classes')));

  const mayChange = $derived(mayShape && shown !== undefined && !shown.system);
  const beneath = $derived(shown ? classesUnder(classes, shown.key) : []);
  const usedBy = $derived(shown ? typesJoining(types, shown.lineage) : []);
  const ownKeys = $derived(new Set((shown?.attributes ?? []).map((each) => each.key)));
  const attributeName = (key: string) => words(`app.assets.attribute.${key}`) ?? key;

  const facts = $derived(
    shown
      ? [
          shown.parent ? t('app.cmdb_admin.beneath_named', { name: nameOf(shown.parent) }) : '',
          shown.abstract ? t('app.cmdb_admin.abstract') : '',
          shown.system ? t('app.cmdb_admin.shipped') : '',
          t('app.cmdb_admin.staleness', { days: shown.staleness_days }),
        ].filter((part) => part !== '').join(' · ')
      : undefined,
  );

  const lists = $derived<RecordList[]>([
    { id: 'attributes', title: t('app.assets.attributes'), count: shown ? String(shown.all_attributes.length) : undefined },
    { id: 'below', title: t('app.cmdb_admin.col_below'), count: String(beneath.length) },
    { id: 'used', title: t('app.cmdb_admin.used_by'), count: typesRead.state.status === 'ready' ? String(usedBy.length) : undefined },
  ]);
  const attributeColumns = $derived<DataColumn[]>([
    { id: 'key', label: t('app.cmdb_admin.col_attribute'), priority: 1 },
    { id: 'kind', label: t('app.cmdb_admin.kind_label'), priority: 2 },
    { id: 'need', label: t('app.cmdb_admin.need_label'), priority: 2 },
    { id: 'from', label: t('app.cmdb_admin.col_from'), priority: 3 },
  ]);
  const belowColumns = $derived<DataColumn[]>([
    { id: 'class', label: t('app.cmdb_admin.col_class'), priority: 1 },
    { id: 'attributes', label: t('app.assets.attributes'), priority: 2 },
  ]);
  const typeColumns = $derived<DataColumn[]>([
    { id: 'type', label: t('app.cmdb_admin.col_type'), priority: 1 },
    { id: 'pairs', label: t('app.cmdb_admin.pairs'), priority: 2 },
  ]);

  let isEditing = $state(false);
  let isArchiving = $state(false);
  let archiveFailure = $state<string | undefined>(undefined);
  const mayArchive = $derived(mayChange && shown?.archived !== true);
  const menuItems = $derived<MenuItem[]>(mayArchive ? [{ id: 'archive', label: t('app.cmdb_admin.archive_action'), isDestructive: true }] : []);

  async function archive() {
    const of = shown;
    if (!of) return;
    archiveFailure = undefined;
    try {
      await cmdb.defineClass(of.key, classDefine({ ...classDraft(of), archived: true }), of.version);
      toast.show({ text: t('app.cmdb_admin.archived_toast', { name }), tone: 'success' });
      isArchiving = false;
    } catch (error) {
      archiveFailure = renderProblem(error as never, messages).message;
    }
  }
  function openArchive() {
    archiveFailure = undefined;
    isArchiving = true;
  }
</script>

<RecordPage
  record={{
    kind: t('app.cmdb_admin.col_class'),
    title: name,
    back: { label: t('app.cmdb_admin.classes'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: shown?.archived ? { label: t('app.cmdb_admin.archived'), tone: 'neutral' } : undefined,
    facts,
    edit: mayChange ? { label: t('app.page.edit'), opener: 'edit-class', onclick: () => (isEditing = true) } : undefined,
    menu: menuItems.length > 0 ? { label: t('app.page.more_for', { name }), items: menuItems, onselect: openArchive } : undefined,
  }}
  {lists}
  danger={mayArchive
    ? {
        title: t('app.page.danger_zone'),
        acts: [{ id: 'archive', title: t('app.cmdb_admin.archive_title'), cost: t('app.cmdb_admin.archive_cost'), actionLabel: t('app.cmdb_admin.archive_action') }],
        onact: openArchive,
      }
    : undefined}
  refused={refusal ? failed : isRead && !shown ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if !isRead}
      <Spinner label={t('app.assets.loading')} />
    {:else if item.id === 'attributes'}
      <DataTable label={item.title} columns={attributeColumns} rows={shown?.all_attributes ?? []} key={(each: CiAttribute) => each.key} rowLabel={(each: CiAttribute) => attributeName(each.key)}
        sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')}>
        {#snippet cell(each: CiAttribute, column: DataColumn)}
          {#if column.id === 'key'}<span data-class-attribute={each.key}>{attributeName(each.key)}</span>{#if each.identifier}<span class="quiet">{` · ${t('app.cmdb_admin.identifying')}`}</span>{/if}
          {:else if column.id === 'kind'}{t(`app.cmdb_admin.kind.${each.kind}`)}
          {:else if column.id === 'need'}{t(`app.cmdb_admin.need.${each.need ?? 'OPTIONAL'}`)}
          {:else}<span class="quiet">{ownKeys.has(each.key) ? t('app.cmdb_admin.own') : t('app.cmdb_admin.inherited')}</span>{/if}
        {/snippet}
        {#snippet empty()}<EmptyState kind="settled" title={t('app.cmdb_admin.no_attributes')} />{/snippet}
      </DataTable>
    {:else if item.id === 'below'}
      <DataTable label={item.title} columns={belowColumns} rows={beneath} key={(each: CiClass) => each.key} rowLabel={(each: CiClass) => nameOf(each.key)}
        href={(each: CiClass) => classPath(each.key)} sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')}>
        {#snippet cell(each: CiClass, column: DataColumn)}
          {#if column.id === 'class'}<span data-class-below={each.key}>{nameOf(each.key)}</span>
          {:else}{t('app.cmdb_admin.attribute_count', { count: each.all_attributes.length })}{/if}
        {/snippet}
        {#snippet empty()}<EmptyState kind="settled" title={t('app.cmdb_admin.no_below')} />{/snippet}
      </DataTable>
    {:else}
      <DataTable label={item.title} columns={typeColumns} rows={usedBy} key={(each: CiRelationType) => each.key} rowLabel={(each: CiRelationType) => relationWord(each, each.key, true, words)}
        sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')}>
        {#snippet cell(each: CiRelationType, column: DataColumn)}
          {#if column.id === 'type'}<span data-class-used={each.key}>{relationWord(each, each.key, true, words)}</span>
          {:else}{each.pairs.map((pair) => t('app.cmdb_admin.pair', { from: nameOf(pair.from), to: nameOf(pair.to) })).join(', ')}{/if}
        {/snippet}
        {#snippet empty()}<EmptyState kind="settled" title={t('app.cmdb_admin.not_used')} />{/snippet}
      </DataTable>
    {/if}
  {/snippet}
</RecordPage>

{#snippet notHere()}<EmptyState kind="filtered" title={t('app.cmdb_admin.class_unknown')} />{/snippet}
{#snippet failed()}<FailureNote problem={refusal ?? ''} />{/snippet}

<ClassDialog bind:isOpen={isEditing} editing={shown} {classes} {types} />

{#if isArchiving && shown}
  <Dialog bind:isOpen={isArchiving} title={t('app.cmdb_admin.archive_confirm', { name })} dismissLabel={t('app.assets.cancel')} size="s">
    <p class="body">{t('app.cmdb_admin.archive_cost')}</p>
    {#if archiveFailure}<FailureNote problem={archiveFailure} />{/if}
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isArchiving = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="danger" onclick={() => void archive()}>{t('app.cmdb_admin.archive_confirm_action')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { color: var(--text-subtle); }
  .body { margin: 0; }
</style>
