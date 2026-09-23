<!-- SPDX-License-Identifier: BUSL-1.1
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
  // **F3-07 built the container half; this is the workspace half.** Same store, same grant and
  // revoke, one scope higher — and `people.svelte.ts` already carries the step-up, because F4-04
  // switched that control on rather than leaving a mechanism with no caller.
  //
  // **Read-only rather than absent where the write is refused.** Reading the memberships needs
  // `READ`; changing them needs `MANAGE_MEMBERS`. Somebody who holds the first and not the second
  // sees the people and no controls — a screen that hid itself would leave them unable to tell who
  // is in their own workspace.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Input, PageHeader, RoleBadge, Select, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { MembershipRole } from '@hubtask/sync-engine';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { ownershipOf, type Holder } from '../lib/data/people.ts';
  import RevokeDialog from '../lib/people/RevokeDialog.svelte';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  /** The workspace itself. One scope, named once, so the read and the writes cannot disagree. */
  const TENANT = { scopeType: 'TENANT' } as const;

  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);
  let chosenRole = $state('');
  let chosenSubject = $state('');
  let inviteEmail = $state('');
  let inviteName = $state('');
  let inviteRole = $state('');
  let invited = $state<string | undefined>(undefined);

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

  /** The row a revoke is being confirmed for (issue 778); the dialog is open while there is one. */
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

  const roleOptions = $derived(
    roles.map((role) => ({ value: role, label: t(`app.people.role.${role.toLowerCase()}`) })),
  );

  /** Who could be granted something: the people this workspace already knows about. */
  const candidates = $derived(
    people
      .candidates({})
      .map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  const columns = $derived([
    { id: 'who', label: t('app.people.who') },
    { id: 'role', label: t('app.people.role_column') },
    { id: 'status', label: t('app.people.status') },
    { id: 'actions', label: t('app.people.actions'), isLabelHidden: true },
  ]);

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
    await attempt(async () => {
      await people.invite(address, inviteName.trim() || undefined, inviteRole as MembershipRole, TENANT);
      invited = address;
    }, t('app.people.invited_announced'));
    // Out of the form whatever happened. A second press with the same address in it would be a
    // second invitation the reader did not mean to send.
    inviteEmail = '';
    inviteName = '';
    inviteRole = '';
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

    {#if holders.length === 0}
      <p class="quiet"><Spinner label={t('app.people.reading')} /> <span>{t('app.people.reading')}</span></p>
    {:else}
      <Table label={t('app.people.title')} isLabelHidden {columns}>
        {#each holders as holder (holder.membershipId)}
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
              <!-- Asks first (issue 778): a single keystroke on a focused control must not end
                   somebody's access. -->
              <Button
                size="sm"
                tone="subtle"
                isBusy={isWriting}
                busyLabel={t('app.people.working')}
                onclick={() => (revoking = holder)}
              >
                {t('app.people.revoke')}
              </Button>
            </td>
          </tr>
        {/each}
      </Table>
    {/if}

    <RevokeDialog
      holder={revoking}
      ownership={revokingOwnership}
      who={revoking ? nameOf(revoking) : ''}
      where={scopeLabel}
      isBusy={isWriting}
      onConfirm={() => void revoke()}
      onCancel={() => (revoking = undefined)}
    />

    <Stack gap="150">
      <h2 class="section">{t('app.people.invite_title')}</h2>
      <p class="quiet small">{t('app.people.invite_hint')}</p>
      {#if invited}
        <Banner tone="success">{t('app.people.invited_sent', { email: invited })}</Banner>
      {/if}
      <form class="panel" onsubmit={invite}>
        <Stack gap="150">
          <Input
            label={t('app.people.invite_email')}
            bind:value={inviteEmail}
            type="email"
            autocomplete="off"
            spellcheck={false}
            isRequired
          />
          <Input
            label={t('app.people.invite_name')}
            hint={t('app.people.invite_name_hint')}
            bind:value={inviteName}
            autocomplete="off"
          />
          <Select
            label={t('app.people.role_column')}
            hint={t('app.people.invite_role_hint')}
            bind:value={inviteRole}
            placeholder={t('app.people.choose_role')}
            options={roleOptions}
          />
          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWriting}
              busyLabel={t('app.people.working')}
            >
              {t('app.people.invite')}
            </Button>
          </div>
        </Stack>
      </form>
    </Stack>

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
     (ADR-0065 decision 2). The 60ch column that stood here was a document's measure around a
     screen that is not a document. */

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .who { color: var(--text-primary); font-weight: var(--fw-medium); }
</style>
