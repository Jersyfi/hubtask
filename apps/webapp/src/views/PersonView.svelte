<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One person of the workspace as a record page (page-system §5.1, §7.2): the roles they hold at
  // the workspace, the groups they are in, and the last mail to them - each from what the contract
  // answers today, nothing more. Their sessions are not listed: no endpoint lists another
  // account's sessions.
  //
  // The person is found the way *People* finds everybody - through the workspace's memberships -
  // so somebody without one is "not here" rather than a page of empty lists.

  import { untrack } from 'svelte';

  import { DataTable, EmptyState, RecordPage, RoleBadge, Spinner, type DataColumn, type MenuItem, type RecordList } from '@hubtask/design-system/components';
  import type { InvitationDelivery, SignInDetail } from '@hubtask/sync-engine';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import RevokeDialog from '../lib/people/RevokeDialog.svelte';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { endsTheLastOwner, mayRevoke, ownershipOf, type Holder } from '../lib/data/people.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { deliveryLine } from '../lib/people/invitation.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const TENANT = { scopeType: 'TENANT' } as const;
  const LIST = '/administration/people';

  $effect(() =>
    untrack(() => {
      people.openScope(TENANT);
      return groups.open();
    }),
  );

  const all = $derived(people.holders({}, TENANT));
  const held = $derived(all.filter((holder) => holder.accountId === id));
  const isRead = $derived(people.isCompleteAt(TENANT));
  const name = $derived(accounts.nameOf(id) ?? t('app.people.unnamed'));
  const status = $derived(accounts.statusOf(id));
  const mayManage = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const mayViewAs = $derived(mayManage || holdsHere('READ_CONFIGURATION') === true);

  $effect(() => page.entitle(name));

  // The groups they are in: each group's members are read once, as the group's own page does.
  $effect(() => {
    const ids = groups.all.map((group) => group.id);
    untrack(() => ids.forEach((groupId) => void groups.read(groupId).catch(() => undefined)));
  });
  const memberOf = $derived(groups.all.filter((group) => groups.detail(group.id)?.members.includes(id)));

  // The address and the last mail, for whoever manages members - the same reads as the list's.
  let detail = $state<SignInDetail | undefined>(undefined);
  let delivery = $state<InvitationDelivery | undefined>(undefined);
  $effect(() => {
    if (!mayManage) return;
    untrack(() => {
      void people.signInDetails([id]).then((answer) => (detail = answer[0])).catch(() => (detail = undefined));
      void people.invitationDeliveries([id]).then((answer) => (delivery = answer[0])).catch(() => (delivery = undefined));
    });
  });

  const roleWord = (role: string) => t(`app.people.role.${role.toLowerCase()}`);
  const scopeLabel = $derived(t('app.people.at_workspace'));
  const standing = $derived(
    status === 'INVITED'
      ? { label: t('app.people.invited'), tone: 'info' as const }
      : status
        ? { label: t(`app.people.status_${status.toLowerCase()}`), tone: status === 'ACTIVE' ? ('success' as const) : ('neutral' as const) }
        : undefined,
  );

  function canAct(accountId: string): boolean {
    const each = accounts.statusOf(accountId);
    return each === 'ACTIVE' || each === 'RESTRICTED';
  }

  /** The role this page revokes: their own grant at the workspace, never one through a group. */
  const own = $derived(held.find((holder) => holder.isHere));
  const lastOwner = $derived(own ? endsTheLastOwner(all, own.membershipId, isRead, canAct) : false);
  const mayTakeAway = $derived(own !== undefined && mayRevoke(own.role, people.grantableAt(TENANT)));

  let revoking = $state<Holder | undefined>(undefined);
  let isWriting = $state(false);
  let failure = $state<string | undefined>(undefined);
  const ownership = $derived(revoking ? ownershipOf(all, revoking.membershipId, actor.account?.id, (group) => people.membersOf(group)) : 'other');

  async function revoke(): Promise<void> {
    const holder = revoking;
    revoking = undefined;
    if (!holder) return;
    failure = undefined;
    isWriting = true;
    try {
      await people.revoke(holder.membershipId, TENANT);
      announcer.say(t('app.people.revoked_announced'));
      onnavigate(LIST);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }

  const menuItems = $derived<MenuItem[]>([
    ...(mayViewAs ? [{ id: 'view_as', label: t('app.people.view_as'), icon: 'shield' as const }] : []),
    ...(mayTakeAway
      ? [{ id: 'revoke', label: t('app.people.revoke_menu'), isDestructive: true, hasSeparatorBefore: mayViewAs, disabledReason: lastOwner ? t('app.people.last_owner') : undefined }]
      : []),
  ]);

  function onMenu(item: string): void {
    if (item === 'view_as') onnavigate(`/administration/people?tab=permissions&person=${id}`);
    else if (item === 'revoke' && !lastOwner) revoking = own;
  }

  const lists = $derived<RecordList[]>([
    { id: 'roles', title: t('app.person.roles'), count: String(held.length) },
    { id: 'groups', title: t('app.person.groups'), count: String(memberOf.length) },
  ]);

  const roleColumns = $derived<DataColumn[]>([
    { id: 'role', label: t('app.people.role_column'), priority: 1 },
  ]);
  const groupColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.person.group_name'), priority: 1 },
    { id: 'description', label: t('app.person.group_description'), priority: 2 },
  ]);

  const facts = $derived(
    mayManage
      ? {
          title: t('app.person.facts'),
          rows: [
            { label: t('app.people.email_column'), value: detail?.email ?? t('app.people.no_address') },
            {
              label: t('app.person.last_mail'),
              value: delivery
                ? t(deliveryLine(delivery).code, { at: formatDateTime(delivery.at, messages.locale), kind: deliveryLine(delivery).kind ? t(deliveryLine(delivery).kind as string) : '' })
                : t('app.person.no_mail'),
            },
          ],
        }
      : undefined,
  );

  const factLine = $derived(
    t('app.person.figure', { roles: String(held.length), groups: String(memberOf.length) }),
  );
