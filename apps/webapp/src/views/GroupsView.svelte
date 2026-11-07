<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's groups as *People*'s *Groups* list page (page-system §7.1, §9.2): one row per
  // group, opening its record page (GroupView), where its members and roles are; *New group* in the
  // head opens a Dialog M.
  //
  // **A group is a way to grant a role once instead of eleven times.** That is also why deleting
  // one is not a tidy-up: what the group granted, nobody holds afterwards - the confirmation says
  // so before the button.
  //
  // **A group passes on every role it holds.** The server answers `members_changeable`, and the
  // delete is offered only where it says yes (domain-model.md §3.2, P-05). Renaming places
  // nothing, so it asks only MANAGE_MEMBERS, like every write here.
  //
  // **Any member may read this.** A membership granted to a group is unreadable until the group
  // can be shown as the people it reaches.

  import { untrack } from 'svelte';

  import {
    Button,
    DataTable,
    Dialog,
    EmptyState,
    FilterBar,
    Input,
    ListPage,
    Spinner,
    Stack,
    Textarea,
    type DataColumn,
    type DataRowMenu,
    type MenuItem,
  } from '@hubtask/design-system/components';
  import type { Group } from '@hubtask/sync-engine';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import DeleteGroupDialog from '../lib/people/DeleteGroupDialog.svelte';
  import EditGroupDialog from '../lib/people/EditGroupDialog.svelte';
  import PeopleStrip from '../lib/people/PeopleStrip.svelte';
  import { peopleHead } from '../lib/people/peoplehead.svelte.ts';
  import { DiscardGuard } from '../lib/page/discard.svelte.ts';
  import DiscardDialog from '../lib/page/DiscardDialog.svelte';
  import { matchesSearch } from '../lib/page/list.ts';
  import { holdsHere, mayHere } from '../lib/data/capability.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { isComplete, type Missing } from '../lib/forms/missing.ts';
  import { marked } from '../lib/forms/press.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  const { onnavigate, onopen }: { onnavigate?: (path: string) => void; onopen?: (path: string) => void } = $props();

  const TENANT = { scopeType: 'TENANT' } as const;

  $effect(() =>
    untrack(() => {
      people.openScope(TENANT);
      return groups.open();
    }),
  );
  $effect(() => page.entitle(t('app.people.title')));

  const reading = $derived(groups.state);
  const all = $derived(groups.all);

  $effect(() => {
    // One read per group, for its members and whether this reader may change them.
    for (const group of all) if (!groups.detail(group.id)) void groups.read(group.id);
  });

  const membersOf = (groupId: string): readonly string[] => groups.detail(groupId)?.members ?? [];
  const changeable = (groupId: string): boolean => groups.detail(groupId)?.members_changeable === true;
  const mayRename = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const mayCreate = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');

  // The search narrows the groups loaded: `GET /groups` has no search of its own yet.
  let search = $state('');
  const shown = $derived(all.filter((group) => matchesSearch(search, [group.name, group.description ?? undefined])));

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.groups.name'), isSortable: true, priority: 1 },
    { id: 'description', label: t('app.groups.description'), priority: 2 },
    { id: 'members', label: t('app.groups.members'), isSortable: true, priority: 3 },
  ]);
  const order = sorting({
    rows: () => shown,
    read: (group: Group, columnId) => (columnId === 'members' ? membersOf(group.id).length : group.name),
  });

  // New group: a name and a description, a Dialog M.
  let isCreating = $state(false);
  let newName = $state('');
  let newDescription = $state('');
  let missing = $state<Missing>({});
  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);
  let createForm = $state<HTMLFormElement | undefined>(undefined);

  function reset(): void {
    newName = '';
    newDescription = '';
    missing = {};
    failure = undefined;
  }
  const createGuard = new DiscardGuard(() => newName.trim() !== '' || newDescription.trim() !== '', () => {
    isCreating = false;
    reset();
  });

  async function create(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const name = newName.trim();
    missing = marked(event, [{ kind: 'field', key: 'new', code: 'app.forms.required', isFilled: name !== '' }]);
    if (!isComplete(missing)) return;
    failure = undefined;
    isWriting = true;
    try {
      await groups.create(name, newDescription.trim() || undefined);
      announcer.say(t('app.groups.created_announced'));
      toast.show({ text: t('app.groups.created_toast', { name }), tone: 'success' });
      isCreating = false;
      reset();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }

  // A row's ⋯: open it, edit it, delete it - the destructive one last.
  let editing = $state<Group | undefined>(undefined);
  let removing = $state<Group | undefined>(undefined);

  function rowMenu(group: Group): DataRowMenu {
    const items: MenuItem[] = [
      { id: 'open', label: t('app.page.open') },
      ...(mayRename ? [{ id: 'edit', label: t('app.groups.edit_menu') }] : []),
      ...(changeable(group.id) ? [{ id: 'delete', label: t('app.groups.delete_menu'), isDestructive: true, hasSeparatorBefore: true }] : []),
    ];
    return { label: t('app.page.more_for', { name: group.name }), items };
  }

  function onRowMenu(group: Group, id: string): void {
    if (id === 'open') onopen?.(`/administration/groups/${group.id}`);
    else if (id === 'edit') editing = group;
    else if (id === 'delete') removing = group;
  }

  const primary = $derived(
    mayCreate
      ? { label: t('app.groups.new_title'), icon: 'plus' as const, opener: 'new-group', onclick: () => { reset(); isCreating = true; } }
      : undefined,
  );
</script>

<ListPage head={peopleHead(primary)}>
  {#snippet strip()}<PeopleStrip current="groups" {onnavigate} />{/snippet}
  {#snippet intro()}<p class="quiet">{t('app.groups.intro')}</p>{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.groups.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(all.length) }) : undefined}
    />
  {/snippet}

  <!-- Says once, to a reader who may not manage members, why nothing here changes. -->
  <WorkspaceGate permission="MANAGE_MEMBERS" />

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.groups.reading')} /> <span>{t('app.groups.reading')}</span></p>
  {:else}
    <DataTable
      label={t('app.groups.title')}
      {columns}
      rows={order.rows}
      key={(group: Group) => group.id}
      href={(group: Group) => `/administration/groups/${group.id}`}
      rowLabel={(group: Group) => group.name}
      menu={rowMenu}
      onmenu={onRowMenu}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      sort={order.sort}
      onSort={(next) => order.by(next)}
      shownLabel={all.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(group: Group, column: DataColumn)}
        {#if column.id === 'name'}
          <span class="name">{group.name}</span>
        {:else if column.id === 'description'}
          <span class="quiet">{group.description ?? ''}</span>
        {:else if groups.detail(group.id)}
          {t('app.groups.member_count', { count: String(membersOf(group.id).length) })}
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if all.length === 0}
          <EmptyState kind="unused" title={t('app.groups.none')} />
        {:else}
          <EmptyState kind="filtered" title={t('app.groups.filtered_title')}>
            {#snippet action()}<Button onclick={() => (search = '')}>{t('app.people.filtered_clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#if mayCreate}
  <Dialog
    size="m"
    title={t('app.groups.new_title')}
    bind:isOpen={isCreating}
    dismissLabel={t('app.groups.cancel')}
    onDismiss={createGuard.dismiss}
    onClose={reset}
    primary={{ label: t('app.groups.create'), isBusy: isWriting, busyLabel: t('app.groups.working'), onclick: () => createForm?.requestSubmit() }}
  >
    <form bind:this={createForm} onsubmit={create}>
      <Stack gap="150">
        {#if failure}<FailureNote problem={failure} />{/if}
        <Input label={t('app.groups.name')} hint={t('app.groups.name_hint')} bind:value={newName} isRequired error={missing.new && !newName.trim() ? t(missing.new) : undefined} />
        <Textarea label={t('app.groups.description')} hint={t('app.groups.description_hint')} bind:value={newDescription} />
      </Stack>
    </form>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => { if (createGuard.dismiss()) { isCreating = false; reset(); } }}>{t('app.groups.cancel')}</Button>
    {/snippet}
  </Dialog>
  <DiscardDialog guard={createGuard} />
{/if}

<EditGroupDialog group={editing} onClose={() => (editing = undefined)} />
<DeleteGroupDialog group={removing} memberCount={removing ? membersOf(removing.id).length : 0} onClose={() => (removing = undefined)} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  form { margin: 0; }
</style>
