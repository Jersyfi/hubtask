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

  import { Badge, Banner, Button, Input, RoleBadge, Select, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { MembershipRole } from '@hubtask/sync-engine';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

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

  const columns = [
    { id: 'who', label: t('app.people.who') },
    { id: 'role', label: t('app.people.role_column') },
    { id: 'status', label: t('app.people.status') },
    { id: 'actions', label: t('app.people.actions'), isLabelHidden: true },
  ];

  /** The workspace is the only scope here, so every badge says the same thing about reach. */
  const scopeLabel = t('app.people.at_workspace');

  function statusOf(accountId: string | undefined): string | undefined {
    return accountId ? accounts.statusOf(accountId) : undefined;
  }

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    isWriting = true;
    try {
      await work();
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
    });
    // Out of the form whatever happened. A second press with the same address in it would be a
    // second invitation the reader did not mean to send.
    inviteEmail = '';
    inviteName = '';
    inviteRole = '';
  }

  async function grant(): Promise<void> {
    if (!chosenSubject || !chosenRole) return;
    await attempt(() =>
      people.grant({ accountId: chosenSubject }, chosenRole as MembershipRole, TENANT),
    );
    chosenSubject = '';
    chosenRole = '';
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.people.title')}</h1>
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
              {#if holder.groupId}
                <!-- A membership granted to a group reaches the people in it. The group's own
                     screen is where that list lives; here it is named as what it is. -->
                {t('app.people.group_named', {
                  name: groups.nameOf(holder.groupId) ?? t('app.people.unnamed_group'),
                })}
              {:else}
                {accounts.nameOf(holder.accountId) ?? t('app.people.unnamed')}
              {/if}
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
              <Button
                size="sm"
                tone="subtle"
                isBusy={isWriting}
                busyLabel={t('app.people.working')}
                onclick={() => void attempt(() => people.revoke(holder.membershipId, TENANT))}
              >
                {t('app.people.revoke')}
              </Button>
            </td>
          </tr>
        {/each}
      </Table>
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.people.invite_title')}</h2>
      <p class="quiet small">{t('app.people.invite_hint')}</p>
      {#if invited}
        <Banner tone="success">{t('app.people.invited_sent', { email: invited })}</Banner>
      {/if}
      <form onsubmit={invite}>
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
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .who { color: var(--text-primary); font-weight: var(--fw-medium); }
</style>
