<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import Button from './Button.svelte';
  import DataTable from './DataTable.svelte';
  import Dialog from './Dialog.svelte';
  import EmptyState from './EmptyState.svelte';
  import RecordPage from './RecordPage.svelte';
  import Refused from './_RefusedDemo.svelte';

  const { content = 'full' }: { content?: 'full' | 'empty' | 'refused' } = $props();

  let isDeleting = $state(false);
  const isEmpty = $derived(content === 'empty');

  const members = [
    { id: 'lb', name: 'Lukas Brandt', role: 'Administrator', added: 'Oct 2, 2026' },
    { id: 'pn', name: 'Priya Natarajan', role: 'Member', added: 'Oct 2, 2026' },
  ];
  type Member = (typeof members)[number];
  const roles = [{ id: 'cab', role: 'CAB member', where: 'IT-Service-Desk' }];
  type Grant = (typeof roles)[number];
  const memberMenu = (member: Member) => ({ label: `More for ${member.name}`, items: [{ id: 'open', label: 'Open person' }, { id: 'out', label: 'Take out of the group', isDestructive: true, hasSeparatorBefore: true }] });
</script>

<RecordPage
  record={{
    kind: 'Group',
    title: 'Change Advisory Board',
    back: { label: 'Groups', href: '#groups' },
    facts: isEmpty ? 'Approves normal and emergency changes · no members · no role · created just now' : 'Approves normal and emergency changes · 2 members · 1 role · changed 2 days ago',
    edit: { label: 'Edit', onclick: () => {} },
    menu: { label: 'More for Change Advisory Board', items: [{ id: 'copy', label: 'Copy as new group' }, { id: 'file', label: 'View as file' }, { id: 'delete', label: 'Delete group…', isDestructive: true, hasSeparatorBefore: true }], onselect: (id) => { if (id === 'delete') isDeleting = true; } },
  }}
  lists={[
    { id: 'members', title: 'Members', count: isEmpty ? '0' : '2', add: { label: 'Add people', icon: 'plus', onclick: () => {} } },
    { id: 'roles', title: 'Roles it grants', count: isEmpty ? '0' : '1', add: { label: 'Grant a role', icon: 'plus', onclick: () => {} } },
  ]}
  facts={{ title: 'About it', rows: [{ label: 'Created', value: 'Sep 30, 2026 by Jana Krüger' }, { label: 'Changed', value: 'Nov 5, 2026 by Lukas Brandt' }] }}
  danger={{ title: 'Danger zone', acts: [{ id: 'delete', title: 'Delete this group', cost: isEmpty ? 'Nobody is in it; nothing else changes.' : 'Its 2 people keep their own roles; the role it grants ends for them.', actionLabel: 'Delete group…' }], onact: () => (isDeleting = true) }}
  refused={content === 'refused' ? refused : undefined}
>
  {#snippet list(item)}
    {#if item.id === 'members'}
      <DataTable label="Members" columns={[{ id: 'name', label: 'Name', priority: 1 }, { id: 'role', label: 'Role in the workspace', priority: 2 }, { id: 'added', label: 'Added', priority: 3 }]} rows={isEmpty ? [] : members} key={(member: Member) => member.id} href={(member: Member) => `#person-${member.id}`} rowLabel={(member: Member) => member.name} menu={memberMenu} sheetDismissLabel="Close" sheetGrabLabel="Pull to full height" actionsLabel="Actions">
        {#snippet cell(member: Member, column)}{column.id === 'name' ? member.name : column.id === 'role' ? member.role : member.added}{/snippet}
        {#snippet empty()}<EmptyState kind="unused" title="Nobody is in this group" description="Add people, and they get the roles it grants." icon="users" />{/snippet}
      </DataTable>
    {:else}
      <DataTable label="Roles it grants" columns={[{ id: 'role', label: 'Role', priority: 1 }, { id: 'where', label: 'Where', priority: 2 }]} rows={isEmpty ? [] : roles} key={(grant: Grant) => grant.id} rowLabel={(grant: Grant) => grant.role} menu={() => ({ label: 'More for CAB member', items: [{ id: 'withdraw', label: 'Withdraw…', isDestructive: true }] })} sheetDismissLabel="Close" sheetGrabLabel="Pull to full height" actionsLabel="Actions">
        {#snippet cell(grant: Grant, column)}{#if column.id === 'role'}<Badge size="sm">{grant.role}</Badge>{:else}{grant.where}{/if}{/snippet}
        {#snippet empty()}<EmptyState kind="unused" title="It grants no role yet" description="A role granted here reaches everybody in the group." icon="shield" />{/snippet}
      </DataTable>
    {/if}
  {/snippet}
</RecordPage>

<Dialog bind:isOpen={isDeleting} size="s" title="Delete “Change Advisory Board”?" primary={{ label: 'Delete group', tone: 'danger', onclick: () => (isDeleting = false) }}>
  Its 2 people keep their own roles. The role “CAB member” it grants ends for them. This cannot be undone.
  {#snippet actions()}<Button tone="subtle" onclick={() => (isDeleting = false)}>Cancel</Button>{/snippet}
</Dialog>

{#snippet refused()}<Refused what="this group" />{/snippet}
