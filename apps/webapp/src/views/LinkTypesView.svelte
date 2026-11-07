<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Link types* under *How we work* (UC-PLN-01 check 5) as a list page (page-system §7.1, §9.1):
  // the six every workspace has and its own, one row each. *New link type* in the head and a row
  // open the type's Dialog M; ordering, archiving and deleting are in the row's ⋯, deleting behind
  // a Dialog S.
  //
  // **Each action acts at once**, as on *Types*: a link type is one small record. A type in use is
  // refused by the server on delete and can only be archived, which the refusal says.
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE at the workspace the
  // page lists the types and offers no control.

  import { untrack } from 'svelte';
  import {
    Badge, Button, DataTable, Dialog, EmptyState, FilterBar, ListPage, Spinner,
    type DataColumn, type DataRowMenu, type MenuItem,
  } from '@hubtask/design-system/components';
  import type { LinkType } from '@hubtask/sync-engine';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { linkTypes, sideName } from '../lib/data/links.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import LinkTypeDialog from '../lib/linktypes/LinkTypeDialog.svelte';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';

  $effect(() => page.entitle(t('app.link_types.title')));
  $effect(() => untrack(() => linkTypes.open()));

  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const reading = $derived(linkTypes.state);
  const readRefusal = $derived(reading?.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  const ordered = $derived([...linkTypes.all].sort((a, b) => a.position - b.position));
  const movable = $derived(ordered.filter((each) => !each.system && !each.archived));

  const nameOf = (type: LinkType) => sideName(type, type.key, true, t);
  const inverseOf = (type: LinkType) => sideName(type, type.key, false, t);

  /** What a type does, in one line: how it reads from the other end, and its rules. */
  function behaviour(type: LinkType): string {
    return [
      type.direction === 'SYMMETRIC' ? t('app.link_types.symmetric') : inverseOf(type),
      ...(type.waits === 'SOURCE' ? [t('app.link_types.waits_source')] : []),
      ...(type.waits === 'TARGET' ? [t('app.link_types.waits_target')] : []),
      ...(type.cardinality === 'ONE' ? [t('app.link_types.one')] : []),
      ...(type.cycles === 'ALLOWED' ? [t('app.link_types.loops')] : []),
    ].join(' · ');
  }

  let search = $state('');
  const shown = $derived(ordered.filter((type) => matchesSearch(search, [nameOf(type), inverseOf(type), type.key])));

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.link_types.column_name'), priority: 1 },
    { id: 'reads', label: t('app.link_types.column_reads'), priority: 2 },
    { id: 'state', label: t('app.link_types.column_state'), priority: 2, isStatus: true },
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

  /** Swaps a type with its neighbour among the movable ones, writing both positions. */
  function move(type: LinkType, by: -1 | 1) {
    const at = movable.findIndex((each) => each.key === type.key);
    const other = movable[at + by];
    if (!other) return;
    void act(async () => {
      await linkTypes.change(type, { position: other.position });
      await linkTypes.change(other, { position: type.position });
    });
  }

  function archive(type: LinkType, archived: boolean) {
    const code = archived ? 'app.link_types.archived_done' : 'app.link_types.unarchived_done';
    void act(() => linkTypes.change(type, { archived }), t(code, { name: nameOf(type) }));
  }

  let editing = $state<LinkType | 'new' | undefined>(undefined);
  let deleting = $state<LinkType | undefined>(undefined);

  function rowMenu(type: LinkType): DataRowMenu | undefined {
    if (!mayShape || type.system) return undefined;
    const index = movable.findIndex((each) => each.key === type.key);
    const items: MenuItem[] = [
      { id: 'edit', label: t('app.link_types.edit_menu') },
      ...(index >= 0
        ? [
            { id: 'up', label: t('app.link_types.move_up_menu'), disabledReason: index === 0 ? t('app.link_types.first') : undefined, hasSeparatorBefore: true },
            { id: 'down', label: t('app.link_types.move_down_menu'), disabledReason: index === movable.length - 1 ? t('app.link_types.last') : undefined },
          ]
        : []),
      { id: 'archive', label: type.archived ? t('app.link_types.unarchive') : t('app.link_types.archive'), hasSeparatorBefore: index < 0 },
      { id: 'delete', label: t('app.link_types.delete_menu'), isDestructive: true, hasSeparatorBefore: true },
    ];
    return { label: t('app.page.more_for', { name: nameOf(type) }), items };
  }

  function onRowMenu(type: LinkType, id: string) {
    if (id === 'edit') editing = type;
    else if (id === 'up') move(type, -1);
    else if (id === 'down') move(type, 1);
    else if (id === 'archive') archive(type, !type.archived);
    else if (id === 'delete') deleting = type;
  }

  function confirmDelete() {
    const type = deleting;
    deleting = undefined;
    if (type) void act(() => linkTypes.remove(type), t('app.link_types.deleted', { name: nameOf(type) }));
  }

  let isFileOpen = $state(false);
  const fileAction = $derived(viewAsFileAction(() => (isFileOpen = true)));
  const head = $derived({
    title: t('app.link_types.title'),
    breadcrumb: adminTrail({ id: 'link-types', label: t('app.link_types.title') }),
    primary: mayShape && !readRefusal ? { label: t('app.link_types.new'), icon: 'plus' as const, opener: 'new-link-type', onclick: () => (editing = 'new') } : undefined,
    secondary: fileAction ? ([fileAction] as const) : ([] as const),
  });
</script>

<ListPage {head}>
  {#snippet intro()}<p class="quiet">{t('app.link_types.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.link_types.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      countLabel={reading?.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(ordered.length) }) : undefined}
    />
  {/snippet}

  {#if failure}<FailureNote problem={failure} />{/if}

  {#if !reading || reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.link_types.reading')} /> <span>{t('app.link_types.reading')}</span></p>
  {:else if readRefusal}
    <FailureNote problem={readRefusal} />
  {:else}
    <DataTable
      label={t('app.link_types.title')}
      {columns}
      rows={shown}
      key={(type: LinkType) => type.key}
      onopen={mayShape ? (type: LinkType) => (editing = type) : undefined}
      rowLabel={nameOf}
      menu={rowMenu}
      onmenu={onRowMenu}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={ordered.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(type: LinkType, column: DataColumn)}
        {#if column.id === 'name'}
          <span class="name" class:archived={type.archived} data-link-type={type.key}>{nameOf(type)}</span>
        {:else if column.id === 'reads'}
          <span class="quiet">{behaviour(type)}</span>
        {:else if type.archived}
          <Badge tone="neutral">{t('app.link_types.archived')}</Badge>
        {:else if type.system}
          <span class="quiet">{t('app.link_types.system')}</span>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if ordered.length === 0}
          <EmptyState kind="unused" title={t('app.link_types.none')} />
        {:else}
          <EmptyState kind="filtered" title={t('app.link_types.filtered_title')}>
            {#snippet action()}<Button onclick={() => (search = '')}>{t('app.people.filtered_clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

<LinkTypeDialog subject={editing} onClose={() => (editing = undefined)} />

<Dialog
  size="s"
  isOpen={deleting !== undefined}
  title={t('app.link_types.delete_title', { name: deleting ? nameOf(deleting) : '' })}
  dismissLabel={t('app.link_types.cancel')}
  onClose={() => (deleting = undefined)}
  primary={{ label: t('app.link_types.delete_confirm'), tone: 'danger', onclick: confirmDelete }}
>
  <p class="quiet">{t('app.link_types.delete_explain')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (deleting = undefined)}>{t('app.link_types.cancel')}</Button>
  {/snippet}
</Dialog>

<ViewSectionsAsFile sections={['link_types']} bind:isOpen={isFileOpen} isTriggerHidden />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .name.archived { color: var(--text-subtle); }
</style>
