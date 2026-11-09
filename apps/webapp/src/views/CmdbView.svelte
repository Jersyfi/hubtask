<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The assets' settings (M6, page-system §9.6): the relation catalogue with which end suffers when
  // the other fails - an administrator adds and changes the workspace's own, a shipped type is read,
  // never changed - and the software aliases (UC-AST-13 check 3), which say which reported product
  // counts as which; one is added or removed. Each a section at `/assets/settings/<id>`, the assets'
  // last tab under the home's head and `ViewBar` (the navigation concept §4); the classes and the
  // sources are tabs of their own (AssetClassesView, AssetSourcesView).
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE at the workspace the
  // page lists the definitions and offers no control.

  import {
    Button, DataTable, Dialog, EmptyState, IconButton, Input, Select, SettingsLayout, Spinner, Stack, Switch,
    type DataColumn, type DataRowMenu, type SectionLink, type SelectOption,
  } from '@hubtask/design-system/components';
  import TableSection from '../lib/page/TableSection.svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import type {
    CiClass, CiClassList, CiRelationType, CiRelationTypeList, SoftwareAlias, SoftwareAliasList,
  } from '@hubtask/sync-engine';

  import { holdsHere, reachesAdministrationArea } from '../lib/data/capability.svelte.ts';
  import {
    aliasDefine, classesPath, classWord, cmdb, keyIsWellFormed, relationTypesPath, relationWord, softwareAliasesPath, type Words,
  } from '../lib/data/cmdb.svelte.ts';
  import { untrack } from 'svelte';
  import { engine } from '../lib/data/engine.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ProductTabs from '../lib/product/ProductTabs.svelte';
  import { productHead } from '../lib/product/frame.svelte.ts';

  interface Props {
    /** The section the address names (`/assets/settings/<id>`); none is the list of sections. */
    section?: string;
    /** Replaces the address: a move between sections. */
    onreplace?: (path: string) => void;
    onnavigate?: (path: string) => void;
  }
  const { section, onreplace, onnavigate }: Props = $props();
  const BASE = '/assets/settings';
  const title = $derived(t('app.cmdb_admin.settings_title'));
  $effect(() => page.entitle(title));
  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const words: Words = (code) => (messages.has(code) ? t(code) : undefined);

  const classesRead = resource<CiClassList>(classesPath);
  const typesRead = resource<CiRelationTypeList>(relationTypesPath);
  const classes = $derived(classesRead.state.status === 'ready' ? classesRead.state.data.data : []);
  const types = $derived(typesRead.state.status === 'ready' ? typesRead.state.data.data : []);
  const refusal = $derived(classesRead.state.status === 'failed' ? renderProblem(classesRead.state.error, messages).message : undefined);

  const SECTIONS = ['types', 'aliases'];
  const named = $derived(SECTIONS.find((each) => each === section));
  const shown = $derived(named ?? 'types');
  const view = $derived<'list' | 'detail'>(named ? 'detail' : 'list');
  const links = $derived<SectionLink[]>(SECTIONS.map((id) => ({ id, label: t(`app.cmdb_admin.${id}`), href: `${BASE}/${id}` })));
  // The software aliases (UC-AST-13 check 3), read only once their tab is opened.
  let aliases = $state<readonly SoftwareAlias[] | undefined>(undefined);
  $effect(() => {
    if (shown !== 'aliases') return;
    return untrack(() =>
      engine.subscribe<SoftwareAliasList>({ path: softwareAliasesPath }, (state) => {
        if (state.status === 'ready') aliases = state.data.data;
        if (state.status === 'failed') {
          aliases = [];
          toast.show({ text: renderProblem(state.error, messages).message, tone: 'danger' });
        }
      }),
    );
  });
  let isAliasOpen = $state(false);
  let aliasDraft = $state({ matchPublisher: '', matchProduct: '', publisher: '', product: '' });
  function openAlias() {
    aliasDraft = { matchPublisher: '', matchProduct: '', publisher: '', product: '' };
    dialogFailure = undefined;
    isAliasOpen = true;
  }
  async function saveAlias() {
    const saved = await act(() => cmdb.defineAlias(aliasDefine(aliasDraft)), t('app.cmdb_admin.alias_saved'), true);
    if (saved) isAliasOpen = false;
  }

  const nameOf = (key: string) => classWord(key, classes, words);
  /** The classes in reading order, for a relation type's pairs. */
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

  const classOptions = $derived<SelectOption[]>(tree.map((each) => ({ value: each.key, label: nameOf(each.key) })));

  // --- A relation type -------------------------------------------------------------------------

  interface TypeDraft {
    key: string;
    name: string;
    reverse: string;
    impact: CiRelationType['impact'];
    pairs: { from: string; to: string }[];
    archived: boolean;
  }
  let typeEditing = $state<CiRelationType | undefined>(undefined);
  let typeDraft = $state<TypeDraft | undefined>(undefined);
  let isTypeOpen = $state(false);

  function openType(of?: CiRelationType) {
    typeEditing = of;
    typeDraft = {
      key: of?.key ?? '', name: of?.name ?? '', reverse: of?.reverse_name ?? '', impact: of?.impact ?? 'SOURCE',
      pairs: (of?.pairs ?? []).map((pair) => ({ ...pair })), archived: of?.archived ?? false,
    };
    dialogFailure = undefined;
    isTypeOpen = true;
  }

  const typeReason = $derived(
    !typeDraft ? undefined
    : !typeEditing && !keyIsWellFormed(typeDraft.key) ? t('app.cmdb_admin.key_hint')
    : !typeDraft.name.trim() || !typeDraft.reverse.trim() ? t('app.cmdb_admin.needs_both_names')
    : undefined,
  );

  async function saveType() {
    const draft = typeDraft;
    if (!draft) return;
    const body = {
      name: draft.name.trim(), reverse_name: draft.reverse.trim(), impact: draft.impact,
      pairs: draft.pairs.filter((pair) => pair.from && pair.to), archived: draft.archived,
    };
    const done = await act(() => cmdb.defineRelationType(typeEditing?.key ?? draft.key, body, typeEditing?.version), t('app.cmdb_admin.saved'), true);
    if (done) isTypeOpen = false;
  }

  const impactOptions = $derived<SelectOption[]>((['NONE', 'SOURCE', 'TARGET', 'BOTH'] as const).map((each) => ({ value: each, label: t(`app.cmdb_admin.impact.${each}`) })));

  const tableProps = $derived({ actionsLabel: t('app.page.row_actions'), sheetDismissLabel: t('app.page.sheet_close'), sheetGrabLabel: t('app.page.sheet_grab') });
  const typeColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.cmdb_admin.type_name'), priority: 1 },
    { id: 'impact', label: t('app.cmdb_admin.impact_label'), priority: 2 },
    { id: 'pairs', label: t('app.cmdb_admin.pairs'), priority: 3 },
  ]);
  const typeMenu = (each: CiRelationType): DataRowMenu | undefined => mayShape && !each.system
    ? { label: t('app.page.more_for', { name: relationWord(each, each.key, true, words) }), items: [{ id: 'edit', label: t('app.cmdb_admin.edit_named', { name: relationWord(each, each.key, true, words) }) }] }
    : undefined;
  const aliasColumns = $derived<DataColumn[]>([
    { id: 'counts', label: t('app.cmdb_admin.alias_target'), priority: 1 },
    { id: 'matches', label: t('app.cmdb_admin.alias_reported'), priority: 2 },
  ]);
  // The one act on an alias is its removal (UC-AST-13 check 3).
  const aliasMenu = (each: SoftwareAlias): DataRowMenu | undefined => mayShape
    ? { label: t('app.page.more_for', { name: each.match_product }), items: [{ id: 'remove', label: t('app.cmdb_admin.alias_remove', { product: each.match_product }), isDestructive: true }] }
    : undefined;
