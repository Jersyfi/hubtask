<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Types* under *How we work* (UC-FLW-01 check 1) as a list page (page-system §7.1, §9.1): the
  // workspace's entry types grouped by level, one row each with its mark, level, whether it has a
  // form and its state. A row opens the type's own page (EntryTypeView), where its form and what it
  // does are; *New type* in the head opens a Dialog M; ordering, archiving and deleting are in the
  // row's ⋯, deleting behind a Dialog S. The page is a row of the workflows module, so it is
  // reached only where the module is on (check 8).
  //
  // **Each action acts at once**, as a module switch does: a type is one small record. A type in
  // use is refused by the server and can only be archived (check 5), which the refusal says.
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE at the workspace the
  // page lists the types and offers no control.

  import { untrack } from 'svelte';
  import {
    Badge, Button, DataTable, EmptyState, FilterBar, Icon, LabelChip, ListPage, Spinner,
    type DataColumn, type DataRowMenu, type MenuItem,
  } from '@hubtask/design-system/components';
  import type { EntryType } from '@hubtask/sync-engine';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { hasForm } from '../lib/data/entryforms.ts';
  import { TYPE_LEVELS, entryTypes, iconOf, levelTypes, reorder } from '../lib/data/entrytypes.svelte.ts';
  import DeleteTypeDialog from '../lib/entrytypes/DeleteTypeDialog.svelte';
  import NewTypeDialog from '../lib/entrytypes/NewTypeDialog.svelte';
  import TypeDialog from '../lib/entrytypes/TypeDialog.svelte';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import SectionFindings from '../lib/settings/SectionFindings.svelte';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';

  const { onopen }: { onopen?: (path: string) => void } = $props();

  $effect(() => page.entitle(t('app.entry_types.title')));
  $effect(() => untrack(() => entryTypes.open()));

  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const reading = $derived(entryTypes.state);
  const readRefusal = $derived(reading?.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  const levelName = (level: string) => t('app.entry_types.level', { level });
  const nameOf = (type: EntryType) => type.name ?? levelName(type.level);
  const pathOf = (type: EntryType) => `/administration/types/${encodeURIComponent(type.key)}`;

  /** Every type, level by level in the levels' order, each level in its own order. */
  const ordered = $derived(TYPE_LEVELS.flatMap((level) => levelTypes(entryTypes.all, level)));

  let search = $state('');
  const shown = $derived(ordered.filter((type) => matchesSearch(search, [nameOf(type), type.description ?? undefined])));

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.entry_types.column_name'), priority: 1 },
    { id: 'level', label: t('app.entry_types.column_level'), priority: 3 },
    { id: 'form', label: t('app.entry_types.form'), priority: 2 },
    { id: 'state', label: t('app.entry_types.column_state'), priority: 2, isStatus: true },
  ]);

  let failure = $state<string | undefined>(undefined);

  async function act(run: () => Promise<unknown>, done?: string) {
    failure = undefined;
    try {
      await run();
      if (done) toast.show({ text: done, tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  function move(type: EntryType, by: -1 | 1) {
    const moves = reorder(entryTypes.all, type.key, by);
    void act(async () => {
      for (const { type: each, position } of moves) await entryTypes.change(each, { position });
    });
  }

  function archive(type: EntryType, archived: boolean) {
    const code = archived ? 'app.entry_types.archived_done' : 'app.entry_types.unarchived_done';
    void act(() => entryTypes.change(type, { archived }), t(code, { name: nameOf(type) }));
  }

  let isCreating = $state(false);
  let editing = $state<EntryType | undefined>(undefined);
  let deleting = $state<EntryType | undefined>(undefined);

  function rowMenu(type: EntryType): DataRowMenu {
    const movable = levelTypes(entryTypes.all, type.level).filter((each) => !each.system && !each.archived);
    const index = movable.findIndex((each) => each.key === type.key);
    const items: MenuItem[] = [
      { id: 'open', label: t('app.page.open') },
      ...(mayShape
        ? [
            { id: 'edit', label: t('app.entry_types.edit_menu') },
            ...(index >= 0
              ? [
                  { id: 'up', label: t('app.entry_types.move_up_menu'), disabledReason: index === 0 ? t('app.entry_types.first') : undefined, hasSeparatorBefore: true },
                  { id: 'down', label: t('app.entry_types.move_down_menu'), disabledReason: index === movable.length - 1 ? t('app.entry_types.last') : undefined },
                ]
              : []),
            ...(type.system
              ? []
              : [
                  { id: 'archive', label: type.archived ? t('app.entry_types.unarchive') : t('app.entry_types.archive'), hasSeparatorBefore: index < 0 },
                  { id: 'delete', label: t('app.entry_types.delete_menu'), isDestructive: true, hasSeparatorBefore: true },
                ]),
          ]
        : []),
    ];
    return { label: t('app.page.more_for', { name: nameOf(type) }), items };
  }

  function onRowMenu(type: EntryType, id: string) {
    if (id === 'open') onopen?.(pathOf(type));
    else if (id === 'edit') editing = type;
    else if (id === 'up') move(type, -1);
    else if (id === 'down') move(type, 1);
    else if (id === 'archive') archive(type, !type.archived);
    else if (id === 'delete') deleting = type;
  }

  let isFileOpen = $state(false);
  const fileAction = $derived(viewAsFileAction(() => (isFileOpen = true)));
  const head = $derived({
    title: t('app.entry_types.title'),
    breadcrumb: adminTrail({ id: 'entry-types', label: t('app.entry_types.title') }),
    primary: mayShape && !readRefusal ? { label: t('app.entry_types.new'), icon: 'plus' as const, opener: 'new-type', onclick: () => (isCreating = true) } : undefined,
    secondary: fileAction ? ([fileAction] as const) : ([] as const),
  });
</script>

<ListPage {head}>
  {#snippet intro()}<p class="quiet">{t('app.entry_types.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.entry_types.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      countLabel={reading?.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(ordered.length) }) : undefined}
    />
  {/snippet}

  <SectionFindings sections={['types']} />
  {#if failure}<FailureNote problem={failure} />{/if}

  {#if !reading || reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.entry_types.reading')} /> <span>{t('app.entry_types.reading')}</span></p>
  {:else if readRefusal}
    <FailureNote problem={readRefusal} />
  {:else}
    <DataTable
      label={t('app.entry_types.title')}
      {columns}
      rows={shown}
      key={(type: EntryType) => type.key}
      href={pathOf}
      group={(type: EntryType) => t('app.entry_types.level_heading', { level: type.level })}
      rowLabel={nameOf}
      menu={rowMenu}
      onmenu={onRowMenu}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={ordered.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(type: EntryType, column: DataColumn)}
        {#if column.id === 'name'}
          <span class="identity" class:archived={type.archived} data-entry-type={type.key}>
            <Icon name={iconOf(type)} size="sm" />
            <LabelChip name={nameOf(type)} colorToken={type.color_token} />
          </span>
        {:else if column.id === 'level'}
          <span class="quiet">{levelName(type.level)}</span>
        {:else if column.id === 'form'}
          <span class="quiet" data-has-form={hasForm(type.form) ? '' : undefined}>
            {hasForm(type.form) ? t('app.entry_types.form_set') : t('app.entry_types.form_none')}
          </span>
        {:else if type.archived}
          <Badge tone="neutral">{t('app.entry_types.archived')}</Badge>
        {:else if type.system}
          <span class="quiet">{t('app.entry_types.system')}</span>
        {:else if type.record}
          <span class="quiet">{t('app.entry_types.record')}</span>
        {/if}
      {/snippet}
      {#snippet empty()}
        <EmptyState kind="filtered" title={t('app.entry_types.filtered_title')}>
          {#snippet action()}<Button onclick={() => (search = '')}>{t('app.people.filtered_clear')}</Button>{/snippet}
        </EmptyState>
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#if mayShape}
  <NewTypeDialog isOpen={isCreating} onClose={() => (isCreating = false)} />
  <TypeDialog type={editing} onClose={() => (editing = undefined)} />
  <DeleteTypeDialog type={deleting} onClose={() => (deleting = undefined)} />
{/if}

<ViewSectionsAsFile sections={['types']} bind:isOpen={isFileOpen} isTriggerHidden />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .identity { display: inline-flex; align-items: center; gap: var(--sp-100); }
  .identity.archived { color: var(--text-subtle); }
</style>
