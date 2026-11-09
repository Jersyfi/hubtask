<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One service account as its record page (page-system §7.2): it has lists of its own - the roles
  // it holds and its tokens - so it opens as a page, not a dialog (§5.1).
  //
  // **Its tokens are minted through the same form as a person's**, with `account_id` naming it —
  // the one exception the contract carves into "a token is its holder's", and the reason it needs
  // the permission that manages members. The form asks for one list to pick from (the scopes), so
  // it is a Dialog L; the token it answers is shown once in the same dialog.
  //
  // No *Edit* and no danger zone: the contract renames and deletes no service account. Its roles
  // are given and taken away under *People*, which is where the head's facts send the reader.

  import { untrack } from 'svelte';

  import { Button, DataTable, Dialog, EmptyState, OneTimeSecret, RecordPage, RoleBadge, type DataColumn, type RecordList } from '@hubtask/design-system/components';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { mayIssueWayIn, type Holder } from '../lib/data/people.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import type { MintedToken } from '../lib/data/tokens.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import DiscardDialog from '../lib/page/DiscardDialog.svelte';
  import { DiscardGuard } from '../lib/page/discard.svelte.ts';
  import MintTokenForm from '../lib/tokens/MintTokenForm.svelte';
  import TokenTable from '../lib/tokens/TokenTable.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  const { id, onnavigate }: { id: string; onnavigate: (path: string) => void } = $props();

  const LIST = '/administration/machines';
  const TENANT = { scopeType: 'TENANT' } as const;

  $effect(() =>
    untrack(() => {
      people.openScope(TENANT);
      return serviceAccounts.open();
    }),
  );

  const reading = $derived(serviceAccounts.state);
  const account = $derived(serviceAccounts.all.find((each) => each.id === id));
  const name = $derived(account?.display_name ?? t('app.service_accounts.kind'));
  $effect(() => page.entitle(name));

  const roles = $derived<readonly Holder[]>(people.holders({}, TENANT).filter((holder) => holder.accountId === id));

  // A service account's tokens are listed, minted and revoked under MANAGE_MEMBERS (security.md
  // §5); a reader without it reads the account and its roles only (P-05).
  const mayAdminister = $derived(holdsHere('MANAGE_MEMBERS') === true);
  /** A token for it is held to the role ceiling (identity.md §13): above the reader's own is refused. */
  const mintable = $derived(mayIssueWayIn(roles.map((holder) => holder.role), people.grantableAt(TENANT)));

  let isMinting = $state(false);
  let minted = $state<MintedToken | undefined>(undefined);
  let mintForm = $state<HTMLFormElement | null>(null);
  let isWorking = $state(false);
  let hasChanges = $state(false);

  // The credential dies with the screen, and with the dialog that showed it.
  $effect(() => () => (minted = undefined));

  const mintGuard = new DiscardGuard(() => hasChanges, () => (isMinting = false));

  const lists = $derived<RecordList[]>([
    { id: 'roles', title: t('app.service_accounts.roles_column'), count: String(roles.length) },
    ...(mayAdminister
      ? [
          {
            id: 'tokens',
            title: t('app.tokens.title'),
            add: mintable ? { label: t('app.tokens.mint_title'), icon: 'plus' as const, opener: 'new-token', onclick: () => (isMinting = true) } : undefined,
          },
        ]
      : []),
  ]);

  const roleColumns = $derived<DataColumn[]>([
    { id: 'role', label: t('app.people.role_column'), priority: 1 },
    { id: 'where', label: t('app.service_accounts.where_column'), priority: 2 },
  ]);
</script>

<RecordPage
  record={{
    kind: t('app.service_accounts.kind'),
    title: name,
    back: { label: t('app.service_accounts.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: account && people.isCompleteAt(TENANT) && roles.length === 0 ? { label: t('app.service_accounts.state_no_role'), tone: 'warning' } : undefined,
    facts: account ? t('app.service_accounts.facts', { roles: String(roles.length) }) : undefined,
  }}
  {lists}
  refused={reading.status === 'ready' && !account ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if item.id === 'roles'}
      <DataTable label={item.title} columns={roleColumns} rows={roles} key={(holder: Holder) => holder.membershipId}>
        {#snippet cell(holder: Holder, column: DataColumn)}
          {#if column.id === 'role'}
            <RoleBadge label={t(`app.people.role.${holder.role.toLowerCase()}`)} size="sm" />
          {:else}
            <span class="quiet">{t('app.people.at_workspace')}</span>
          {/if}
        {/snippet}
        {#snippet empty()}
          <!-- The confusion this page exists to head off: the token will authenticate and every
               operation will refuse it. -->
          <EmptyState kind="settled" title={t('app.service_accounts.no_roles')} />
        {/snippet}
      </DataTable>
    {:else}
      {#if !mintable}<p class="quiet">{t('app.tokens.mint_above_own')}</p>{/if}
      <TokenTable accountId={id} label={item.title} />
    {/if}
  {/snippet}
</RecordPage>
<!-- A reader without MANAGE_MEMBERS is told once why the tokens are not here (P-05). -->
<WorkspaceGate permission="MANAGE_MEMBERS" />

{#if mayAdminister && mintable}
  <Dialog
    size="l"
    title={t('app.tokens.mint_title')}
    bind:isOpen={isMinting}
    dismissLabel={minted ? undefined : t('app.workspace.cancel')}
    onDismiss={mintGuard.dismiss}
    onClose={() => (minted = undefined)}
    primary={minted ? undefined : { label: t('app.tokens.mint'), isBusy: isWorking, busyLabel: t('app.tokens.minting'), onclick: () => mintForm?.requestSubmit() }}
  >
    {#if minted}
      <!-- The only time this value exists anywhere but the server's hash. The acknowledgement is
           required, because a token scrolled past is a token nobody copied. -->
      <OneTimeSecret
        value={minted.token}
        label={t('app.tokens.minted_title')}
        hint={t('app.tokens.minted_hint')}
        revealLabel={t('app.tokens.reveal')}
        hideLabel={t('app.tokens.hide')}
        copyLabel={t('app.tokens.copy')}
        copiedLabel={t('app.tokens.copied')}
        acknowledgementLabel={t('app.tokens.kept')}
        notAcknowledgedReason={t('app.tokens.keep_first')}
        dismissLabel={t('app.tokens.done')}
        onDismiss={() => {
          minted = undefined;
          isMinting = false;
        }}
      />
    {:else}
      <MintTokenForm accountId={id} isInDialog bind:form={mintForm} bind:isWorking bind:hasChanges onminted={(next) => (minted = next)} />
    {/if}
    {#snippet actions()}
      {#if !minted}<Button tone="subtle" onclick={() => { if (mintGuard.dismiss()) isMinting = false; }}>{t('app.workspace.cancel')}</Button>{/if}
    {/snippet}
  </Dialog>
  <DiscardDialog guard={mintGuard} />
{/if}

{#snippet notHere()}
  <EmptyState kind="settled" title={t('app.service_accounts.not_here')} />
{/snippet}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
