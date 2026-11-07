<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One role as its record page (page-system §5.1, §7.2): the permissions it carries, by area, and
  // who holds it. A shipped role does not change and is given on *People*; a role of the
  // workspace's own (UC-ID-26 check 6) is changed through its draft behind *Edit*, given behind
  // *Give the role* (a Dialog M) and deleted through the configuration's plan - its reach and
  // refusals shown before the button (user-experience.md §5.9) - in a Dialog S and the danger zone.

  import { untrack } from 'svelte';

  import {
    Button,
    DataTable,
    Dialog,
    EmptyState,
    RecordPage,
    Select,
    Spinner,
    Stack,
    type DataColumn,
    type DataRowMenu,
    type MenuItem,
    type RecordList,
  } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import RoleDialog from '../lib/people/RoleDialog.svelte';
  import { tabPath } from '../lib/people/peoplepage.ts';
  import { permissionLabel, roleLabel, SHIPPED_ROLES } from '../lib/people/rolewords.ts';
  import { DiscardGuard } from '../lib/page/discard.svelte.ts';
  import DiscardDialog from '../lib/page/DiscardDialog.svelte';
  import BlockRemoval from '../lib/settings/BlockRemoval.svelte';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { grantsOf, kindOf, roles } from '../lib/data/roles.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { check, isComplete, type Missing } from '../lib/forms/missing.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const TENANT = { scopeType: 'TENANT' } as const;
  const LIST = tabPath('roles');
  const WORKSPACE_PLACE = '';

  $effect(() =>
    untrack(() => {
      const closers = [roles.open(), groups.open(), people.openScope(TENANT), containers.start()];
      return () => closers.forEach((close) => close?.());
    }),
  );

  const catalogue = $derived(roles.catalogue);
  const custom = $derived(catalogue?.custom.find((role) => role.key === id));
  const isShipped = $derived((SHIPPED_ROLES as readonly string[]).includes(id) && catalogue?.shipped.some((role) => role.role === id) === true);
  const name = $derived(custom?.name ?? (isShipped ? roleLabel(id) : ''));
  const mayManage = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');

  $effect(() => page.entitle(name || t('app.people.tab_roles')));

  // What it carries, each permission with its area.
  interface Carried {
    readonly permission: string;
    readonly area: string;
  }
  const areaOf = (permission: string): string => {
    const entry = catalogue?.catalogue.find((each) => each.permission === permission);
    if (entry) return entry.group;
    const kind = kindOf(permission);
    return kind === 'FIELD' ? 'fields' : kind === 'STEP' ? 'workflows' : 'other';
  };
  function areaLabel(group: string): string {
    const known = ['content', 'how_we_work', 'people', 'evidence', 'portal', 'fields', 'workflows'];
    return known.includes(group) ? t(`app.roles.area.${group}`) : group;
  }
  const carried = $derived<Carried[]>(
    (custom ? custom.permissions : isShipped ? (catalogue?.catalogue ?? []).filter((entry) => entry.shipped.includes(id as never)).map((entry) => entry.permission) : [])
      .map((permission) => ({ permission, area: areaOf(permission) }))
      .sort((a, b) => a.area.localeCompare(b.area)),
  );

  // Who holds it: a shipped role through the workspace's memberships, a role of its own through
  // its grants, at the workspace or a hub.
  interface Holding {
    readonly id: string;
    readonly who: string;
    readonly where: string;
    readonly accountId?: string;
    readonly groupId?: string;
    readonly isGrant: boolean;
  }
  const placeName = (scopeType: string, scopeId?: string): string =>
    scopeType === 'TENANT' || !scopeId ? t('app.people.managed_where_workspace') : (containers.find(scopeId)?.name ?? t('app.roles.place_unknown'));
  const subjectName = (accountId?: string, groupId?: string): string =>
    groupId
      ? t('app.people.group_named', { name: groups.nameOf(groupId) ?? t('app.people.unnamed_group') })
      : (accounts.nameOf(accountId ?? '') ?? t('app.people.unnamed'));
  const holdings = $derived<Holding[]>(
    custom
      ? grantsOf(catalogue, id).map((grant) => ({
          id: grant.id,
          who: subjectName(grant.account_id, grant.group_id),
          where: placeName(grant.scope_type, grant.scope_id),
          accountId: grant.account_id,
          groupId: grant.group_id,
          isGrant: true,
        }))
      : people
          .holders({}, TENANT)
          .filter((holder) => holder.role === id && holder.isHere)
          .map((holder) => ({
            id: holder.membershipId,
            who: subjectName(holder.accountId, holder.groupId),
            where: t('app.people.managed_where_workspace'),
            accountId: holder.accountId,
            groupId: holder.groupId,
            isGrant: false,
          })),
  );

  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);

  async function attempt(work: () => Promise<unknown>, said: string): Promise<boolean> {
    failure = undefined;
    isWriting = true;
    try {
      await work();
      announcer.say(said);
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    } finally {
      isWriting = false;
    }
  }

  // Give the role: who and where, a Dialog M.
  let isGiving = $state(false);
  let subject = $state('');
  let place = $state(WORKSPACE_PLACE);
  let missing = $state<Missing>({});
  const subjects = $derived([
    ...people.candidates({}).map((accountId) => ({ value: `account:${accountId}`, label: accounts.nameOf(accountId) ?? t('app.people.unnamed') })),
    ...groups.all.map((group) => ({ value: `group:${group.id}`, label: t('app.people.group_named', { name: group.name }) })),
  ]);
  const places = $derived([
    { value: WORKSPACE_PLACE, label: t('app.people.managed_where_workspace') },
    ...containers.hubs.map((hub) => ({ value: hub.id, label: hub.name })),
  ]);
  function closeGiving(): void {
    isGiving = false;
    subject = '';
    place = WORKSPACE_PLACE;
    missing = {};
  }
  const giveGuard = new DiscardGuard(() => subject !== '' || place !== WORKSPACE_PLACE, closeGiving);

  async function give(): Promise<void> {
    missing = check([{ kind: 'field', key: 'subject', code: 'app.forms.choose', isFilled: subject !== '' }]);
    if (!isComplete(missing)) return;
    const [kind, subjectId] = subject.split(':');
    const done = await attempt(
      () =>
        roles.grant({
          role: id,
          scope_type: place === WORKSPACE_PLACE ? 'TENANT' : 'HUB',
          ...(place === WORKSPACE_PLACE ? {} : { scope_id: place }),
          ...(kind === 'group' ? { group_id: subjectId } : { account_id: subjectId }),
        }),
      t('app.roles.granted_announced'),
    );
    if (done) {
      toast.show({ text: t('app.roles.granted_toast', { name }), tone: 'success' });
      closeGiving();
    }
  }

  function holdingMenu(holding: Holding): DataRowMenu | undefined {
    const items: MenuItem[] = [
      ...(holding.accountId ? [{ id: 'open', label: t('app.groups.open_person') }] : []),
      ...(holding.groupId ? [{ id: 'group', label: t('app.roles.open_group') }] : []),
      ...(custom && holding.isGrant && mayManage ? [{ id: 'revoke', label: t('app.roles.revoke'), isDestructive: true, hasSeparatorBefore: true }] : []),
    ];
    return items.length > 0 ? { label: t('app.page.more_for', { name: holding.who }), items } : undefined;
  }

  function onHoldingMenu(holding: Holding, item: string): void {
    if (item === 'open' && holding.accountId) onnavigate(`/administration/people/${holding.accountId}`);
    else if (item === 'group' && holding.groupId) onnavigate(`/administration/groups/${holding.groupId}`);
    else if (item === 'revoke') void attempt(() => roles.revoke(holding.id), t('app.roles.revoked_announced'));
  }

  const lists = $derived<RecordList[]>([
    { id: 'permissions', title: t('app.roles.carries_title'), count: String(carried.length) },
    {
      id: 'holders',
      title: t('app.roles.holders_title'),
      count: String(holdings.length),
      add: custom && mayManage ? { label: t('app.roles.grant'), icon: 'plus' as const, opener: 'give-role', onclick: () => (isGiving = true) } : undefined,
    },
  ]);

  const carriedColumns = $derived<DataColumn[]>([
    { id: 'permission', label: t('app.access.permission'), priority: 1 },
    { id: 'area', label: t('app.roles.area_column'), priority: 2 },
  ]);
  const holdingColumns = $derived<DataColumn[]>([
    { id: 'who', label: t('app.people.who'), priority: 1 },
    { id: 'where', label: t('app.roles.where'), priority: 2 },
  ]);

  let editing = $state(false);
  let removing = $state(false);

  // Once deleted - the catalogue read again without it - the page leaves for the list.
  let wasHere = false;
  $effect(() => {
    if (custom) wasHere = true;
    else if (wasHere && catalogue) untrack(() => onnavigate(LIST));
  });
  const menuItems = $derived<MenuItem[]>(custom && mayManage ? [{ id: 'delete', label: t('app.roles.delete_menu'), isDestructive: true }] : []);
