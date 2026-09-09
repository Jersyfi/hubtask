<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who holds which role here, and granting or revoking one.
  //
  // **Both what was granted here and what applies from above.** "Who is on this hub" is a question
  // about effect, not about paperwork: a workspace owner is on every hub whether or not anybody
  // granted them anything on it, and a list that showed only the local grants would be a list that
  // is wrong about who can read the entries. A row granted higher up says so and cannot be revoked
  // from here — the grant is not here to revoke.
  //
  // **A subject appears once per membership.** `AUDITOR` is not a rung on the same ladder, and the
  // contract says two memberships add their rights up rather than the stronger one winning. A
  // single badge per person would be inventing an order the server does not have.
  //
  // **`OWNER` is on, and the step-up it needs is asked for when the server asks for it.** Granting
  // or revoking it is a privileged action needing a fresh re-authentication (`security.md` §5).
  // F3-07 shipped the control switched off because this client could not produce one; F4-04 built
  // the prompt, so the control is switched on here — one task earlier than F4-09 planned it,
  // because a mechanism with no caller is a mechanism nobody has proved.
  //
  // Nothing is predicted. The client does not decide which roles are privileged: it makes the
  // request, and `stepUp.around` turns the server's `auth.step_up_required` into a prompt and the
  // proof into a retry of the same intent. A client that carried its own list of privileged roles
  // would be wrong on the installation whose policy differs, and silently.
  //
  // The roles offered come from `/meta/capabilities`. A list written here would be wrong on the
  // installation that has one more.

  import {
    Avatar,
    Badge,
    Button,
    Dialog,
    Select,
    Stack,
  } from '@hubtask/design-system/components';
  import type { MembershipRole } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { people, type Path, type Scope } from '../data/people.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  let {
    isOpen = $bindable(false),
    scope,
    path,
    title,
  }: { isOpen?: boolean; scope: Scope; path: Path; title: string } = $props();

  const holders = $derived(people.holders(path, scope));

  /** The installation's roles, in the order it reports them. */
  const roles = $derived(
    (manifest.value?.roles ?? [])
      .map((description) => description.role)
      .filter((role): role is MembershipRole => role !== undefined),
  );

  let chosenAccount = $state('');
  let chosenRole = $state('');
  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);

  /** Who could be granted something here: the people the path already knows about. */
  const candidates = $derived(
    people.candidates(path).map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  const roleOptions = $derived(
    roles.map((role) => ({
      value: role,
      label: t(`app.people.role.${role.toLowerCase()}`),
    })),
  );

  $effect(() => {
    if (isOpen) people.openScope(scope);
  });

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    isWriting = true;
    try {
      await work();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
    isWriting = false;
  }

  function grant() {
    if (!chosenAccount || !chosenRole) return;
    void attempt(async () => {
      await people.grant({ accountId: chosenAccount }, chosenRole as MembershipRole, scope);
      chosenAccount = '';
      chosenRole = '';
    });
  }
</script>

<Dialog bind:isOpen {title} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="200">
    {#if holders.length === 0}
      <p class="empty">{t('app.people.nobody_here')}</p>
    {:else}
      <ul class="rows">
        {#each holders as holder (holder.membershipId)}
          <li class="row">
            {#if holder.accountId}
              <Avatar name={accounts.nameOf(holder.accountId) ?? t('app.people.unnamed')} size="sm" />
              <span class="who">{accounts.nameOf(holder.accountId) ?? t('app.people.unnamed')}</span>
            {:else}
              <!-- A group holds a role the same way a person does, and expanding it into its
                   members here would hide who the grant was actually made to. -->
              <span class="who">{t('app.people.a_group')}</span>
            {/if}

            <Badge>{t(`app.people.role.${holder.role.toLowerCase()}`)}</Badge>

            {#if !holder.isHere}
              <Badge tone="neutral">{t('app.people.inherited')}</Badge>
            {/if}

            <Button
              size="sm"
              tone="secondary"
              disabledReason={holder.isHere ? undefined : t('app.people.granted_elsewhere')}
              onclick={() => void attempt(() => people.revoke(holder.membershipId, scope))}
            >
              {t('app.people.revoke')}
            </Button>
          </li>
        {/each}
      </ul>
    {/if}

    <Stack gap="100">
      <h3 class="section">{t('app.people.grant_title')}</h3>
      <Select
        label={t('app.people.grant_who')}
        options={candidates}
        placeholder={t('app.people.grant_who')}
        bind:value={chosenAccount}
      />
      <Select
        label={t('app.people.grant_role')}
        options={roleOptions}
        placeholder={t('app.people.grant_role')}
        bind:value={chosenRole}
      />
      <div>
        <Button
          isBusy={isWriting}
          busyLabel={t('app.workspace.saving')}
          onclick={grant}
        >
          {t('app.people.grant')}
        </Button>
      </div>
    </Stack>

    {#if failure}<p class="failure">{failure}</p>{/if}
  </Stack>
</Dialog>

<style>
  .rows { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .who { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .section { margin: 0; font-size: var(--fs-100); }

  .empty { margin: 0; color: var(--text-subtle); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
