<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one panel *Ansicht* (work-surface concept §6.2, E-21, scene viewcfg): layout, what is
  // seen, order, fields, columns and lanes, limits and saving, the same in every collection. It
  // replaces the four menus filter, sort, fields and views. What is seen is the Hubtask query in
  // one line (§6.11): chips and text, suggestions with counts, the count while typing, an error at
  // its chip with its fix, and the same query as text. On a phone it is a sheet whose sections are
  // pages of their own; between phone and desktop a drawer. From `expanded` it is a side pane beside
  // the list, not over it: no scrim, no focus trap, the list narrowed and usable while each change
  // shows at once. `Escape` reaches it through the register as the weakest layer and gives focus
  // back to where the pane was opened from.
  //
  // It holds no query of its own: `setup` is the caller's, and the caller turns it into the read.
  import { Button, Drawer, Icon, Input, MultiSelect, QueryLine, Select, Sheet, Switch, lineText, type QueryToken } from '@hubtask/design-system/components';
  import { createCatalogue, type CountedValue, type QueryKey } from '@hubtask/query';
  import type { IconName } from '@hubtask/design-system/icons';

  import { chooseSeveral } from '../forms/chooseSeveral.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import type { ViewSetup } from '../data/viewquery.ts';
  import { language, wording } from './viewLanguage.ts';
  import ViewSide from './ViewSide.svelte';

  type Option = { readonly value: string; readonly label: string; readonly isNew?: boolean };

  interface Props {
    isOpen?: boolean;
    setup: ViewSetup;
    /** The tab's view, what *Wie die Vorlage* returns to and the changes are counted against. */
    base: ViewSetup;
    baseName: string;
    changed: number;
    layouts: readonly string[];
    groupings: readonly Option[];
    sortFields: readonly Option[];
    fields: readonly Option[];
    boardFields: readonly Option[];
    dateFields: readonly Option[];
    keys: readonly QueryKey[];
    /** How many entries the line keeps, counted by the server for the reader (Q-4, Q-5). */
    total?: number;
    /** The values the data holds for a key, with their counts (Q-3). */
    values?: (key: string, prefix: string) => readonly CountedValue[];
    /** The key whose values are being typed, so the caller can count them. */
    ontyping?: (key: string | undefined) => void;
    /** The pieces searched as words, and the names nobody has. */
    words?: readonly string[];
    unknown?: readonly string[];
    scopes: readonly Option[];
    mayShare: boolean;
    /** The saved view the tab is, where the reader may change it: *„X" überschreiben*. */
    overwritable?: string;
    isSaving?: boolean;
    failure?: string;
    onsave: (name: string, scope: string, sharing: 'PRIVATE' | 'SCOPE') => void;
    onoverwrite?: () => void;
    onexport: () => void;
    onfeeds: () => void;
    onmanage?: () => void;
  }

  let {
    isOpen = $bindable(false),
    setup = $bindable(),
    base,
    baseName,
    changed,
    layouts,
    groupings,
    sortFields,
    fields,
    boardFields,
    dateFields,
    keys,
    total,
    values,
    ontyping,
    words = [],
    unknown = [],
    scopes,
    mayShare,
    overwritable,
    isSaving = false,
    failure,
    onsave,
    onoverwrite,
    onexport,
    onfeeds,
    onmanage,
  }: Props = $props();

  const LAYOUT_ICONS: Readonly<Record<string, IconName>> = {
    LIST_COLLAPSED: 'list', LIST_EXPANDED: 'chevrons-up-down', KANBAN: 'columns-3', TIMELINE: 'clock', CALENDAR: 'calendar',
  };
  const catalogue = $derived(createCatalogue(keys, wording, values));

  let tokens = $state<readonly QueryToken[]>([]);
  let text = $state('');
  /** The line as the tokens spell it, so a line set from outside (a view opened) is read again. */
  let spelled = '';
  $effect(() => {
    const wanted = setup.line;
    if (wanted === spelled) return;
    const read = language.parse(`${wanted} `);
    tokens = read.tokens;
    text = '';
    spelled = wanted;
  });
  function lineChanged(next: readonly QueryToken[]) {
    spelled = lineText(language, next, '');
    setup = { ...setup, line: spelled };
  }
  $effect(() => ontyping?.(/^-?([\w.]+):/u.exec(text.trimStart())?.[1]));

  let name = $state('');
  let scope = $state('COLLECTION');
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

  /** A field shown goes to its place among the offered ones; one hidden leaves the rest as they were. */
  function chooseFields(next: readonly string[]) {
    const isAdded = next.length > setup.fields.length;
    setup = { ...setup, fields: isAdded ? fields.map((field) => field.value).filter((field) => next.includes(field)) : setup.fields.filter((field) => next.includes(field)) };
  }

  const isList = $derived(setup.layout.startsWith('LIST_'));
  const isBoard = $derived(setup.layout === 'KANBAN');
  const conditions = $derived(tokens.length);
  let copied = $state(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(setup.line);
      copied = true;
    } catch {
      copied = false;
    }
  }

  /** The phone's sheet: its sections are pages of their own (scene viewcfg 4). */
  let phonePage = $state<string | undefined>(undefined);
  const isPhone = $derived(viewport.isCompact);
  const pages = $derived([
    { id: 'layout', icon: 'list' as IconName, label: t('app.viewcfg.page_layout', { layout: t(`app.view.${setup.layout}`) }) },
    { id: 'query', icon: 'funnel' as IconName, label: t('app.viewcfg.page_query', { count: String(conditions) }) },
    { id: 'order', icon: 'chevrons-up-down' as IconName, label: t('app.viewcfg.page_order') },
    ...(isList ? [{ id: 'fields', icon: 'columns-3' as IconName, label: t('app.viewcfg.page_fields', { count: String(setup.fields.length) }) }] : []),
    ...(isBoard || setup.layout === 'CALENDAR' ? [{ id: 'board', icon: 'columns-3' as IconName, label: isBoard ? t('app.viewcfg.board') : t('app.viewcfg.calendar') }] : []),
    { id: 'save', icon: 'star' as IconName, label: t('app.viewcfg.page_save') },
  ]);
  const shows = (page: string) => !isPhone || phonePage === page;
  $effect(() => {
    if (!isOpen) phonePage = undefined;
  });

  const isSide = $derived(!viewport.isBelowExpanded);