</script>

<RecordPage
  record={{
    kind: t('app.person.kind'),
    title: name,
    back: { label: t('app.people.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: standing,
    facts: factLine,
    menu: menuItems.length > 0 ? { label: t('app.people.more_for', { name }), items: menuItems, onselect: onMenu } : undefined,
  }}
  {lists}
  {facts}
  danger={mayTakeAway && own
    ? {
        title: t('app.page.danger_zone'),
        acts: [
          {
            id: 'revoke',
            title: t('app.person.revoke_title'),
            cost: t('app.person.revoke_cost', { role: roleWord(own.role) }),
            actionLabel: t('app.people.revoke_menu'),
            disabledReason: lastOwner ? t('app.people.last_owner') : undefined,
          },
        ],
        onact: () => (revoking = own),
      }
    : undefined}
  refused={isRead && held.length === 0 ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if item.id === 'roles'}
      <DataTable label={item.title} columns={roleColumns} rows={held} key={(holder: Holder) => holder.membershipId}>
        {#snippet cell(holder: Holder, column: DataColumn)}
          {#if column.id === 'role'}<RoleBadge label={roleWord(holder.role)} {scopeLabel} size="sm" />{/if}
        {/snippet}
        {#snippet empty()}
          <p class="quiet"><Spinner label={t('app.people.reading')} /> <span>{t('app.people.reading')}</span></p>
        {/snippet}
      </DataTable>
    {:else}
      <DataTable
        label={item.title}
        columns={groupColumns}
        rows={memberOf}
        key={(group) => group.id}
        href={(group) => `/administration/groups/${group.id}`}
      >
        {#snippet cell(group, column: DataColumn)}
          {#if column.id === 'name'}{group.name}{:else}<span class="quiet">{group.description ?? ''}</span>{/if}
        {/snippet}
        {#snippet empty()}
          <EmptyState kind="settled" title={t('app.person.no_groups')} />
        {/snippet}
      </DataTable>
    {/if}
  {/snippet}
</RecordPage>

{#if failure}
  <FailureNote problem={failure} />
{/if}

<RevokeDialog
  holder={revoking}
  {ownership}
  who={name}
  where={scopeLabel}
  isBusy={isWriting}
  onConfirm={() => void revoke()}
  onCancel={() => (revoking = undefined)}
/>

{#snippet notHere()}
  <EmptyState kind="settled" title={t('app.person.not_here')} />
{/snippet}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  p.quiet { padding: var(--sp-200); }
</style>
