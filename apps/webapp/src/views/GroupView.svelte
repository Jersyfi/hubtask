<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One group as its record page (page-system §5.1, §7.2): its members, the roles it grants, *Edit*
  // for its own fields (a Dialog M), *Add people* as a Dialog L with search, and the delete behind a
  // Dialog S and in the danger zone.
  //
  // **Members are set whole.** `PATCH /groups/{id}` takes "the complete membership after the
  // change", so adding one person means sending the list with them in it; this page composes it.
  //
  // **What the server says the reader may change.** `members_changeable` decides the member
  // controls and the delete (domain-model.md §3.2, P-05); where it says no to somebody who manages
  // members, the members' heading says why. The roles listed are what the workspace's memberships
  // and its own roles' grants say the group holds - read, not changed, here.

  import { untrack } from 'svelte';

  import {
    Button,
    CheckboxGroup,
    DataTable,
    Dialog,
    EmptyState,
    RecordPage,
    RoleBadge,
    SearchField,
    Spinner,
    Stack,
    type DataColumn,
    type DataRowMenu,
    type MenuItem,
    type RecordList,
  } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import DeleteGroupDialog from '../lib/people/DeleteGroupDialog.svelte';
  import EditGroupDialog from '../lib/people/EditGroupDialog.svelte';
  import { tabPath } from '../lib/people/peoplepage.ts';
  import { roleLabel } from '../lib/people/rolewords.ts';
  import { DiscardGuard } from '../lib/page/discard.svelte.ts';
  import DiscardDialog from '../lib/page/DiscardDialog.svelte';
  import { matchesSearch } from '../lib/page/list.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { roles } from '../lib/data/roles.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const TENANT = { scopeType: 'TENANT' } as const;
  const LIST = tabPath('groups');

  $effect(() =>
    untrack(() => {
      const closers = [people.openScope(TENANT), groups.open(), roles.open(), containers.start()];
      void groups.read(id).catch(() => undefined);
      return () => closers.forEach((close) => close?.());
    }),
  );

  const group = $derived(groups.all.find((each) => each.id === id));
  const detail = $derived(groups.detail(id));
  const members = $derived(detail?.members ?? []);
  const changeable = $derived(detail?.members_changeable === true);
  const mayRename = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const isRead = $derived(groups.state.status === 'ready');
  const name = $derived(group?.name ?? '');

  $effect(() => page.entitle(name || t('app.groups.title')));

  const nameFor = (accountId: string): string => accounts.nameOf(accountId) ?? t('app.people.unnamed');
  const holders = $derived(people.holders({}, TENANT));
  const workspaceRoleOf = (accountId: string): string | undefined => holders.find((holder) => holder.accountId === accountId && holder.isHere)?.role;

  // The roles it grants: a shipped role through a workspace membership of the group, and the
  // workspace's own roles given to it anywhere.
  interface Granted {
    readonly id: string;
    readonly role: string;
    readonly where: string;
  }
  const granted = $derived<Granted[]>([
    ...holders
      .filter((holder) => holder.groupId === id && holder.isHere)
      .map((holder) => ({ id: holder.membershipId, role: roleLabel(holder.role), where: t('app.people.managed_where_workspace') })),
    ...(roles.catalogue?.grants ?? [])
      .filter((grant) => grant.group_id === id)
      .map((grant) => ({
        id: grant.id,
        role: roleLabel(grant.role, roles.catalogue?.custom),
        where: grant.scope_type === 'TENANT' || !grant.scope_id ? t('app.people.managed_where_workspace') : (containers.find(grant.scope_id)?.name ?? t('app.roles.place_unknown')),
      })),
  ]);

  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);

  /** Adding and taking out are the same call: the list as it should be afterwards. */
  async function setMembers(next: readonly string[], said: string): Promise<boolean> {
    failure = undefined;
    isWriting = true;
    try {
      await groups.update(id, { members: next });
      announcer.say(said);
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    } finally {
      isWriting = false;
    }
  }

  // Add people: one list to pick from, with search - a Dialog L.
  let isAdding = $state(false);
  let query = $state('');
  let picked = $state<readonly string[]>([]);
  let pickMissing = $state(false);
  const candidates = $derived(
    people
      .candidates({})
      .filter((accountId) => !members.includes(accountId))
      .map((accountId) => ({ value: accountId, label: nameFor(accountId) })),
  );
  // The candidates are the people this workspace already holds; the search narrows those.
  const offered = $derived(candidates.filter((option) => picked.includes(option.value) || matchesSearch(query, [option.label])));
  const addGuard = new DiscardGuard(() => picked.length > 0, () => closeAdding());

  function closeAdding(): void {
    isAdding = false;
    query = '';
    picked = [];
    pickMissing = false;
  }

  async function addPicked(): Promise<void> {
    if (picked.length === 0) {
      pickMissing = true;
      return;
    }
    const count = picked.length;
    if (await setMembers([...members, ...picked], t('app.groups.members_announced'))) {
      toast.show({ text: t('app.groups.added_toast', { count: String(count), name }), tone: 'success' });
      closeAdding();
    }
  }

  const memberColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.groups.member_name'), priority: 1 },
    { id: 'role', label: t('app.groups.member_role'), priority: 2 },
  ]);
  const roleColumns = $derived<DataColumn[]>([
    { id: 'role', label: t('app.people.role_column'), priority: 1 },
    { id: 'where', label: t('app.roles.where'), priority: 2 },
  ]);

  function memberMenu(accountId: string): DataRowMenu {
    const items: MenuItem[] = [
      { id: 'open', label: t('app.groups.open_person') },
      ...(changeable ? [{ id: 'out', label: t('app.groups.remove_member'), isDestructive: true, hasSeparatorBefore: true }] : []),
    ];
    return { label: t('app.page.more_for', { name: nameFor(accountId) }), items };
  }

  function onMemberMenu(accountId: string, item: string): void {
    if (item === 'open') onnavigate(`/administration/people/${accountId}`);
    else if (item === 'out') void setMembers(members.filter((each) => each !== accountId), t('app.groups.members_announced'));
  }

  const lists = $derived<RecordList[]>([
    {
      id: 'members',
      title: t('app.groups.members'),
      count: detail ? String(members.length) : undefined,
      add: changeable ? { label: t('app.groups.add_people'), icon: 'plus' as const, opener: 'add-members', onclick: () => (isAdding = true) } : undefined,
    },
    { id: 'roles', title: t('app.groups.roles_granted'), count: String(granted.length) },
  ]);

  let editing = $state(false);
  let removing = $state(false);

  const menuItems = $derived<MenuItem[]>([
    ...(changeable ? [{ id: 'delete', label: t('app.groups.delete_menu'), isDestructive: true }] : []),
  ]);
