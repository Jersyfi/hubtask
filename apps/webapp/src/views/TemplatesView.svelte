<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's *Templates* (page-system §7.1, §9.1): one list page, its strip *Catalogue · In
  // use · Presets*, each its own address. The catalogue is a grid of templates on the canvas, its
  // families a chip; a template opens its Dialog L - the plan first (configuration.md §6). The
  // presets are the workspace's saved structures (UC-WRK-30 check 3): renamed and withdrawn here,
  // saved from a hub by the head's primary. The default preset and the locks are the workspace
  // screen's *New hubs*.
  //
  // **What was made from a preset keeps its settings.** A preset is copied, never followed, so
  // renaming or withdrawing one changes no hub or collection; the presets' sentence says so once.
  //
  // **The default is not withdrawn from under the workspace.** Its *Withdraw…* stays in the menu,
  // unavailable, with the reason (design-system.md §4, case 2) - the server refuses it too.

  import { untrack } from 'svelte';

  import {
    Button,
    DataTable,
    EmptyState,
    FilterBar,
    ListPage,
    Radio,
    ResourceCard,
    ResourceGrid,
    Spinner,
    Tabs,
    Badge,
    LoadMore,
    type DataColumn,
    type DataRowMenu,
    type Filter,
    type MenuItem,
  } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplateList, Preset, ResourceState } from '@hubtask/sync-engine';

  import { config } from '../lib/data/config.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { offerOf } from '../lib/data/configtemplates.ts';
  import { presets } from '../lib/data/presets.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import TemplateDialog from '../lib/settings/TemplateDialog.svelte';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { pagePart } from '../lib/settings/asFile.ts';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import RenamePresetDialog from '../lib/templates/RenamePresetDialog.svelte';
  import SaveHubDialog from '../lib/templates/SaveHubDialog.svelte';
  import WithdrawPresetDialog from '../lib/templates/WithdrawPresetDialog.svelte';
  import { TEMPLATE_TABS, TEMPLATES_PATH, figures, narrowed, tabOf, tabPath, type TemplateTab } from '../lib/templates/templatespage.ts';
  import { familiesOf, familyOf, markOf, switchedOn, type Family } from '../lib/workspace/hubGallery.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  interface Props {
    /** The tab the address names (`?tab=`); the catalogue where it names none or an unknown one. */
    tab?: string;
    /** A template to open at once, as a product's first visit chose it (`?template=`). */
    template?: string;
    /** Opens another screen: the product a taken template is for. */
    onnavigate?: (path: string) => void;
    /** Replaces the address when the tab changes, so a reload or a link keeps it. */
    onreplace?: (path: string) => void;
  }

  const { tab, template, onnavigate, onreplace }: Props = $props();

  const current = $derived(tabOf(tab));

  $effect(() => untrack(() => workspace.open()));
  $effect(() => untrack(() => presets.open()));
  $effect(() => page.entitle(t('app.presets.title')));

  let list = $state<ResourceState<ConfigTemplateList>>();
  $effect(() => untrack(() => config.templates((next) => (list = next))));

  const templates = $derived(list?.status === 'ready' ? list.data.data : []);
  const refusal = $derived(list?.status === 'failed' ? renderProblem(list.error, messages) : undefined);
  const counted = $derived(figures(templates));
  const structure = $derived(workspace.workspace?.structure);
  const mayChange = $derived(holdsHere('STRUCTURE') === true);

  /** Hubtask's templates are named by their key, in the reader's language. */
  const nameOf = (each: ConfigTemplate) =>
    each.origin === 'HUBTASK' ? t(`app.structure.preset.${each.key.toUpperCase()}`) : each.name;
  /** What a card says it does: Hubtask's own sentence; nothing for another's. */
  const summaryOf = (each: ConfigTemplate) =>
    each.origin === 'HUBTASK' && messages.has(`app.templates.summary.${each.key.toUpperCase()}`) ? t(`app.templates.summary.${each.key.toUpperCase()}`) : undefined;
  const productsOf = (each: ConfigTemplate): string[] =>
    switchedOn(each).map((module) => t(`app.modules.name_${module.toLowerCase()}`));
  const familyWord = (family: Family) => t(`app.workspace.hub_family.${family}`);

  // The narrowing: the search over names and sentences, the family chip (user-experience.md §5.2).
  let search = $state('');
  let familyChoice = $state('');
  const family = $derived((familyChoice || undefined) as Family | undefined);
  const families = $derived(familiesOf(templates));
  // The catalogue in its families' order, as the gallery grouped it.
  const shown = $derived(
    narrowed(templates, { tab: current, family, search, words: (each) => [nameOf(each), summaryOf(each), ...productsOf(each)] })
      .map((each, index) => ({ each, index }))
      .sort((a, b) => families.indexOf(familyOf(a.each)) - families.indexOf(familyOf(b.each)) || a.index - b.index)
      .map(({ each }) => each),
  );
  // A catalogue that arrived whole continues at its foot (PS-8); narrowing starts it again.
  const continued = new Continued(() => shown);
  $effect(() => {
    void [current, search, familyChoice];
    untrack(() => continued.restart());
  });
  const inTab = $derived(current === 'in-use' ? counted.inUse : counted.catalogue);
  const chips = $derived<Filter[]>(
    families.length > 1
      ? [{ id: 'family', label: t('app.templates.family_chip', { family: family ? familyWord(family) : t('app.templates.family_all') }), isActive: family !== undefined }]
      : [],
  );
  function clearFilters(): void {
    search = '';
    familyChoice = '';
  }

  /** The template opened in its dialog. */
  let opened = $state<ConfigTemplate>();
  // `?template=` opens its template once the list holds it; unknown, the page is drawn as always.
  let preselected: string | undefined;
  $effect(() => {
    const key = template;
    if (!key || preselected === key || list?.status !== 'ready') return;
    const found = list.data.data.find((each) => each.key === key);
    preselected = key;
    if (found) untrack(() => (opened = found));
  });

  function stateOf(each: ConfigTemplate): { label: string; tone: 'success' | 'info' } | undefined {
    const offer = offerOf(each);
    if (offer === 'UPDATE') return { label: t('app.templates.state_update'), tone: 'info' };
    if (offer === 'IN_USE') return { label: t('app.templates.state_in_use'), tone: 'success' };
    return undefined;
  }

  // The presets: the workspace's own, searched by name.
  const savedPresets = $derived(presets.workspace);
  const shownPresets = $derived(savedPresets.filter((preset) => matchesSearch(search, [preset.name ?? undefined])));
  const isDefault = (preset: Preset): boolean => preset.id === structure?.default_preset_id;
  const presetColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.presets.name'), isSortable: true, priority: 1 },
    { id: 'state', label: t('app.presets.default_badge'), priority: 2 },
  ]);
  const presetOrder = sorting({ rows: () => shownPresets, read: (preset: Preset) => preset.name ?? '' });

  let renaming = $state<Preset>();
  let withdrawing = $state<Preset>();
  function presetMenu(preset: Preset): DataRowMenu | undefined {
    if (!mayChange) return undefined;
    const items: MenuItem[] = [
      { id: 'rename', label: t('app.presets.rename_menu') },
      {
        id: 'withdraw',
        label: t('app.presets.withdraw_menu'),
        isDestructive: true,
        hasSeparatorBefore: true,
        disabledReason: isDefault(preset) ? t('app.presets.withdraw_default') : undefined,
      },
    ];
    return { label: t('app.page.more_for', { name: preset.name ?? '' }), items };
  }
  function onPresetMenu(preset: Preset, id: string): void {
    if (id === 'rename') renaming = preset;
    else if (id === 'withdraw' && !isDefault(preset)) withdrawing = preset;
  }

  let isSavingHub = $state(false);
  let isFileOpen = $state(false);

  const figure = $derived(
    list?.status !== 'ready'
      ? undefined
      : counted.updates > 0
        ? t('app.templates.figure_updates', { catalogue: counted.catalogue, in_use: counted.inUse, updates: counted.updates })
        : t('app.templates.figure', { catalogue: counted.catalogue, in_use: counted.inUse }),
  );
  const fileAction = $derived(viewAsFileAction(() => (isFileOpen = true)));
  const head = $derived({
    title: t('app.presets.title'),
    subtitle: figure,
    breadcrumb: {
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'templates', label: t('app.presets.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
    primary: mayChange ? { label: t('app.templates.save_from_hub'), icon: 'plus' as const, opener: 'save-hub', onclick: () => (isSavingHub = true) } : undefined,
    secondary: fileAction ? ([fileAction] as const) : ([] as const),
  });

  const tabCounts = $derived<Record<TemplateTab, number | undefined>>({
    catalogue: list?.status === 'ready' ? counted.catalogue : undefined,
    'in-use': list?.status === 'ready' ? counted.inUse : undefined,
    presets: presets.all.length > 0 || structure ? savedPresets.length : undefined,
  });
</script>

<ListPage {head}>
  {#snippet strip()}
    <Tabs
      label={t('app.presets.title')}
      selected={current}
      onselect={(next) => {
        search = '';
        familyChoice = '';
        onreplace?.(tabPath(next as TemplateTab));
      }}
      tabs={TEMPLATE_TABS.map((each) => ({ id: each.id, label: t(each.code), count: tabCounts[each.id] }))}
    />
  {/snippet}
  <!-- The catalogue needs no sentence above its grid (the mockup's scene); the presets say once
       that what was made from one keeps its settings. -->
  {#snippet intro()}
    {#if current === 'presets'}<p class="quiet">{t('app.presets.saved_about')}</p>{/if}
  {/snippet}
  {#snippet filters()}
    {#if current === 'presets'}
      <FilterBar
        searchLabel={t('app.presets.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        countLabel={t('app.page.found', { shown: String(shownPresets.length), count: String(savedPresets.length) })}
      />
    {:else}
      <FilterBar
        searchLabel={t('app.templates.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        filters={chips}
        moreLabel={t('app.page.more_filters')}
        filtersLabel={t('app.page.filters', { count: String(family ? 1 : 0) })}
        sheetTitle={t('app.page.filters', { count: '0' })}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        showLabel={t('app.page.show_results', { count: String(shown.length) })}
        clearLabel={t('app.page.clear')}
        onclear={clearFilters}
        countLabel={list?.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(inTab) }) : undefined}
      >
        {#snippet picker(filter: Filter)}
          {#if filter.id === 'family'}
          <Radio
            label={t('app.templates.families')}
            bind:value={familyChoice}
            options={[{ value: '', label: t('app.templates.family_all') }, ...families.map((each) => ({ value: each, label: familyWord(each) }))]}
          />
          {/if}
        {/snippet}
      </FilterBar>
    {/if}
  {/snippet}

  <!-- Says once, to a reader who may not change the workspace's shape, why nothing here changes. -->
  <WorkspaceGate permission="STRUCTURE" />

  {#if current === 'presets'}
    <div class="region" data-presets-saved>
      <DataTable
        label={t('app.templates.tab_presets')}
        columns={presetColumns}
        rows={presetOrder.rows}
        key={(preset: Preset) => preset.id ?? ''}
        rowLabel={(preset: Preset) => preset.name ?? ''}
        onopen={mayChange ? (preset: Preset) => (renaming = preset) : undefined}
        menu={presetMenu}
        onmenu={onPresetMenu}
        actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        sort={presetOrder.sort}
        onSort={(next) => presetOrder.by(next)}
        shownLabel={savedPresets.length === 0 ? undefined : t('app.page.showing', { count: String(shownPresets.length) })}
      >
        {#snippet cell(preset: Preset, column: DataColumn)}
          {#if column.id === 'name'}
            <span class="name" data-preset={preset.id}>{preset.name}</span>
          {:else if isDefault(preset)}
            <Badge tone="info" size="sm">{t('app.presets.default_badge')}</Badge>
          {/if}
        {/snippet}
        {#snippet empty()}
          {#if savedPresets.length === 0}
            <div data-presets-empty><EmptyState kind="unused" title={t('app.presets.none')} /></div>
          {:else}
            <EmptyState kind="filtered" title={t('app.presets.filtered_title')}>
              {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
            </EmptyState>
          {/if}
        {/snippet}
      </DataTable>
    </div>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else if list?.status !== 'ready'}
    <p class="quiet"><Spinner label={t('app.templates.reading')} /> <span>{t('app.templates.reading')}</span></p>
  {:else}
    <ResourceGrid label={t(current === 'in-use' ? 'app.templates.tab_in_use' : 'app.templates.tab_catalogue')} isEmpty={shown.length === 0}>
      {#each continued.shown as each (each.key)}
        <ResourceCard
          title={nameOf(each)}
          description={summaryOf(each)}
          mark={markOf(each)}
          chips={productsOf(each)}
          state={stateOf(each)}
          onopen={() => (opened = each)}
        />
      {/each}
      {#snippet empty()}
        {#if inTab === 0}
          <EmptyState kind="unused" title={t(current === 'in-use' ? 'app.templates.none_in_use' : 'app.templates.none')} />
        {:else}
          <EmptyState kind="filtered" title={t('app.templates.filtered_title')}>
            {#snippet action()}<Button onclick={clearFilters}>{t('app.page.clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </ResourceGrid>
    {#if shown.length > 0}
      <div class="foot">
        <span class="quiet">{continued.shownLabel}</span>
        {#if continued.more}<LoadMore {...continued.more} />{/if}
      </div>
    {/if}
  {/if}
</ListPage>

<TemplateDialog template={opened} isPreviewOnly={!mayChange} {onnavigate} onClose={() => (opened = undefined)} />
{#if mayChange}
  <SaveHubDialog bind:isOpen={isSavingHub} onsaved={() => onreplace?.(`${TEMPLATES_PATH}?tab=presets`)} />
  <RenamePresetDialog preset={renaming} onClose={() => (renaming = undefined)} />
  <WithdrawPresetDialog preset={withdrawing} onClose={() => (withdrawing = undefined)} />
{/if}
<ViewSectionsAsFile {...pagePart('/administration/templates')} bind:isOpen={isFileOpen} isTriggerHidden />

<style>
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; display: flex; gap: var(--sp-100); align-items: center; }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .region { display: contents; }
  .foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-150); }
</style>
