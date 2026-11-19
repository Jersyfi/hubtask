<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The plan's panel *Ansicht* (work-surface concept §6.3, E-20, scene planview): presentation,
  // what is seen, rows, bars, the baseline compared against and the outline columns, saved as a
  // view as in every collection - the collections' pane: beside the plan from `expanded`, a drawer
  // below it, a sheet on a phone. Tolerance and baselines stay under ⋯ › plan settings.
  // It holds no plan of its own: `setup` is the caller's.
  import { Button, Checkbox, Drawer, Input, QueryLine, Select, Sheet, Switch, lineText, type QueryToken } from '@hubtask/design-system/components';
  import { createCatalogue, type QueryKey } from '@hubtask/query';

  import { t } from '../i18n/i18n.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { language, wording } from '../entries/viewLanguage.ts';
  import ViewSide from '../entries/ViewSide.svelte';
  import {
    PLAN_BARS, PLAN_COLOURS, PLAN_COLUMNS, PLAN_GROUPINGS, PLAN_SORTS, planViewDefault, type PlanViewSetup,
  } from '../data/planview.ts';

  type Option = { readonly value: string; readonly label: string };
  interface Props {
    isOpen?: boolean;
    setup: PlanViewSetup;
    base: PlanViewSetup;
    baseName: string;
    changed: number;
    keys: readonly QueryKey[];
    baselines: readonly Option[];
    /** The plan's saved views, to open one; the first is the template. */
    views: readonly Option[];
    viewId: string;
    onview: (id: string) => void;
    scopes: readonly Option[];
    mayShare: boolean;
    overwritable?: string;
    isSaving?: boolean;
    failure?: string;
    onsave: (name: string, scope: string, sharing: 'PRIVATE' | 'SCOPE') => void;
    onoverwrite?: () => void;
    /** Whether the Gantt can be chosen: only where the planning module is on (E-16). */
    hasGantt?: boolean;
  }
  let {
    isOpen = $bindable(false), setup = $bindable(), base, baseName, changed, keys, baselines, views, viewId, onview,
    scopes, mayShare, overwritable, isSaving = false, failure, onsave, onoverwrite, hasGantt = true,
  }: Props = $props();

  const catalogue = $derived(createCatalogue(keys, wording, undefined));
  let tokens = $state<readonly QueryToken[]>([]);
  let text = $state('');
  let spelled = '';
  $effect(() => {
    const wanted = setup.line;
    if (wanted === spelled) return;
    tokens = language.parse(`${wanted} `).tokens;
    text = '';
    spelled = wanted;
  });
  function lineChanged(next: readonly QueryToken[]) {
    spelled = lineText(language, next, '');
    setup = { ...setup, line: spelled };
  }

  let name = $state('');
  let scope = $state('');
  let sharing = $state<'PRIVATE' | 'SCOPE'>('SCOPE');
  let isNameMissing = $state(false);
  $effect(() => {
    if (!scopes.some((each) => each.value === scope)) scope = scopes[0]?.value ?? 'ACCOUNT';
  });
  $effect(() => {
    if (scope === 'ACCOUNT' || !mayShare) sharing = 'PRIVATE';
  });
  function save() {
    if (name.trim() === '') {
      isNameMissing = true;
      return;
    }
    isNameMissing = false;
    onsave(name.trim(), scope, sharing);
  }
  function toggleColumn(id: string, isOn: boolean) {
    const next = isOn ? [...setup.columns, id] : setup.columns.filter((each) => each !== id);
    setup = { ...setup, columns: PLAN_COLUMNS.filter((each) => next.includes(each)) };
  }
  const reset = () => (setup = { ...base });
  const isGantt = $derived(setup.presentation === 'gantt');
</script>