</script>

<RecordPage
  record={{
    kind: t('app.groups.kind'),
    title: name,
    back: { label: t('app.groups.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    facts: [group?.description, detail ? t('app.groups.figure', { members: String(members.length), roles: String(granted.length) }) : undefined].filter(Boolean).join(' · '),
    edit: mayRename && group ? { label: t('app.page.edit'), opener: 'edit-group', onclick: () => (editing = true) } : undefined,
    menu: menuItems.length > 0 ? { label: t('app.page.more_for', { name }), items: menuItems, onselect: () => (removing = true) } : undefined,
  }}
  {lists}
  danger={changeable && group
    ? {
        title: t('app.page.danger_zone'),
        acts: [{ id: 'delete', title: t('app.groups.delete_title_zone'), cost: t('app.groups.delete_cost'), actionLabel: t('app.groups.delete_menu') }],
        onact: () => (removing = true),
      }
    : undefined}
  refused={isRead && !group ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if item.id === 'members'}
      {#if failure}<FailureNote problem={failure} />{/if}
      {#if detail && !changeable && mayRename}
        <!-- Without MANAGE_MEMBERS no group is changeable, and the page's notice says why. -->
        <p class="quiet small">{t('app.groups.above_own')}</p>
      {/if}
      <DataTable
        label={item.title}
        columns={memberColumns}
        rows={members}
        key={(accountId: string) => accountId}
        href={(accountId: string) => `/administration/people/${accountId}`}
        rowLabel={nameFor}
        menu={memberMenu}
        onmenu={onMemberMenu}
        actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        isBusy={isWriting}
      >
        {#snippet cell(accountId: string, column: DataColumn)}
          {#if column.id === 'name'}
            {nameFor(accountId)}
          {:else if workspaceRoleOf(accountId)}
            <span class="quiet">{roleLabel(workspaceRoleOf(accountId) ?? '')}</span>
          {/if}
        {/snippet}
        {#snippet empty()}
          {#if detail}
            <EmptyState kind="settled" title={t('app.groups.empty')} />
          {:else}
            <p class="quiet waiting"><Spinner label={t('app.groups.reading')} /> <span>{t('app.groups.reading')}</span></p>
          {/if}
        {/snippet}
      </DataTable>
    {:else}
      <DataTable label={item.title} columns={roleColumns} rows={granted} key={(each: Granted) => each.id}>
        {#snippet cell(each: Granted, column: DataColumn)}
          {#if column.id === 'role'}<RoleBadge label={each.role} size="sm" />{:else}<span class="quiet">{each.where}</span>{/if}
        {/snippet}
        {#snippet empty()}
          <EmptyState kind="settled" title={t('app.groups.no_roles')} />
        {/snippet}
      </DataTable>
    {/if}
  {/snippet}
</RecordPage>

{#if changeable}
  <Dialog
    size="l"
    title={t('app.groups.add_people_title', { name })}
    bind:isOpen={isAdding}
    dismissLabel={t('app.groups.cancel')}
    onDismiss={addGuard.dismiss}
    onClose={() => { query = ''; picked = []; pickMissing = false; }}
    primary={{ label: t('app.groups.add_picked', { count: String(picked.length) }), isBusy: isWriting, busyLabel: t('app.groups.working'), onclick: () => void addPicked() }}
  >
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <SearchField label={t('app.groups.search_people')} clearLabel={t('app.page.clear_search')} bind:value={query} />
      {#if candidates.length === 0}
        <p class="quiet">{t('app.groups.nobody_to_add')}</p>
      {:else}
        <CheckboxGroup
          label={t('app.groups.add_member')}
          options={offered}
          selected={picked}
          onChange={(next) => { picked = next; pickMissing = false; }}
          error={pickMissing ? t('app.groups.pick_first') : undefined}
        />
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => { if (addGuard.dismiss()) closeAdding(); }}>{t('app.groups.cancel')}</Button>
    {/snippet}
  </Dialog>
  <DiscardDialog guard={addGuard} />
{/if}

<EditGroupDialog group={editing ? group : undefined} onClose={() => (editing = false)} />
<DeleteGroupDialog group={removing ? group : undefined} memberCount={members.length} onClose={() => (removing = false)} onDeleted={() => onnavigate(LIST)} />

{#snippet notHere()}
  <EmptyState kind="settled" title={t('app.groups.not_here')} />
{/snippet}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .waiting { padding: var(--sp-200); }
</style>