</script>

<RecordPage
  record={{
    kind: t('app.roles.kind'),
    title: name,
    back: { label: t('app.people.tab_roles'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: catalogue && (custom || isShipped) ? { label: t(custom ? 'app.roles.kind_own' : 'app.roles.kind_shipped'), tone: custom ? 'info' : 'neutral' } : undefined,
    facts: catalogue && (custom || isShipped) ? t('app.roles.figure', { permissions: String(carried.length), holders: String(holdings.length) }) : undefined,
    edit: custom && mayManage ? { label: t('app.page.edit'), opener: 'edit-role', onclick: () => (editing = true) } : undefined,
    menu: menuItems.length > 0 ? { label: t('app.page.more_for', { name }), items: menuItems, onselect: () => (removing = true) } : undefined,
  }}
  lists={catalogue && (custom || isShipped) ? lists : []}
  danger={custom && mayManage
    ? {
        title: t('app.page.danger_zone'),
        acts: [{ id: 'delete', title: t('app.roles.delete_title_zone'), cost: t('app.roles.delete_cost'), actionLabel: t('app.roles.delete_menu') }],
        onact: () => (removing = true),
      }
    : undefined}
  refused={!catalogue || (!custom && !isShipped) ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if item.id === 'permissions'}
      {#if isShipped}<p class="quiet small">{t('app.roles.shipped_hint')}</p>{/if}
      <DataTable label={item.title} columns={carriedColumns} rows={carried} key={(each: Carried) => each.permission} group={(each: Carried) => areaLabel(each.area)}>
        {#snippet cell(each: Carried, column: DataColumn)}
          {#if column.id === 'permission'}{permissionLabel(each.permission)}{:else}<span class="quiet">{areaLabel(each.area)}</span>{/if}
        {/snippet}
        {#snippet empty()}<EmptyState kind="settled" title={t('app.roles.carries_nothing')} />{/snippet}
      </DataTable>
    {:else}
      {#if failure}<FailureNote problem={failure} />{/if}
      <DataTable
        label={item.title}
        columns={holdingColumns}
        rows={holdings}
        key={(each: Holding) => each.id}
        rowLabel={(each: Holding) => each.who}
        menu={holdingMenu}
        onmenu={onHoldingMenu}
        actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        isBusy={isWriting}
      >
        {#snippet cell(each: Holding, column: DataColumn)}
          {#if column.id === 'who'}{each.who}{:else}<span class="quiet">{each.where}</span>{/if}
        {/snippet}
        {#snippet empty()}<EmptyState kind="settled" title={t('app.roles.held_by_nobody')} />{/snippet}
      </DataTable>
    {/if}
  {/snippet}
</RecordPage>

{#if custom && mayManage}
  <Dialog
    size="m"
    title={t('app.roles.give_title', { name })}
    bind:isOpen={isGiving}
    dismissLabel={t('app.roles.cancel')}
    onDismiss={giveGuard.dismiss}
    onClose={() => { subject = ''; place = WORKSPACE_PLACE; missing = {}; }}
    primary={{ label: t('app.roles.grant'), isBusy: isWriting, busyLabel: t('app.roles.working'), onclick: () => void give() }}
  >
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <Select label={t('app.people.who')} bind:value={subject} placeholder={t('app.people.choose_person')} options={subjects}
        error={missing.subject && !subject ? t(missing.subject) : undefined} />
      <Select label={t('app.roles.where')} bind:value={place} options={places} />
    </Stack>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => { if (giveGuard.dismiss()) closeGiving(); }}>{t('app.roles.cancel')}</Button>
    {/snippet}
  </Dialog>
  <DiscardDialog guard={giveGuard} />

  <RoleDialog role={editing ? custom : undefined} onClose={() => (editing = false)} />

  <!-- Deleting is planned first: the plan's reach and refusals stand before the button. -->
  <Dialog size="s" title={t('app.roles.delete_title', { name })} isOpen={removing} dismissLabel={t('app.roles.keep')} onClose={() => (removing = false)}>
    <Stack gap="150">
      <p class="quiet">{t('app.roles.delete_cost')}</p>
      {#if removing}
        <BlockRemoval section="roles" key={id} removed={t('app.roles.removed_announced')}
          confirm={t('app.roles.delete_now')} keep={t('app.roles.keep')}
          onDone={() => (removing = false)} />
      {/if}
    </Stack>
  </Dialog>
{/if}

{#snippet notHere()}
  {#if !catalogue}
    <p class="quiet waiting"><Spinner label={t('app.roles.reading')} /> <span>{t('app.roles.reading')}</span></p>
  {:else}
    <EmptyState kind="settled" title={t('app.roles.not_here')} />
  {/if}
{/snippet}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .waiting { padding: var(--sp-200); }
</style>