{#snippet resetAct()}
  <Button size="sm" tone="subtle" data-plan-view-reset onclick={() => (setup = { ...planViewDefault(), line: '' })}>{t('app.viewcfg.as_template')}</Button>
{/snippet}

{#snippet chips(label: string, options: readonly Option[], value: string, onpick: (value: string) => void, mark: string)}
  <div class="chips" role="group" aria-label={label}>
    {#each options as each (each.value)}
      <button type="button" class="chip" aria-pressed={value === each.value} data-plan-chip={`${mark}:${each.value}`} onclick={() => onpick(each.value)}>{each.label}</button>
    {/each}
  </div>
{/snippet}

{#snippet body()}
  <div class="panel" data-plan-view-panel>
    {#if views.length > 1}
      <Select label={t('app.planview.saved')} size="sm" value={viewId} options={views} onchange={(event: Event) => onview((event.currentTarget as HTMLSelectElement).value)} />
    {/if}
    {#if hasGantt}
    <section aria-labelledby="planview-presentation">
      <h3 id="planview-presentation">{t('app.planning.presentation')}</h3>
      <div class="segments" role="group" aria-label={t('app.planning.presentation')}>
        {#each ['timeline', 'gantt'] as const as each (each)}
          <button type="button" aria-pressed={setup.presentation === each} data-plan-presentation-panel={each} onclick={() => (setup = { ...setup, presentation: each })}>{t(`app.planning.presentation_${each}`)}</button>
        {/each}
      </div>
      <p class="quiet">{t('app.planview.presentation_hint')}</p>
    </section>
    {/if}

    <section aria-labelledby="planview-query">
      <div class="heading"><h3 id="planview-query">{t('app.viewcfg.query')}</h3><span class="quiet">{t('app.viewcfg.query_language')}</span></div>
      <QueryLine
        label={t('app.viewcfg.query')}
        {language}
        {catalogue}
        bind:tokens
        bind:text
        placeholder={t('app.viewcfg.query_placeholder')}
        suggestionsLabel={t('app.viewcfg.suggestions')}
        fieldsHeading={(typed) => t('app.viewcfg.fields_heading', { typed })}
        valuesHeading={(field) => t('app.viewcfg.values_heading', { field })}
        editLabel={(spelling) => t('app.viewcfg.edit_chip', { spelling })}
        removeLabel={(spelling) => t('app.viewcfg.remove_chip', { spelling })}
        onchange={lineChanged}
        addConditionLabel={t('app.viewcfg.add_condition')}
        addGroupLabel={t('app.viewcfg.add_group')}
      />
      {@render chips(t('app.viewcfg.done'), [{ value: 'show', label: t('app.planview.done_show') }, { value: 'hide', label: t('app.planview.done_hide') }], setup.done, (value) => (setup = { ...setup, done: value as 'show' | 'hide' }), 'done')}
    </section>

    <section aria-labelledby="planview-rows">
      <h3 id="planview-rows">{t('app.planview.rows')}</h3>
      <p class="label">{t('app.viewcfg.group_by')}</p>
      {@render chips(t('app.viewcfg.group_by'), PLAN_GROUPINGS.map((value) => ({ value, label: t(`app.planview.group_${value}`) })), setup.groupBy, (value) => (setup = { ...setup, groupBy: value as PlanViewSetup['groupBy'] }), 'group')}
      <p class="label">{t('app.viewcfg.sort_by')}</p>
      {@render chips(t('app.viewcfg.sort_by'), PLAN_SORTS.map((value) => ({ value, label: t(`app.planview.sort_${value}`) })), setup.sortBy, (value) => (setup = { ...setup, sortBy: value as PlanViewSetup['sortBy'] }), 'sort')}
    </section>

    <section aria-labelledby="planview-bars">
      <h3 id="planview-bars">{t('app.planview.bars')}</h3>
      <p class="label">{t('app.planview.colour_by')}</p>
      {@render chips(t('app.planview.colour_by'), PLAN_COLOURS.map((value) => ({ value, label: t(`app.planview.colour_${value}`) })), setup.colourBy, (value) => (setup = { ...setup, colourBy: value as PlanViewSetup['colourBy'] }), 'colour')}
      {#each PLAN_BARS as bar (bar)}
        <div data-plan-bar={bar}>
          <Switch label={t(`app.planview.bar_${bar}`)} checked={setup.bars[bar]} onchange={(event: Event) => (setup = { ...setup, bars: { ...setup.bars, [bar]: (event.currentTarget as HTMLInputElement).checked } })} />
        </div>
      {/each}
    </section>

    {#if hasGantt}
    <section aria-labelledby="planview-compare">
      <h3 id="planview-compare">{t('app.planview.compare')}</h3>
      <Select label={t('app.planview.compare')} isLabelHidden size="sm" value={setup.baselineId} options={[{ value: '', label: t('app.planning.baseline_none') }, ...baselines]}
        onchange={(event: Event) => (setup = { ...setup, baselineId: (event.currentTarget as HTMLSelectElement).value })} />
      <p class="quiet">{t('app.planview.compare_hint')}</p>
    </section>

    <section aria-labelledby="planview-columns">
      <h3 id="planview-columns">{t('app.planview.columns')}</h3>
      {#if !isGantt}<p class="quiet">{t('app.planview.columns_gantt')}</p>{/if}
      <ul class="fields">
        {#each PLAN_COLUMNS as column (column)}
          <li>
            <Checkbox label={t(`app.planview.col_${column}`)} checked={setup.columns.includes(column)} onchange={(event: Event) => toggleColumn(column, (event.currentTarget as HTMLInputElement).checked)} />
            {#if column === 'slip'}<span class="quiet">{t('app.planview.with_baseline')}</span>{/if}
          </li>
        {/each}
      </ul>
    </section>

    {/if}

    <section class="save" data-changed={changed > 0 ? '' : undefined} aria-labelledby="planview-save">
      <div class="heading">
        <h3 id="planview-save">{#if changed > 0}<span class="dot" aria-hidden="true"></span>{t('app.viewcfg.changed_against', { name: baseName })}{:else}{t('app.viewcfg.page_save')}{/if}</h3>
        {#if changed > 0}<Button size="sm" tone="subtle" onclick={reset}>{t('app.viewcfg.discard')}</Button>{/if}
      </div>
      <Input label={t('app.views.name')} bind:value={name} maxlength={200} error={isNameMissing ? t('views.name_empty') : undefined} />
      {#if scopes.length > 1}
        <p class="label">{t('app.viewcfg.lives')}</p>
        {@render chips(t('app.viewcfg.lives'), scopes, scope, (value) => (scope = value), 'scope')}
      {/if}
      {#if mayShare && scope !== 'ACCOUNT'}
        <p class="label">{t('app.viewcfg.who_sees')}</p>
        {@render chips(t('app.viewcfg.who_sees'), [{ value: 'PRIVATE', label: t('app.views.sharing_PRIVATE') }, { value: 'SCOPE', label: t('app.views.sharing_SCOPE') }], sharing, (value) => (sharing = value as 'PRIVATE' | 'SCOPE'), 'sharing')}
      {/if}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      <div class="acts">
        <Button size="sm" isBusy={isSaving} data-plan-view-save onclick={save}>{t('app.viewcfg.save_new')}</Button>
        {#if overwritable && onoverwrite}<Button size="sm" tone="secondary" onclick={onoverwrite}>{t('app.viewcfg.overwrite', { name: overwritable })}</Button>{/if}
      </div>
    </section>
  </div>
{/snippet}

{#if viewport.isCompact}
  <Sheet bind:isOpen title={t('app.planview.title', { name: baseName })} closeLabel={t('app.dismiss')} actions={resetAct}>{@render body()}</Sheet>
{:else if !viewport.isBelowExpanded}
  <ViewSide bind:isOpen title={t('app.planview.title', { name: baseName })} dismissLabel={t('app.dismiss')} actions={resetAct}>{@render body()}</ViewSide>
{:else}
  <Drawer bind:isOpen edge="inline-end" title={t('app.planview.title', { name: baseName })} dismissLabel={t('app.dismiss')} actions={resetAct}>{@render body()}</Drawer>
{/if}

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-300); }
  section { display: flex; flex-direction: column; gap: var(--sp-150); padding-block-end: var(--sp-300); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  section:last-child { border-block-end: none; }
  h3 { margin: 0; font-weight: var(--fw-semibold); font-size: var(--fs-065); text-transform: uppercase; color: var(--text-secondary); display: flex; align-items: center; gap: var(--sp-100); }
  .heading { display: flex; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  .quiet, .label { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
  .label { color: var(--text-primary); }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .chip { min-block-size: var(--layout-target); padding-inline: var(--sp-150); font-size: var(--fs-075); color: var(--text-primary); background: var(--bg-surface); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); cursor: pointer; }
  .chip[aria-pressed='true'] { color: var(--text-brand); border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  /* One segmented look (T-2): the density switch's - a white chip on a sunken track. */
  .segments { display: inline-flex; align-self: flex-start; padding: var(--sp-025); border-radius: var(--r-md); overflow: hidden; background: var(--bg-recess); }
  .segments button { min-block-size: var(--layout-target); padding-inline: var(--sp-200); font-size: var(--fs-075); color: var(--text-secondary); background: transparent; border: none; border-radius: var(--r-sm); cursor: pointer; }
  .segments button[aria-pressed='true'] { color: var(--text-primary); background: var(--bg-surface); font-weight: var(--fw-semibold); }
  .fields { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--sp-050); }
  .fields li { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .save[data-changed] { background: var(--accent-primary-subtle); border-radius: var(--r-md); padding: var(--sp-200); border: none; }
  .dot { inline-size: var(--sp-100); block-size: var(--sp-100); border-radius: var(--r-full); background: var(--accent-primary); }
  .acts { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
