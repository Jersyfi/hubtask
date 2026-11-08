<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's or a collection's *People* as a settings section (page-system §9.6, PS-4, §5.1): one
  // table of who holds which role here - granted here or applying from above -, a new invitation
  // the row's verb where the reader may renew one, *Revoke* behind its ⋯ (a Dialog S asks first);
  // *Give somebody a role* and *Invite somebody to this place* are Dialogs M. An entry's sharing and
  // the collection's own dialog keep `MembersDialog`, whose reasoning this follows: nothing is
  // predicted, the roles offered are the ones this scope's answer says the reader may give, a
  // grant from above is revoked where it was made, and `OWNER`'s step-up is asked by the server.

  import { untrack } from 'svelte';

  import { Avatar, Badge, Banner, Button, DataTable, Dialog, Input, OneTimeSecret, Select, Stack } from '@hubtask/design-system/components';
  import type { DataColumn } from '@hubtask/design-system/components';
  import type { MembershipRole } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { people, type Path, type Scope } from '../data/people.svelte.ts';
  import { grantableOf, mayIssueWayIn, mayRevoke, ownershipOf, type Holder } from '../data/people.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import TableSection from '../page/TableSection.svelte';
  import { renderProblem } from '../problem.ts';
  import { inviteWay } from './invitation.ts';
  import RevokeDialog from './RevokeDialog.svelte';

  const { scope, path }: { scope: Scope; path: Path } = $props();

  const holders = $derived(people.holders(path, scope));
  $effect(() => people.openScope(scope));

  /** The installation's roles, narrowed to the ones this scope's answer says the reader may give. */
  const roles = $derived((manifest.value?.roles ?? []).map((each) => each.role).filter((role): role is MembershipRole => role !== undefined));
  const grantable = $derived(grantableOf(roles, people.grantableAt(scope)));
  const roleOptions = $derived(grantable.map((role) => ({ value: role, label: t(`app.people.role.${role.toLowerCase()}`) })));

  /** Somebody who manages the workspace's members reads every hub's, and may invite (identity.md §12). */
  const managesWorkspace = $derived((people.grantableAt({ scopeType: 'TENANT' })?.length ?? 0) > 0);
  $effect(() => {
    if (!managesWorkspace) return;
    for (const hub of containers.hubs) untrack(() => people.openScope({ scopeType: 'HUB', scopeId: hub.id }));
  });

  /** The people on the path and everybody else this reader knows of (UC-WRK-21 check 4). */
  const candidates = $derived(
    [...new Set([...people.candidates(path), ...people.known()])]
      .map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') }))
      .sort((one, other) => one.label.localeCompare(other.label, messages.locale)),
  );

  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<boolean> {
    failure = undefined;
    isWriting = true;
    try {
      await work();
      if (said) announcer.say(said);
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    } finally {
      isWriting = false;
    }
  }

  const nameOf = (holder: Holder): string =>
    holder.accountId ? (accounts.nameOf(holder.accountId) ?? t('app.people.unnamed')) : t('app.people.a_group');

  const columns = $derived<DataColumn[]>([
    { id: 'who', label: t('app.people.who'), priority: 1 },
    { id: 'role', label: t('app.people.role_column'), priority: 2 },
    { id: 'where', label: t('app.people.invite_where'), priority: 3 },
  ]);

  // --- Revoking: behind the row's ⋯, asked first in a Dialog S. A grant from above says where it
  // is changed; one above the reader's own role is absent, as handing it out is (P-05). ---------
  const menuOf = (holder: Holder) =>
    !holder.isHere || mayRevoke(holder.role, people.grantableAt(scope))
      ? {
          label: t('app.page.more_for', { name: nameOf(holder) }),
          items: [{ id: 'revoke', label: t('app.people.revoke'), isDestructive: true, disabledReason: holder.isHere ? undefined : t('app.people.granted_elsewhere') }],
        }
      : undefined;
  let revoking = $state<Holder | undefined>(undefined);
  const revokingOwnership = $derived(revoking ? ownershipOf(holders, revoking.membershipId, actor.account?.id, (id) => people.membersOf(id)) : 'other');
  const where = $derived(t(`app.people.at_${scope.scopeType.toLowerCase()}`));

  function revoke() {
    const holder = revoking;
    revoking = undefined;
    if (holder) void attempt(() => people.revoke(holder.membershipId, scope), t('app.people.revoked_announced'));
  }

  // --- Renewing an invitation not yet accepted: the row's verb, where the reader manages the
  // workspace's members and may hand out the way in (identity.md §12, §13). -----------------------
  const way = $derived(inviteWay(manifest.value?.features));
  /** The invitation's link where no mail goes out, shown once and kept nowhere. */
  let handedOver = $state<{ readonly who: string; readonly link: string } | undefined>(undefined);
  /** Who an invitation went out to, said once above the table. */
  let invited = $state<string | undefined>(undefined);
  let resent = $state<string | undefined>(undefined);

  const mayRenew = (holder: Holder): boolean =>
    managesWorkspace &&
    holder.accountId !== undefined &&
    accounts.statusOf(holder.accountId) === 'INVITED' &&
    (way === 'mail' || mayIssueWayIn([holder.role], people.grantableAt({ scopeType: 'TENANT' })));

  function renew(holder: Holder): void {
    const accountId = holder.accountId;
    if (!accountId) return;
    invited = undefined;
    resent = undefined;
    handedOver = undefined;
    const who = nameOf(holder);
    void attempt(async () => {
      const link = await people.renewInvitation(accountId);
      if (link !== undefined) {
        handedOver = { who, link };
        announcer.say(t('app.people.renew_link_announced'));
      } else {
        resent = who;
        announcer.say(t('app.people.invited_announced'));
      }
    });
  }

  // --- Granting a role here: a Dialog M, who and which role. -------------------------------------
  let isGranting = $state(false);
  let chosenAccount = $state('');
  let chosenRole = $state('');
  const grantGuard = new DiscardGuard(() => chosenAccount !== '' || chosenRole !== '', () => (isGranting = false));

  function startGranting() {
    chosenAccount = '';
    chosenRole = '';
    failure = undefined;
    isGranting = true;
  }

  async function grant() {
    if (!chosenAccount || !chosenRole) return;
    if (await attempt(() => people.grant({ accountId: chosenAccount }, chosenRole as MembershipRole, scope), t('app.people.granted_announced'))) isGranting = false;
  }

  // --- Inviting somebody straight to this place, with a role here and nowhere else: a Dialog M
  // (UC-WRK-21 check 5). A link where no mail goes out is shown once in the same dialog. ----------
  let isInviting = $state(false);
  let inviteEmail = $state('');
  let inviteName = $state('');
  let inviteRole = $state('');
  const inviteGuard = new DiscardGuard(
    () => !handedOver && (inviteEmail.trim() !== '' || inviteName.trim() !== '' || inviteRole !== ''),
    () => (isInviting = false),
  );

  function startInviting() {
    inviteEmail = '';
    inviteName = '';
    inviteRole = '';
    invited = undefined;
    resent = undefined;
    handedOver = undefined;
    failure = undefined;
    isInviting = true;
  }

  async function invite(): Promise<void> {
    const address = inviteEmail.trim();
    if (!address || !inviteRole) return;
    let grantFailure: unknown;
    const done = await attempt(async () => {
      const answer = await people.invite(address, inviteName.trim() || undefined, inviteRole as MembershipRole, scope);
      grantFailure = answer.grantFailure;
      if (answer.link !== undefined) {
        handedOver = { who: address, link: answer.link };
        announcer.say(t('app.people.invite_link_announced'));
      } else {
        invited = address;
        announcer.say(t('app.people.invited_announced'));
      }
    });
    if (grantFailure !== undefined) failure = renderProblem(grantFailure as never, messages).message;
    if (done && !handedOver && grantFailure === undefined) isInviting = false;
  }
