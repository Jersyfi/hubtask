<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who holds a role at the workspace itself.
  //
  // **"The workspace's people" means "who holds a membership at it", and that is a limit of the
  // contract rather than a choice.** There is no `GET /accounts`: the contract lists no accounts
  // at all — only `/accounts/me`, `/accounts/{id}` and the invitation — so the only way to a list
  // of people is `GET /memberships?scope_type=TENANT`. An account with no membership anywhere is
  // invisible here, which is why the invitation below grants a role in the same breath.
  //
  // **This is the workspace half; a container has its own.** Same store, same grant and revoke,
  // one scope higher — and `people.svelte.ts` carries the step-up for both.
  //
  // **Read-only rather than absent where the write is refused.** Reading the memberships needs
  // `READ`; changing them needs `MANAGE_MEMBERS`. Somebody who holds the first and not the second
  // sees the people and no controls — a screen that hid itself would leave them unable to tell who
  // is in their own workspace.

  import { tick, untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    Input,
    OneTimeSecret,
    PageHeader,
    RoleBadge,
    Select,
    Spinner,
    Stack,
    Table,
  } from '@hubtask/design-system/components';
  import type { MembershipRole } from '@hubtask/sync-engine';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { grantableOf, ownershipOf, type Holder } from '../lib/data/people.ts';
  import RevokeDialog from '../lib/people/RevokeDialog.svelte';
  import ManagedAccountForm from '../lib/people/ManagedAccountForm.svelte';
  import AddressDialog from '../lib/people/AddressDialog.svelte';
  import RenewDialog from '../lib/people/RenewDialog.svelte';
  import StartPasswordResult from '../lib/people/StartPasswordResult.svelte';
  import { signInPolicy } from '../lib/data/signinpolicy.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { inviteWay } from '../lib/people/invitation.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { sorting } from '../lib/sorting.svelte.ts';

  /** The workspace itself. One scope, named once, so the read and the writes cannot disagree. */
  const TENANT = { scopeType: 'TENANT' } as const;
  /** The invitation's choice that means the whole workspace rather than a hub. */
  const WORKSPACE_PLACE = '';

  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);
  let chosenRole = $state('');
  let chosenSubject = $state('');
  let inviteEmail = $state('');
  let inviteName = $state('');
  let inviteRole = $state('');
  /**
   * Where the invited person is given their role: the whole workspace, or one hub (UC-WRK-21 check
   * 5, UC-ID-15) - a child or a guest limited to one hub from the start rather than given the
   * workspace first. A collection or a single entry is chosen from that place's own *People*.
   */
  let inviteWhere = $state(WORKSPACE_PLACE);
  let invited = $state<string | undefined>(undefined);
  /** Why the address was refused, said at the address field (UC-ID-14 check 6, WCAG 3.3.1). */
  let inviteRefusal = $state<string | undefined>(undefined);
  /** Why the role could not be given after the invitation went out, said at the form. */
  let grantRefusal = $state<string | undefined>(undefined);
  let inviteAddressField = $state<HTMLElement | undefined>(undefined);
  /**
   * The one answer that carried an invitation's link - an installation that sends no mail - until
   * the reader dismisses it. Held here only: the link is a credential, and nothing keeps it.
   */
  let handedOver = $state<{ readonly who: string; readonly link: string } | undefined>(undefined);
  let handedOverPanel = $state<HTMLElement | undefined>(undefined);

  /** Whether an invitation goes out by mail or as a link, as the manifest says before the press. */
  const way = $derived(inviteWay(manifest.value?.features));

  $effect(() =>
    untrack(() => {
      people.openScope(TENANT);
      // The groups' names, for the rows that name a group rather than a person. Any member may
      // read them, which is the contract's own reasoning: a grant to a group is unreadable until
      // the group can be shown as the people it reaches.
      return groups.open();
    }),
  );

  const holders = $derived(people.holders({}, TENANT));

  // The workspace's sign-in rule, for one switch: whether accounts without an address may be given
  // here at all (ADR-0074 §7). Read rather than assumed, and the form is absent - not disabled -
  // where the rule says no or cannot be read (P-05).
  $effect(() => untrack(() => signInPolicy.open()));
  const managedAllowed = $derived(signInPolicy.policy?.managed_accounts?.value === true);

  /** The row a revoke is being confirmed for; the dialog is open while there is one. */
  let revoking = $state<Holder | undefined>(undefined);
  const revokingOwnership = $derived(
    revoking ? ownershipOf(holders, revoking.membershipId, actor.account?.id, (id) => people.membersOf(id)) : 'other',
  );

  function nameOf(holder: Holder): string {
    if (holder.groupId) {
      return t('app.people.group_named', { name: groups.nameOf(holder.groupId) ?? t('app.people.unnamed_group') });
    }
    return accounts.nameOf(holder.accountId) ?? t('app.people.unnamed');
  }

  // A managed person's two actions (UC-ID-20 checks 5 and 7), on their row and nowhere else: the
  // summary's `managed` is the server's full condition, so a person with an address never sees them.
  /** The row a new start password is being confirmed for. */
  let renewing = $state<Holder | undefined>(undefined);
  /** The one answer that carried the new start password, until the reader dismisses it. */
  let renewed = $state<{ signInName: string; startPassword: string; who: string } | undefined>(undefined);
  /** The person an address is being added for. */
  let addressing = $state<{ accountId: string; who: string } | undefined>(undefined);

  function isManaged(holder: Holder): boolean {
    return holder.groupId === undefined && accounts.isManaged(holder.accountId);
  }

  // An address is added only where the password is a way in (identity.md §13): elsewhere it would
  // take a managed person's one way in. The sign-in rules say whether it is, fallback included; the
  // action is offered once they have answered, and its reason stands under the table otherwise.
  $effect(() => signInRules.read());
  const addressOffered = $derived(signInRules.isSettled && signInRules.hasPassword);
  const addressWithheld = $derived(
    signInRules.isSettled && !signInRules.hasPassword && holders.some((holder) => isManaged(holder)),
  );

  async function issueStartPassword(): Promise<void> {
    const holder = renewing;
    renewing = undefined;
    if (!holder?.accountId) return;
    const who = nameOf(holder);
    await attempt(async () => {
      const issued = await people.issueStartPassword(holder.accountId as string);
      renewed = { ...issued, who };
    }, t('app.people.renewed_announced', { who }));
  }

  async function revoke(): Promise<void> {
    const holder = revoking;
    revoking = undefined;
    if (!holder) return;
    await attempt(() => people.revoke(holder.membershipId, TENANT), t('app.people.revoked_announced'));
  }

  /** The installation's roles, in the order it reports them — never a list compiled in here. */
  const roles = $derived(
    (manifest.value?.roles ?? [])
      .map((description) => description.role)
      .filter((role): role is MembershipRole => role !== undefined),
  );

  /**
   * Of those, the ones the server says this reader may give at the workspace: an administrator is
   * not offered Owner (UC-ID-14 check 1). With none - or before the answer - the invitation and the
   * grant are absent, since every press would be refused (P-05).
   */
  const grantable = $derived(grantableOf(roles, people.grantableAt(TENANT)));

  const roleOptions = $derived(
    grantable.map((role) => ({ value: role, label: t(`app.people.role.${role.toLowerCase()}`) })),
  );

  /**
   * The invitation's place, and the roles offered there: what the reader may give is decided along
   * that place's path (domain-model.md §3.2), so a hub's answer is read when the hub is chosen and
   * nothing is offered until it arrives.
   */
  const invitePlace = $derived(inviteWhere === WORKSPACE_PLACE ? TENANT : { scopeType: 'HUB' as const, scopeId: inviteWhere });
  $effect(() => {
    const place = invitePlace;
    if (place.scopeType === 'HUB') untrack(() => people.openScope(place));
  });
  const inviteRoleOptions = $derived(
    grantableOf(roles, people.grantableAt(invitePlace)).map((role) => ({
      value: role,
      label: t(`app.people.role.${role.toLowerCase()}`),
    })),
  );
  // A role chosen for one place is not carried to a place that does not offer it.
  $effect(() => {
    if (inviteRole && !inviteRoleOptions.some((option) => option.value === inviteRole)) inviteRole = '';
  });
  $effect(() => untrack(() => containers.start()));
  const inviteWhereOptions = $derived([
    { value: WORKSPACE_PLACE, label: t('app.people.managed_where_workspace') },
    ...containers.hubs.map((hub) => ({ value: hub.id, label: hub.name })),
  ]);

  /** Who could be granted something: the people this workspace already knows about. */
  const candidates = $derived(
    people
      .candidates({})
      .map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  const columns = $derived([
    { id: 'who', label: t('app.people.who'), isSortable: true },
    { id: 'role', label: t('app.people.role_column'), isSortable: true },
    { id: 'status', label: t('app.people.status'), isSortable: true },
    { id: 'actions', label: t('app.people.actions'), isLabelHidden: true },
  ]);

  // The list on this screen is the one that actually grows - a workspace of two hundred people is
  // two hundred rows, and the server answers `GET /memberships` in one response with no cursor.
  // It is sorted here, over the list the client is already holding whole, and it is **not** paged:
  // a client-side pager would give the product a second answer to "where am I in this list"
  // beside `LoadMore`'s, and one of the two would always be the wrong guess.
  //
  // The role sorts by the **word** rather than by the enum's own order: a reader sorting this
  // column is looking for "every administrator together", and `ADMIN` before `MEMBER` in a
  // language where the words run the other way would be an order that looks arbitrary.
  const order = sorting({
    rows: () => holders,
    read: (holder, columnId) => {
      if (columnId === 'role') return t(`app.people.role.${holder.role.toLowerCase()}`);
      if (columnId === 'status') return statusOf(holder.accountId);
      return nameOf(holder);
    },
  });

  /**
   * The workspace is the only scope here, so every badge says the same thing about reach. Derived
   * rather than read once: the reader's catalogue arrives after the first render, and a label
   * fixed before it would stay in the source language beside translated sentences.
   */
  const scopeLabel = $derived(t('app.people.at_workspace'));

  function statusOf(accountId: string | undefined): string | undefined {
    return accountId ? accounts.statusOf(accountId) : undefined;
  }

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    isWriting = true;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see the screen is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (error) {
      // The server's own sentence — including the step-up refusal, which is not a failure but a
      // question the prompt has already asked and the reader declined.
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }

  /**
   * Invites somebody and gives them the role at once.
   *
   * The role is not optional here, and that is the contract's doing rather than a preference: an
   * invitation carries no role, and there is no accounts listing — so an invitation on its own
   * produces somebody this screen could never show again.
   */
  async function invite(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const address = inviteEmail.trim();
    if (!address || !inviteRole) return;
    invited = undefined;
    handedOver = undefined;
    inviteRefusal = undefined;
    grantRefusal = undefined;
    // The form's own, not the page's `attempt`: a refusal drawn at the top of the page is drawn
    // above the screen of whoever pressed the button at the form (UC-ID-14 check 6).
    isWriting = true;
    let answer: Awaited<ReturnType<typeof people.invite>>;
    try {
      answer = await people.invite(address, inviteName.trim() || undefined, inviteRole as MembershipRole, invitePlace);
    } catch (error) {
      // Nothing went out: the address stays to be corrected, the sentence is at its field, the
      // reader is taken there, and it is said once.
      inviteRefusal = renderProblem(error as never, messages).message;
      announcer.say(inviteRefusal);
      await tick();
      inviteAddressField?.querySelector('input')?.focus();
      return;
    } finally {
      isWriting = false;
    }
    if (answer.link !== undefined) {
      // UC-ID-14 check 5: no mail went out, and the screen does not pretend one did.
      await showLink(address, answer.link, t('app.people.invite_link_announced'));
    } else {
      invited = address;
      announcer.say(t('app.people.invited_announced'));
    }
    // The invitation stands even where the role could not be given; the reason is said at the form.
    if (answer.grantFailure !== undefined) grantRefusal = renderProblem(answer.grantFailure as never, messages).message;
    // An invitation went out, so the form empties: a second press with the same address in it
    // would be a second invitation the reader did not mean to send.
    inviteEmail = '';
    inviteName = '';
    inviteRole = '';
    inviteWhere = WORKSPACE_PLACE;
  }

  /**
   * Gives an invitation not yet accepted a new way to arrive: the mail again, or a new link shown
   * once. The earlier link stops working, which the panel's hint says.
   */
  async function renew(holder: Holder): Promise<void> {
    if (!holder.accountId) return;
    invited = undefined;
    handedOver = undefined;
    const accountId = holder.accountId;
    await attempt(async () => {
      const link = await people.renewInvitation(accountId);
      if (link !== undefined) {
        await showLink(nameOf(holder), link, t('app.people.renew_link_announced'));
      } else {
        invited = nameOf(holder);
        announcer.say(t('app.people.invited_announced'));
      }
    });
  }

  /** Shows the one answer's link, and takes the reader to it: it may be drawn far from the press. */
  async function showLink(who: string, link: string, said: string): Promise<void> {
    handedOver = { who, link };
    announcer.say(said);
    await tick();
    handedOverPanel?.focus();
  }

  async function grant(): Promise<void> {
    if (!chosenSubject || !chosenRole) return;
    await attempt(() =>
      people.grant({ accountId: chosenSubject }, chosenRole as MembershipRole, TENANT), t('app.people.granted_announced')
    );
    chosenSubject = '';
    chosenRole = '';
  }
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.people.title')));
</script>

<Stack gap="300">
  <!-- The section's page pattern (ADR-0063 decision 7): the trail says where in the
     administration this screen is and leads back to its index, the title is read rather than
     drawn where the bar carries it, and the content under the head keeps the reading measure.
     The head is **not** in that measure - `PageHeader` folds by the width it has, so a head
     inside a 60ch column folds like one on a phone and hides the trail behind the parent link.

       No primary action in the head, and that is the screen rather than the pattern: what one
       does here is give somebody a role, which is a form on the page. A screen whose one action
       opens a dialog puts its button here.

       No `onnavigate` either: the crumb carries an `href` and the router takes every same-origin
       link, so a handler here would be a second way to do what the link already does. -->
  <PageHeader
    title={t('app.people.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'people', label: t('app.people.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

    <Stack gap="300">
    <p class="quiet">{t('app.people.intro')}</p>

    {#if failure}
      <Banner tone="danger">{failure}</Banner>
    {/if}

    {#if renewed}
      <!-- The only time the new start password exists anywhere but the server's hash. -->
      <Stack gap="150">
        <StartPasswordResult
          signInName={renewed.signInName}
          startPassword={renewed.startPassword}
          label={t('app.people.renewed_start_password', { who: renewed.who, name: renewed.signInName })}
          onDismiss={() => (renewed = undefined)}
        />
      </Stack>
    {/if}

    {#if holders.length === 0}
      <p class="quiet"><Spinner label={t('app.people.reading')} /> <span>{t('app.people.reading')}</span></p>
    {:else}
      <Table
        label={t('app.people.title')}
        isLabelHidden
        {columns}
        sort={order.sort}
        onSort={(next) => order.by(next)}
      >
        {#each order.rows as holder (holder.membershipId)}
          <tr>
            <th scope="row" class="who">
              <!-- A membership granted to a group reaches the people in it. The group's own
                   screen is where that list lives; here it is named as what it is. -->
              {nameOf(holder)}
            </th>
            <td>
              <RoleBadge
                label={t(`app.people.role.${holder.role.toLowerCase()}`)}
                scopeLabel={scopeLabel}
                size="sm"
              />
            </td>
            <td>
              {#if statusOf(holder.accountId) === 'INVITED'}
                <!-- Not a failure and not a person yet: the invitation is out and unredeemed. -->
                <Badge tone="info">{t('app.people.invited')}</Badge>
              {:else if statusOf(holder.accountId)}
                <Badge>{t(`app.people.status_${statusOf(holder.accountId)?.toLowerCase()}`)}</Badge>
              {/if}
            </td>
            <td>
              <div class="actions">
                {#if statusOf(holder.accountId) === 'INVITED'}
                  <!-- An invitation not yet accepted can be given a new way to arrive: a link lost
                       or run out would otherwise leave the address taken for good. -->
                  <Button
                    size="sm"
                    isBusy={isWriting}
                    busyLabel={t('app.people.working')}
                    onclick={() => void renew(holder)}
                  >
                    {way === 'link' ? t('app.people.renew_link') : t('app.people.renew_mail')}
                  </Button>
                {/if}
                {#if isManaged(holder)}
                  <Button size="sm" tone="secondary" onclick={() => (renewing = holder)}>
                    {t('app.people.renew')}
                  </Button>
                  {#if addressOffered}
                    <Button
                      size="sm"
                      tone="secondary"
                      onclick={() => (addressing = { accountId: holder.accountId as string, who: nameOf(holder) })}
                    >
                      {t('app.people.address_add')}
                    </Button>
                  {/if}
                {/if}
                <!-- Asks first: a single keystroke on a focused control must not end
                     somebody's access. -->
                <Button
                  size="sm"
                  tone="danger"
                  isBusy={isWriting}
                  busyLabel={t('app.people.working')}
                  onclick={() => (revoking = holder)}
                >
                  {t('app.people.revoke')}
                </Button>
              </div>
            </td>
          </tr>
        {/each}
      </Table>

      {#if addressWithheld}
        <p class="quiet small">{t('app.people.address_needs_password')}</p>
      {/if}
    {/if}

    <RenewDialog
      who={renewing ? nameOf(renewing) : undefined}
      isBusy={isWriting}
      onConfirm={() => void issueStartPassword()}
      onCancel={() => (renewing = undefined)}
    />

    <AddressDialog target={addressing} onClose={() => (addressing = undefined)} />

    <RevokeDialog
      holder={revoking}
      ownership={revokingOwnership}
      who={revoking ? nameOf(revoking) : ''}
      where={scopeLabel}
      isBusy={isWriting}
      onConfirm={() => void revoke()}
      onCancel={() => (revoking = undefined)}
    />

    {#if grantable.length > 0 || invited || handedOver}
    <Stack gap="150">
      <h2 class="section">{t('app.people.invite_title')}</h2>
      <!-- What the press will do, said before it: a mail, or - where no mail goes out - a link to
           hand over. Neither until the manifest has said which. -->
      <p class="quiet small">
        {way === 'link' ? t('app.people.invite_hint_link') : way === 'mail' ? t('app.people.invite_hint') : t('app.people.invite_hint_neutral')}
      </p>
      {#if invited}
        <Banner tone="success">{t('app.people.invited_sent', { email: invited })}</Banner>
      {/if}
      {#if handedOver}
        <!-- The only time the link exists anywhere but as the server's hash. Focused when drawn: a
             renewal pressed in the table above draws it here. -->
        <div class="handed-over" tabindex="-1" bind:this={handedOverPanel} data-invitation-link>
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
        </div>
      {/if}
      {#if grantable.length > 0}
      <form class="panel" onsubmit={invite}>
        <Stack gap="150">
          <div bind:this={inviteAddressField}>
            <Input
              label={t('app.people.invite_email')}
              bind:value={inviteEmail}
              type="email"
              autocomplete="off"
              spellcheck={false}
              isRequired
              error={inviteRefusal}
              oninput={() => (inviteRefusal = undefined)}
            />
          </div>
          <Input
            label={t('app.people.invite_name')}
            hint={t('app.people.invite_name_hint')}
            bind:value={inviteName}
            autocomplete="off"
          />
          <Select
            label={t('app.people.invite_where')}
            hint={t('app.people.invite_where_hint')}
            bind:value={inviteWhere}
            options={inviteWhereOptions}
          />
          <Select
            label={t('app.people.role_column')}
            hint={t('app.people.invite_role_hint')}
            bind:value={inviteRole}
            placeholder={t('app.people.choose_role')}
            options={inviteRoleOptions}
          />
          {#if grantRefusal}
            <Banner tone="warning">{grantRefusal}</Banner>
          {/if}
          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWriting}
              busyLabel={t('app.people.working')}
            >
              {way === 'link' ? t('app.people.invite_link_button') : t('app.people.invite')}
            </Button>
          </div>
        </Stack>
      </form>
      {/if}
    </Stack>
    {/if}

    {#if managedAllowed && grantable.length > 0}
      <ManagedAccountForm {roles} workspaceRoles={grantable} />
    {/if}

    {#if grantable.length > 0}
    <Stack gap="150">
      <h2 class="section">{t('app.people.grant_title')}</h2>
      <p class="quiet small">{t('app.people.grant_hint')}</p>
      <Select
        label={t('app.people.who')}
        bind:value={chosenSubject}
        placeholder={t('app.people.choose_person')}
        options={candidates}
      />
      <Select
        label={t('app.people.role_column')}
        bind:value={chosenRole}
        placeholder={t('app.people.choose_role')}
        options={roleOptions}
      />
      <div>
        <Button
          tone="primary"
          isBusy={isWriting}
          busyLabel={t('app.people.working')}
          onclick={() => void grant()}
        >
          {t('app.people.grant')}
        </Button>
      </div>
    </Stack>
    {/if}
    </Stack>
</Stack>

<style>
  /* A form is neither prose nor a table, and it is the third case of ADR-0065 decision 2: an
     input as wide as the region is a target nobody aims at, so the fields carry a measure of
     their own while the lists and tables beside them take the width. */
  form { margin: 0; max-inline-size: 52ch; }

  /* The surface a form stands on, as the other screens of the section draw one: a standalone
     element in the sense of design-system.md rule 1, on the frame's canvas. */
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  /* The screen takes the region it is given, and what needs a measure carries one: prose has the
     one `app.css` gives every paragraph, fields have `.fields`, and a table or a list has none
     (ADR-0065 decision 2). A 60ch column around the whole of it would be a document's measure
     around a screen that is not a document. */

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .who { color: var(--text-primary); font-weight: var(--fw-medium); }

  /* A row still invited carries two actions; they wrap rather than widen the table on a phone. */
  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-100); justify-content: flex-end; white-space: nowrap; }

  /* Focused when drawn, so that a renewal pressed in the table reaches the reader here; the ring
     is the focus ring every control draws. */
  .handed-over { max-inline-size: 52ch; }
  .handed-over:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
