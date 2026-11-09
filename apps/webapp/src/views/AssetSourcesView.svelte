<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The assets' *Sources* (M6, UC-AST-06 check 5, the navigation concept §4): where configuration
  // items come from, by precedence - a lower rank wins. An administrator adds a source and changes
  // the workspace's own; the shipped *manual* is read, never changed (P-05: without STRUCTURE the
  // page lists the sources and offers no control).
  import {
    CheckboxGroup, DataTable, Dialog, EmptyState, FilterBar, Input, ListPage, Select, Spinner, Stack, Switch, Button,
    type DataColumn, type DataRowMenu, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { CiClass, CiClassList, CiSource, CiSourceDefine, CiSourceList } from '@hubtask/sync-engine';
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
  import { classesPath, classWord, cmdb, keyIsWellFormed, sourcesPath, type Words } from '../lib/data/cmdb.svelte.ts';

  interface Props {
    onnavigate?: (path: string) => void;
  }
  const { onnavigate }: Props = $props();
  $effect(() => page.entitle(t('app.cmdb_admin.sources')));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const words: Words = (code) => (messages.has(code) ? t(code) : undefined);

  const classesRead = untrack(() => resource<CiClassList>(classesPath));
  const sourcesRead = untrack(() => resource<CiSourceList>(sourcesPath));
  const classes = $derived<readonly CiClass[]>(classesRead.state.status === 'ready' ? classesRead.state.data.data : []);
  const sources = $derived(sourcesRead.state.status === 'ready' ? [...sourcesRead.state.data.data].sort((a, b) => a.rank - b.rank) : []);
  const refusal = $derived(sourcesRead.state.status === 'failed' ? renderProblem(sourcesRead.state.error, messages).message : undefined);
  const nameOf = (key: string) => classWord(key, classes, words);
  let search = $state('');
  const shown = $derived(sources.filter((each) => matchesSearch(search, [each.key, t(`app.cmdb_admin.source_kind.${each.kind}`)])));

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

  // --- A source --------------------------------------------------------------------------------

  interface SourceDraft {
    key: string;
    kind: CiSourceDefine['kind'];
    rank: string;
    classes: string[];
    mayCreate: boolean;
  }
  let sourceEditing = $state<CiSource | undefined>(undefined);
  let sourceDraft = $state<SourceDraft | undefined>(undefined);
  let isSourceOpen = $state(false);

  function openSource(of?: CiSource) {
    sourceEditing = of;
    sourceDraft = {
      key: of?.key ?? '', kind: of && of.kind !== 'MANUAL' ? of.kind : 'FILE', rank: String(of?.rank ?? 50),
      classes: [...(of?.classes ?? [])], mayCreate: of?.may_create ?? true,
    };
    dialogFailure = undefined;
    isSourceOpen = true;
  }

  const rankOf = (text: string) => Number.parseInt(text, 10);
  const sourceReason = $derived(
    !sourceDraft ? undefined
    : !sourceEditing && (!keyIsWellFormed(sourceDraft.key) || sourceDraft.key === 'manual') ? t('app.cmdb_admin.key_hint')
    : !(rankOf(sourceDraft.rank) >= 1 && rankOf(sourceDraft.rank) <= 1000) ? t('app.cmdb_admin.rank_hint')
    : undefined,
  );

  async function saveSource() {
    const draft = sourceDraft;
    if (!draft) return;
    const body: CiSourceDefine = {
      kind: draft.kind, rank: rankOf(draft.rank), classes: draft.classes, may_create: draft.mayCreate,
      ...(sourceEditing?.mapping ? { mapping: sourceEditing.mapping } : {}),
    };
    const done = await act(() => cmdb.defineSource(sourceEditing?.key ?? draft.key, body, sourceEditing?.version), t('app.cmdb_admin.saved'), true);
    if (done) isSourceOpen = false;
  }

  const sourceKinds = $derived<SelectOption[]>((['FILE', 'API', 'CONNECTOR'] as const).map((each) => ({ value: each, label: t(`app.cmdb_admin.source_kind.${each}`) })));
  const classChecks = $derived(classes.filter((each) => each.key !== 'ci').map((each) => ({ value: each.key, label: nameOf(each.key) })));

  const columns = $derived<DataColumn[]>([
    { id: 'key', label: t('app.cmdb_admin.col_source'), priority: 1 },
    { id: 'kind', label: t('app.cmdb_admin.source_kind_label'), priority: 2 },
    { id: 'rank', label: t('app.cmdb_admin.rank_label'), priority: 2 },
    { id: 'classes', label: t('app.cmdb_admin.classes'), priority: 3 },
    { id: 'creates', label: t('app.cmdb_admin.may_create'), priority: 3 },
  ]);
  const rowMenu = (each: CiSource): DataRowMenu | undefined =>
    mayShape && !each.system ? { label: t('app.page.more_for', { name: each.key }), items: [{ id: 'edit', label: t('app.cmdb_admin.edit_named', { name: each.key }) }] } : undefined;
  const primary = $derived(mayShape ? { label: t('app.cmdb_admin.new_source'), icon: 'plus' as const, onclick: () => openSource() } : undefined);
</script>

{#snippet strip()}<ProductTabs module="ASSETS" current="sources" {onnavigate} />{/snippet}

<ListPage head={{ ...productHead('ASSETS'), primary }} {strip} refused={reachesAdministrationArea() === false ? notYours : undefined}>
  {#snippet intro()}<p class="meta">{t('app.cmdb_admin.sources_hint')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar searchLabel={t('app.cmdb_admin.search_sources')} clearSearchLabel={t('app.page.clear_search')} bind:search
      countLabel={sourcesRead.state.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(sources.length) }) : undefined} />
  {/snippet}
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if sourcesRead.state.status !== 'ready'}
    <Spinner label={t('app.assets.loading')} />
  {:else}
    <DataTable label={t('app.cmdb_admin.sources')} {columns} rows={shown} key={(each: CiSource) => each.key} rowLabel={(each: CiSource) => each.key}
      onopen={mayShape ? (each: CiSource) => { if (!each.system) openSource(each); } : undefined}
      menu={rowMenu} onmenu={(each: CiSource) => openSource(each)}
      actionsLabel={t('app.page.row_actions')} sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')}>
      {#snippet cell(each: CiSource, column: DataColumn)}
        <!-- A source has no name of its own: its key is what names it. -->
        {#if column.id === 'key'}<span data-key-is-name>{each.key}</span>
        {:else if column.id === 'kind'}{t(`app.cmdb_admin.source_kind.${each.kind}`)}
        {:else if column.id === 'rank'}{each.rank}
        {:else if column.id === 'classes'}{each.classes.length > 0 ? each.classes.map(nameOf).join(', ') : t('app.cmdb_admin.every_class')}
        {:else}{each.may_create ? t('app.cmdb_admin.may_create') : t('app.cmdb_admin.updates_only')}{/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#snippet notYours()}<EmptyState kind="filtered" title={t('app.settings.not_yours')} />{/snippet}

{#if isSourceOpen && sourceDraft}
  <Dialog bind:isOpen={isSourceOpen} title={sourceEditing ? t('app.cmdb_admin.edit_named', { name: sourceEditing.key }) : t('app.cmdb_admin.new_source')} dismissLabel={t('app.assets.cancel')} size="m">
    <Stack gap="150">
      {#if !sourceEditing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={sourceDraft.key} maxlength={63} />{/if}
      <Select label={t('app.cmdb_admin.source_kind_label')} options={sourceKinds} bind:value={sourceDraft.kind} />
      <Input label={t('app.cmdb_admin.rank_label')} hint={t('app.cmdb_admin.rank_hint')} inputmode="numeric" bind:value={sourceDraft.rank} />
      <Switch label={t('app.cmdb_admin.may_create')} bind:checked={sourceDraft.mayCreate} />
      <CheckboxGroup label={t('app.cmdb_admin.source_classes')} hint={t('app.cmdb_admin.source_classes_hint')} options={classChecks} selected={sourceDraft.classes} onChange={(next) => { if (sourceDraft) sourceDraft.classes = [...next]; }} />
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isSourceOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={sourceReason} onclick={() => void saveSource()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