</script>

{#snippet acts()}
  {#if grantable.length > 0}
    {#if managesWorkspace}
      <Button size="sm" tone="subtle" onclick={startInviting}>{t('app.people.invite_here')}</Button>
    {/if}
    <Button size="sm" tone="secondary" onclick={startGranting}>{t('app.people.grant_title')}</Button>
  {/if}
{/snippet}

<TableSection title={t('app.people.title')} note={String(holders.length)} {acts}>
  {#if failure && !isGranting && !isInviting}<p class="failure" role="alert">{failure}</p>{/if}
  {#if invited}<Banner tone="success">{t('app.people.invited_here', { email: invited })}</Banner>{/if}
  {#if resent}<Banner tone="success">{t('app.people.renewed_here', { name: resent })}</Banner>{/if}
  {#if handedOver && !isInviting}
    <OneTimeSecret
      value={handedOver.link}
      label={t('app.people.invite_link', { name: handedOver.who })}
      hint={t('app.people.invite_link_hint')}
      revealLabel={t('app.people.invite_link_reveal')}
      hideLabel={t('app.people.invite_link_hide')}
      copyLabel={t('app.people.managed_copy')}
      copiedLabel={t('app.people.invite_link_copied')}
      acknowledgementLabel={t('app.people.invite_link_kept')}
      notAcknowledgedReason={t('app.people.invite_link_keep_first')}
      dismissLabel={t('app.tokens.done')}
      onDismiss={() => (handedOver = undefined)}
    />
  {/if}
  <div data-place-people>
    <DataTable
      label={t('app.people.holders_label')}
      {columns}
      rows={holders}
      key={(holder) => holder.membershipId}
      rowLabel={nameOf}
      menu={menuOf}
      onmenu={(holder) => (revoking = holder)}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
    >
      {#snippet cell(holder, column)}
        {#if column.id === 'who'}
          <span class="who">
            {#if holder.accountId}<Avatar name={nameOf(holder)} src={accounts.pictureOf(holder.accountId)} size="sm" />{/if}
            <!-- A group holds a role the way a person does; expanding it here would hide whom the grant was made to. -->
            <span class="name">{nameOf(holder)}</span>
          </span>
        {:else if column.id === 'role'}
          <Badge>{t(`app.people.role.${holder.role.toLowerCase()}`)}</Badge>
        {:else}
          <span class="quiet">{holder.isHere ? t('app.people.granted_here') : t('app.people.inherited')}</span>
        {/if}
      {/snippet}
      {#snippet verb(holder)}
        {#if mayRenew(holder)}
          <!-- There is no list of invitations, and a place's invitee is not on People. -->
          <Button size="sm" tone="secondary" isBusy={isWriting} busyLabel={t('app.people.working')} onclick={() => renew(holder)}>
            {way === 'link' ? t('app.people.renew_link') : t('app.people.renew_mail')}
          </Button>
        {/if}
      {/snippet}
      {#snippet empty()}<p class="quiet">{t('app.people.nobody_here')}</p>{/snippet}
    </DataTable>
  </div>
</TableSection>

<Dialog
  size="m"
  title={t('app.people.grant_title')}
  bind:isOpen={isGranting}
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={grantGuard.dismiss}
  primary={{ label: t('app.people.grant'), isBusy: isWriting, busyLabel: t('app.workspace.saving'), onclick: () => void grant() }}
>
  <Stack gap="150">
    <Select label={t('app.people.grant_who')} options={candidates} placeholder={t('app.people.grant_who')} bind:value={chosenAccount} />
    <Select label={t('app.people.grant_role')} options={roleOptions} placeholder={t('app.people.grant_role')} bind:value={chosenRole} />
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (grantGuard.dismiss()) isGranting = false; }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog guard={grantGuard} />

<Dialog
  size="m"
  title={t('app.people.invite_here')}
  bind:isOpen={isInviting}
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={inviteGuard.dismiss}
  primary={handedOver
    ? undefined
    : { label: way === 'link' ? t('app.people.invite_link_button') : t('app.people.invite'), isBusy: isWriting, busyLabel: t('app.people.working'), onclick: () => void invite() }}
>
  {#if handedOver}
    <OneTimeSecret
      value={handedOver.link}
      label={t('app.people.invite_link', { name: handedOver.who })}
      hint={t('app.people.invite_link_hint')}
      revealLabel={t('app.people.invite_link_reveal')}
      hideLabel={t('app.people.invite_link_hide')}
      copyLabel={t('app.people.managed_copy')}
      copiedLabel={t('app.people.invite_link_copied')}
      acknowledgementLabel={t('app.people.invite_link_kept')}
      notAcknowledgedReason={t('app.people.invite_link_keep_first')}
      dismissLabel={t('app.tokens.done')}
      onDismiss={() => {
        handedOver = undefined;
        isInviting = false;
      }}
    />
  {:else}
    <form onsubmit={(event) => { event.preventDefault(); void invite(); }}>
      <Stack gap="150">
        <Input label={t('app.people.invite_email')} bind:value={inviteEmail} type="email" autocomplete="off" spellcheck={false} isRequired />
        <Input label={t('app.people.invite_name')} hint={t('app.people.invite_name_hint')} bind:value={inviteName} autocomplete="off" />
        <Select label={t('app.people.role_column')} bind:value={inviteRole} placeholder={t('app.people.choose_role')} options={roleOptions} />
        {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      </Stack>
    </form>
  {/if}
  {#snippet actions()}
    {#if !handedOver}
      <Button tone="subtle" onclick={() => { if (inviteGuard.dismiss()) isInviting = false; }}>{t('app.workspace.cancel')}</Button>
    {/if}
  {/snippet}
</Dialog>
<DiscardDialog guard={inviteGuard} />

<RevokeDialog
  holder={revoking}
  ownership={revokingOwnership}
  who={revoking ? nameOf(revoking) : ''}
  {where}
  isBusy={isWriting}
  onConfirm={revoke}
  onCancel={() => (revoking = undefined)}
/>

<style>
  .who { display: inline-flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }
  .name { min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .failure { margin: 0; color: var(--text-danger); }
</style>