</script>

{#snippet tabs()}<ProductTabs module="ASSETS" current="settings" {onnavigate} />{/snippet}

<SettingsLayout
  head={productHead('ASSETS')}
  {tabs}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current: shown, onselect: (next) => onreplace?.(`${BASE}/${next}`) }}
  {view}
  back={{ label: t('app.product_home.tab_settings'), onback: () => onreplace?.(BASE) }}
  refused={reachesAdministrationArea() === false ? notYours : undefined}
>
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if classesRead.state.status !== 'ready'}
    <Spinner label={t('app.assets.loading')} />
  {:else}
    <!-- Every change here is a dialog's own save or a row's act, so nothing waits for a save bar. -->
    <TableSection title={t(`app.cmdb_admin.${shown}`)} description={t(`app.cmdb_admin.${shown}_hint`)}>
      {#snippet acts()}
        {#if mayShape}
          {#if shown === 'types'}<Button data-list-add icon="plus" onclick={() => openType()}>{t('app.cmdb_admin.new_type')}</Button>
          {:else}<Button data-list-add icon="plus" data-new-alias onclick={openAlias}>{t('app.cmdb_admin.new_alias')}</Button>{/if}
        {/if}
      {/snippet}
      {#if shown === 'types'}
        <DataTable label={t('app.cmdb_admin.types')} columns={typeColumns} rows={types} key={(each: CiRelationType) => each.key}
          rowLabel={(each: CiRelationType) => relationWord(each, each.key, true, words)}
          onopen={mayShape ? (each: CiRelationType) => { if (!each.system) openType(each); } : undefined}
          menu={typeMenu} onmenu={(each: CiRelationType) => openType(each)} {...tableProps}>
          {#snippet cell(each: CiRelationType, column: DataColumn)}
            {#if column.id === 'name'}{relationWord(each, each.key, true, words)} / {relationWord(each, each.key, false, words)}
            {:else if column.id === 'impact'}{t(`app.cmdb_admin.impact.${each.impact}`)}
            {:else}<span class="meta">{[
              each.system ? t('app.cmdb_admin.shipped') : '',
              each.archived ? t('app.cmdb_admin.archived') : '',
              each.pairs.map((pair) => t('app.cmdb_admin.pair', { from: nameOf(pair.from), to: nameOf(pair.to) })).join(', '),
            ].filter((part) => part !== '').join(' · ')}</span>{/if}
          {/snippet}
        </DataTable>
      {:else if !aliases}
        <Spinner label={t('app.assets.loading')} />
      {:else if aliases.length === 0}
        <p class="meta">{t('app.cmdb_admin.aliases_none')}</p>
      {:else}
        <DataTable label={t('app.cmdb_admin.aliases')} columns={aliasColumns} rows={aliases} key={(each: SoftwareAlias) => each.id}
          rowLabel={(each: SoftwareAlias) => each.match_product} menu={aliasMenu}
          onmenu={(each: SoftwareAlias) => void act(() => cmdb.removeAlias(each), t('app.cmdb_admin.alias_removed'))} {...tableProps}>
          {#snippet cell(each: SoftwareAlias, column: DataColumn)}
            {#if column.id === 'counts'}<span data-alias={each.id}>{t('app.cmdb_admin.alias_counts_as', { product: each.product, publisher: each.publisher || '—' })}</span>
            {:else}<span class="meta">{t('app.cmdb_admin.alias_matches', { product: each.match_product, publisher: each.match_publisher || t('app.cmdb_admin.any_publisher') })}</span>{/if}
          {/snippet}
        </DataTable>
      {/if}
    </TableSection>
  {/if}
</SettingsLayout>

<!-- Offered to whoever reaches the administration, as the door before them was (P-05). -->
{#snippet notYours()}<EmptyState kind="filtered" title={t('app.settings.not_yours')} />{/snippet}

{#if isTypeOpen && typeDraft}
  <Dialog bind:isOpen={isTypeOpen} title={typeEditing ? t('app.cmdb_admin.edit_named', { name: typeEditing.key }) : t('app.cmdb_admin.new_type')} dismissLabel={t('app.assets.cancel')} size="m">
    <Stack gap="150">
      {#if !typeEditing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={typeDraft.key} maxlength={63} />{/if}
      <Input label={t('app.cmdb_admin.type_name')} bind:value={typeDraft.name} maxlength={120} />
      <Input label={t('app.cmdb_admin.type_reverse')} bind:value={typeDraft.reverse} maxlength={120} />
      <Select label={t('app.cmdb_admin.impact_label')} options={impactOptions} bind:value={typeDraft.impact} />
      <fieldset class="group">
        <legend>{t('app.cmdb_admin.pairs')}</legend>
        {#each typeDraft.pairs as pair, index (index)}
          <div class="line">
            <Select label={t('app.cmdb_admin.pair_from')} size="sm" options={classOptions} bind:value={pair.from} />
            <Select label={t('app.cmdb_admin.pair_to')} size="sm" options={classOptions} bind:value={pair.to} />
            <IconButton icon="x" size="sm" tone="subtle" label={t('app.cmdb_admin.remove')} onclick={() => typeDraft?.pairs.splice(index, 1)} />
          </div>
        {/each}
        <div><Button size="sm" tone="subtle" icon="plus" onclick={() => typeDraft?.pairs.push({ from: 'ci', to: 'ci' })}>{t('app.cmdb_admin.add_pair')}</Button></div>
      </fieldset>
      {#if typeEditing}<Switch label={t('app.cmdb_admin.archived')} bind:checked={typeDraft.archived} />{/if}
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isTypeOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={typeReason} onclick={() => void saveType()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isAliasOpen}
  <Dialog bind:isOpen={isAliasOpen} title={t('app.cmdb_admin.new_alias')} dismissLabel={t('app.assets.cancel')} size="m">
    <Stack gap="150">
      <p class="meta">{t('app.cmdb_admin.alias_reported')}</p>
      <Input label={t('app.cmdb_admin.alias_match_product')} bind:value={aliasDraft.matchProduct} maxlength={200} isRequired />
      <Input label={t('app.cmdb_admin.alias_match_publisher')} hint={t('app.cmdb_admin.alias_match_publisher_hint')} bind:value={aliasDraft.matchPublisher} maxlength={200} />
      <p class="meta">{t('app.cmdb_admin.alias_target')}</p>
      <Input label={t('app.cmdb_admin.alias_product')} bind:value={aliasDraft.product} maxlength={200} isRequired />
      <Input label={t('app.cmdb_admin.alias_publisher')} bind:value={aliasDraft.publisher} maxlength={200} />
      {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isAliasOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button
        tone="primary"
        disabledReason={!aliasDraft.matchProduct.trim() ? t('cmdb.alias_match_invalid') : !aliasDraft.product.trim() ? t('cmdb.alias_target_invalid') : undefined}
        onclick={() => void saveAlias()}
      >{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .group { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .line { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