</script>

{#snippet body()}
  <div class="panel" data-view-panel>
    {#if isPhone && phonePage}
      <Button size="sm" tone="subtle" icon="chevron-left" onclick={() => (phonePage = undefined)}>{t('app.viewcfg.back')}</Button>
    {:else if isPhone}
      <ul class="pages">
        {#each pages as page (page.id)}
          <li><button type="button" data-view-page={page.id} onclick={() => (phonePage = page.id)}><Icon name={page.icon} size="sm" /><span>{page.label}</span><Icon name="chevron-right" size="sm" /></button></li>
        {/each}
      </ul>
      <p class="quiet">{t('app.viewcfg.phone_hint')}</p>
    {/if}

    {#if shows('layout')}
      <section aria-labelledby="viewcfg-layout">
        <h3 id="viewcfg-layout">{t('app.viewcfg.layout')}</h3>
        <div class="layouts">
          {#each layouts as each (each)}
            <button type="button" class="layout" aria-pressed={setup.layout === each || (each === 'LIST_COLLAPSED' && setup.layout === 'LIST_COLLAPSED')} data-layout={each} onclick={() => (setup = { ...setup, layout: each })}>
              <Icon name={LAYOUT_ICONS[each] ?? 'list'} size="sm" />
              <b>{t(`app.view.${each}`)}</b>
              <span>{t(`app.viewcfg.layout_${each}`)}</span>
              {#if each === 'CALENDAR'}<span class="new">{t('app.viewcfg.new')}</span>{/if}
            </button>
          {/each}
        </div>
      </section>
    {/if}

    {#if shows('query')}
      <section aria-labelledby="viewcfg-query">
        <div class="heading"><h3 id="viewcfg-query">{t('app.viewcfg.query')}</h3><span class="quiet">{t('app.viewcfg.query_language')}</span></div>
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
          hint={t('app.viewcfg.chip_hint')}
        >
          {#snippet count()}{#if typeof total === 'number'}<span data-view-count>{t('app.viewcfg.count', { count: String(total) })}</span>{/if}{/snippet}
        </QueryLine>
        {#if words.length > 0}<p class="quiet" data-as-words>{t('app.viewcfg.words_hint', { words: words.join(', ') })}</p>{/if}
        {#if unknown.length > 0}<p class="quiet" data-unknown-names>{t('app.viewcfg.unknown_names', { names: unknown.join(', ') })}</p>{/if}
        {#if setup.line.trim() !== ''}
          <p class="as-line"><span>{t('app.viewcfg.as_text')}</span><code data-view-line>{setup.line}</code>
            <Button size="sm" tone="subtle" icon={copied ? 'check' : 'copy'} onclick={() => void copy()}>{t('app.viewcfg.copy')}</Button></p>
        {/if}
      </section>
    {/if}

    {#if shows('order')}
      <section aria-labelledby="viewcfg-order">
        <h3 id="viewcfg-order">{t('app.viewcfg.order')}</h3>
        {#if isList && groupings.length > 1}
          <p class="label">{t('app.viewcfg.group_by')}</p>
          <div class="chips" role="group" aria-label={t('app.viewcfg.group_by')}>
            {#each groupings as each (each.value)}
              <button type="button" class="chip" aria-pressed={setup.grouping === each.value} data-grouping={each.value} onclick={() => (setup = { ...setup, grouping: each.value })}>{each.label}</button>
            {/each}
          </div>
        {/if}
        <div class="sort">
          <Select label={t('app.viewcfg.sort_by')} options={[{ value: '', label: t('app.query.sort_manual') }, ...sortFields]} value={setup.order?.field ?? ''} onchange={(event) => { const field = (event.currentTarget as HTMLSelectElement).value; setup = { ...setup, order: field ? { field, dir: setup.order?.dir ?? 'ASC' } : undefined }; }} />
          {#if setup.order}
            <div class="chips" role="group" aria-label={t('app.viewcfg.direction')}>
              {#each ['ASC', 'DESC'] as const as dir (dir)}
                <button type="button" class="chip" aria-pressed={setup.order.dir === dir} onclick={() => setup.order && (setup = { ...setup, order: { field: setup.order.field, dir } })}>{t(`app.viewcfg.dir_${dir}`)}</button>
              {/each}
            </div>
          {/if}
        </div>
        {#if isList}
          <p class="label">{t('app.viewcfg.done')}</p>
          <div class="chips" role="group" aria-label={t('app.viewcfg.done')}>
            {#each ['inline', 'bottom', 'hidden'] as each (each)}
              <button type="button" class="chip" aria-pressed={setup.done === each} onclick={() => (setup = { ...setup, done: each })}>{t(`app.list.done_${each}`)}</button>
            {/each}
          </div>
        {/if}
      </section>
    {/if}

    {#if isList && shows('fields')}
      <section aria-labelledby="viewcfg-fields">
        <h3 id="viewcfg-fields">{t('app.viewcfg.fields')}</h3>
        <MultiSelect
          label={t('app.viewcfg.fields')}
          aria-labelledby="viewcfg-fields"
          presentation="inline"
          options={fields.map((each) => ({ value: each.value, label: each.label, hint: each.isNew ? t('app.viewcfg.new') : undefined }))}
          selected={setup.fields}
          onChange={chooseFields}
          {...chooseSeveral()}
        />
      </section>
    {/if}

    {#if isBoard && shows('board')}
      <section aria-labelledby="viewcfg-board">
        <h3 id="viewcfg-board">{t('app.viewcfg.board')}</h3>
        <Select label={t('app.viewcfg.columns_by')} options={boardFields} value={setup.columnsBy} onchange={(event) => (setup = { ...setup, columnsBy: (event.currentTarget as HTMLSelectElement).value })} />
        <Select label={t('app.viewcfg.lanes_by')} options={[{ value: '', label: t('app.viewcfg.no_lanes') }, ...boardFields]} value={setup.lanesBy} data-lanes-by onchange={(event) => (setup = { ...setup, lanesBy: (event.currentTarget as HTMLSelectElement).value })} />
        <Switch label={t('app.viewcfg.limits')} checked={setup.limits} onchange={(event) => (setup = { ...setup, limits: (event.currentTarget as HTMLInputElement).checked })} />
      </section>
    {/if}
    {#if setup.layout === 'CALENDAR' && shows('board')}
      <section aria-labelledby="viewcfg-calendar">
        <h3 id="viewcfg-calendar">{t('app.viewcfg.calendar')}</h3>
        <Select label={t('app.viewcfg.date_field')} options={dateFields} value={setup.dateField} onchange={(event) => (setup = { ...setup, dateField: (event.currentTarget as HTMLSelectElement).value })} />
      </section>
    {/if}

    {#if shows('save')}
      <section class="save" aria-labelledby="viewcfg-save" data-changed={changed > 0 ? '' : undefined}>
        <div class="heading">
          <h3 id="viewcfg-save">{#if changed > 0}<span class="dot" aria-hidden="true"></span>{t('app.viewcfg.changed_against', { name: baseName })}{:else}{t('app.viewcfg.page_save')}{/if}</h3>
          {#if changed > 0}<Button size="sm" tone="subtle" onclick={() => (setup = base)}>{t('app.viewcfg.discard')}</Button>{/if}
        </div>
        <Input label={t('app.views.name')} bind:value={name} error={isNameMissing ? t('views.name_empty') : undefined} />
        <p class="label">{t('app.viewcfg.lives')}</p>
        <div class="chips" role="group" aria-label={t('app.viewcfg.lives')}>
          {#each scopes as each (each.value)}
            <button type="button" class="chip" aria-pressed={scope === each.value} data-scope={each.value} onclick={() => (scope = each.value)}>{each.label}</button>
          {/each}
        </div>
        {#if mayShare && scope !== 'ACCOUNT'}
          <p class="label">{t('app.viewcfg.who_sees')}</p>
          <div class="chips" role="group" aria-label={t('app.viewcfg.who_sees')}>
            {#each ['PRIVATE', 'SCOPE'] as const as each (each)}
              <button type="button" class="chip" aria-pressed={sharing === each} onclick={() => (sharing = each)}>{t(`app.viewcfg.sharing_${each}`)}</button>
            {/each}
          </div>
        {/if}
        {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
        <div class="acts">
          <Button tone="primary" isBusy={isSaving} data-view-save onclick={save}>{t('app.viewcfg.save_new')}</Button>
          {#if overwritable && onoverwrite && changed > 0}<Button tone="secondary" isBusy={isSaving} data-view-overwrite onclick={onoverwrite}>{t('app.viewcfg.overwrite', { name: overwritable })}</Button>{/if}
        </div>
        <div class="more">
          {#if onmanage}<Button size="sm" tone="subtle" icon="pencil" onclick={onmanage}>{t('app.views.menu_manage')}</Button>{/if}
          <Button size="sm" tone="subtle" icon="file-text" onclick={onexport}>{t('app.views.menu_export')}</Button>
          <Button size="sm" tone="subtle" icon="calendar" onclick={onfeeds}>{t('app.views.menu_feeds')}</Button>
        </div>
      </section>
    {/if}
  </div>
{/snippet}

{#snippet resetAct()}
  {#if changed > 0}<Button size="sm" tone="subtle" data-view-reset onclick={() => (setup = base)}>{t('app.viewcfg.as_template')}</Button>{/if}
{/snippet}

{#if isPhone}
  <Sheet bind:isOpen title={t('app.viewcfg.title_short')} closeLabel={t('app.dismiss')} actions={resetAct}>{@render body()}</Sheet>
{:else if isSide}
  <ViewSide bind:isOpen title={t('app.viewcfg.title', { name: baseName })} dismissLabel={t('app.dismiss')} actions={resetAct}>{@render body()}</ViewSide>
{:else}
  <Drawer bind:isOpen edge="inline-end" title={t('app.viewcfg.title', { name: baseName })} dismissLabel={t('app.dismiss')} actions={resetAct}>{@render body()}</Drawer>
{/if}

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-300); }
  section { display: flex; flex-direction: column; gap: var(--sp-150); padding-block-end: var(--sp-300); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  section:last-child { border-block-end: none; }
  h3 { margin: 0; font-weight: var(--fw-semibold); font-size: var(--fs-065); text-transform: uppercase; color: var(--text-secondary); display: flex; align-items: center; gap: var(--sp-100); }
  .heading { display: flex; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  .quiet, .label { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
  .label { color: var(--text-primary); }
  .layouts { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--sp-1000), 1fr)); gap: var(--sp-100); }
  .layout { display: flex; flex-direction: column; align-items: flex-start; gap: var(--sp-050); padding: var(--sp-150); text-align: start; font-size: var(--fs-075); color: var(--text-secondary); background: var(--bg-surface); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); cursor: pointer; min-block-size: var(--layout-target); }
  .layout b { color: var(--text-primary); }
  .layout[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); box-shadow: inset 0 0 0 var(--bw-hairline) var(--accent-primary); }
  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .chip { min-block-size: var(--layout-target); padding-inline: var(--sp-150); font-size: var(--fs-075); color: var(--text-primary); background: var(--bg-surface); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-md); cursor: pointer; }
  .chip[aria-pressed='true'] { color: var(--text-brand); border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  .sort { display: flex; flex-wrap: wrap; align-items: flex-end; gap: var(--sp-100); }
  .new { font-weight: var(--fw-semibold); font-size: var(--fs-065); color: var(--text-brand); background: var(--accent-primary-subtle); border-radius: var(--r-full); padding-inline: var(--sp-100); }
  .as-line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
  .as-line code { font-family: var(--font-mono); font-size: var(--fs-075); color: var(--text-primary); background: var(--bg-surface-sunken); border-radius: var(--r-sm); padding-inline: var(--sp-050); overflow-wrap: anywhere; }
  .save[data-changed] { background: var(--accent-primary-subtle); border-radius: var(--r-md); padding: var(--sp-200); border: none; }
  .dot { inline-size: var(--sp-100); block-size: var(--sp-100); border-radius: var(--r-full); background: var(--accent-primary); }
  .acts, .more { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .pages { list-style: none; margin: 0; padding: 0; }
  .pages button { display: flex; align-items: center; gap: var(--sp-150); inline-size: 100%; min-block-size: var(--layout-target); padding-block: var(--sp-150); font-size: var(--fs-090); color: var(--text-primary); background: none; border: none; border-block-end: var(--bw-hairline) solid var(--border-subtle); cursor: pointer; text-align: start; }
  .pages button span { flex: 1; }
</style>
