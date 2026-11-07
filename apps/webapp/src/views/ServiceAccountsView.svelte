<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The accounts that are nothing but access (`security.md` §5), as *Machines and apps*' first list
  // page (page-system §7.1). A row opens the account's record page, where its tokens are.
  //
  // **What a service account is for, in the catalogue's words**: the integration that must outlive
  // the person who wrote it. A rule that runs as somebody dies with their departure; a rule that
  // runs as a service account does not.
  //
  // **The memberships are shown, because a service account with none can do nothing** — and that
  // is a common confusion rather than a rare one. A token minted for an account that holds no role
  // is a credential that authenticates perfectly and is refused by every operation.
  //
  // **Adding is the head's primary**, a Dialog S: one field (page-system §5.1), with a refusal kept
  // inside it.

  import { untrack } from 'svelte';

  import {
    Badge,
    Button,
    DataTable,
    Dialog,
    EmptyState,
    FilterBar,
    Input,
    ListPage,
    Radio,
    RoleBadge,
    Spinner,
    Stack,
    type DataColumn,
    type Filter,
  } from '@hubtask/design-system/components';
  import type { Account } from '@hubtask/sync-engine';

  import { apps } from '../lib/data/apps.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { isComplete, type Missing } from '../lib/forms/missing.ts';
  import { marked } from '../lib/forms/press.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import MachinesFigure from '../lib/machines/MachinesFigure.svelte';
  import MachinesStrip from '../lib/machines/MachinesStrip.svelte';
  import { machinesHead } from '../lib/machines/machineshead.svelte.ts';
  import DiscardDialog from '../lib/page/DiscardDialog.svelte';
  import { DiscardGuard } from '../lib/page/discard.svelte.ts';
  import { matchesChoice, matchesSearch } from '../lib/page/list.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  const TENANT = { scopeType: 'TENANT' } as const;

  $effect(() =>
    untrack(() => {
      // The workspace's memberships say what each service account may actually reach; the apps
      // are counted in the head's figure.
      people.openScope(TENANT);
      const stopApps = apps.open();
      const stopAccounts = serviceAccounts.open();
      return () => {
        stopApps();
        stopAccounts();
      };
    }),
  );
  $effect(() => page.entitle(t('app.machines.title')));

  const reading = $derived(serviceAccounts.state);
  const all = $derived(serviceAccounts.all);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  // Adding needs MANAGE_MEMBERS (security.md §5); a reader without it, an auditor among them,
  // reads the accounts and is not offered a door every press of which is refused (P-05).
  const mayAdminister = $derived(holdsHere('MANAGE_MEMBERS') === true);

  /** What this account holds at the workspace. Empty is the case the screen exists to explain. */
  function rolesOf(accountId: string) {
    return people.holders({}, TENANT).filter((holder) => holder.accountId === accountId);
  }

  // The search and the role chip narrow the accounts loaded: `GET /auth/service-accounts` answers
  // them whole and filters nothing itself.
  let search = $state('');
  let standing = $state('');
  const shown = $derived(
    all.filter(
      (account) =>
        matchesSearch(search, [account.display_name]) &&
        matchesChoice(standing || undefined, rolesOf(account.id).length > 0 ? 'with' : 'without'),
    ),
  );
  const chips = $derived<Filter[]>([
    {
      id: 'role',
      label: t('app.service_accounts.role_chip', { choice: standing ? t(`app.service_accounts.role_${standing}`) : t('app.page.any') }),
      isActive: standing !== '',
    },
  ]);

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.service_accounts.name'), isSortable: true, priority: 1 },
    { id: 'roles', label: t('app.service_accounts.roles_column'), priority: 2 },
    { id: 'state', label: t('app.service_accounts.state_column'), isStatus: true },
  ]);
  const order = sorting({ rows: () => shown, read: (account: Account) => account.display_name });

  let isAdding = $state(false);
  let newName = $state('');
  /** What *Create* found missing when pressed (UC-WRK-32 check 3). */
  let missing = $state<Missing>({});
  let addFailure = $state<string | undefined>(undefined);
  let isWorking = $state(false);
  let addForm = $state<HTMLFormElement | undefined>(undefined);

  function reset(): void {
    newName = '';
    missing = {};
    addFailure = undefined;
  }

  const addGuard = new DiscardGuard(() => newName.trim() !== '', () => {
    isAdding = false;
    reset();
  });

  async function create(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const name = newName.trim();
    missing = marked(event, [{ kind: 'field', key: 'name', code: 'app.forms.required', isFilled: name !== '' }]);
    if (!isComplete(missing)) return;
    addFailure = undefined;
    isWorking = true;
    try {
      await serviceAccounts.create(name);
      announcer.say(t('app.service_accounts.created_announced'));
      toast.show({ text: t('app.service_accounts.created', { name }), tone: 'success' });
      isAdding = false;
      reset();
    } catch (cause) {
      addFailure = renderProblem(cause as never, messages).message;
    } finally {
      isWorking = false;
    }
  }

  const primary = $derived(
    mayAdminister ? { label: t('app.service_accounts.new_title'), icon: 'plus' as const, opener: 'add-service-account', onclick: () => (isAdding = true) } : undefined,
  );

  function clearFilters(): void {
    search = '';
    standing = '';
  }
</script>

{#snippet figure()}<MachinesFigure />{/snippet}

<ListPage head={machinesHead(primary, figure)}>
  {#snippet strip()}<MachinesStrip current="service-accounts" {onnavigate} />{/snippet}
  {#snippet filters()}
    <FilterBar
      searchLabel={t('app.service_accounts.search')}
      clearSearchLabel={t('app.page.clear_search')}
      bind:search
      filters={chips}
      moreLabel={t('app.page.more_filters')}
      filtersLabel={t('app.page.filters', { count: String(standing ? 1 : 0) })}
      sheetTitle={t('app.page.filters', { count: '0' })}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      showLabel={t('app.page.show_results', { count: String(shown.length) })}
      clearLabel={t('app.page.clear')}
      onclear={clearFilters}
      countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(all.length) }) : undefined}
    >
      {#snippet picker(_filter: Filter)}
        <Radio
          label={t('app.service_accounts.roles_column')}
          bind:value={standing}
          options={[
            { value: '', label: t('app.page.any') },
            { value: 'with', label: t('app.service_accounts.role_with') },
            { value: 'without', label: t('app.service_accounts.role_without') },
          ]}
        />
      {/snippet}
    </FilterBar>
  {/snippet}

  <p class="quiet">{t('app.service_accounts.intro')}</p>
  <!-- A reader without MANAGE_MEMBERS is told once why nothing here can be added (P-05). -->
  <WorkspaceGate permission="MANAGE_MEMBERS" />

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet waiting"><Spinner label={t('app.service_accounts.reading')} /> <span>{t('app.service_accounts.reading')}</span></p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else}
    <DataTable
      label={t('app.service_accounts.title')}
      {columns}
      rows={order.rows}
      key={(account: Account) => account.id}
      href={(account: Account) => `/administration/machines/${account.id}`}
      rowLabel={(account: Account) => account.display_name}
      sort={order.sort}
      onSort={(next) => order.by(next)}
      shownLabel={all.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(account: Account, column: DataColumn)}
        {#if column.id === 'name'}
          <span class="name">{account.display_name}</span>
        {:else if column.id === 'roles'}
          <span class="roles">
            {#each rolesOf(account.id) as holder (holder.membershipId)}
              <RoleBadge label={t(`app.people.role.${holder.role.toLowerCase()}`)} scopeLabel={t('app.people.at_workspace')} size="sm" />
            {/each}
          </span>
        {:else if rolesOf(account.id).length === 0}
          <!-- The confusion this screen exists to head off: its tokens will be refused by everything. -->
          <Badge tone="warning">{t('app.service_accounts.state_no_role')}</Badge>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if all.length === 0}
          <EmptyState kind="unused" icon="key" title={t('app.service_accounts.none')} description={t('app.service_accounts.empty_body')} action={mayAdminister ? addAct : undefined} />
        {:else}
          <EmptyState kind="filtered" title={t('app.service_accounts.filtered_title')}>
            {#snippet action()}<Button onclick={clearFilters}>{t('app.people.filtered_clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>
  {/if}
</ListPage>

{#snippet addAct()}
  <Button tone="primary" icon="plus" onclick={() => (isAdding = true)}>{t('app.service_accounts.new_title')}</Button>
{/snippet}

{#if mayAdminister}
  <Dialog
    size="s"
    bind:isOpen={isAdding}
    title={t('app.service_accounts.new_title')}
    dismissLabel={t('app.workspace.cancel')}
    onDismiss={addGuard.dismiss}
    onClose={() => (addFailure = undefined)}
    primary={{ label: t('app.service_accounts.create'), isBusy: isWorking, busyLabel: t('app.service_accounts.creating'), onclick: () => addForm?.requestSubmit() }}
  >
    <form bind:this={addForm} onsubmit={create}>
      <Stack gap="150">
        {#if addFailure}<FailureNote problem={addFailure} />{/if}
        <Input
          label={t('app.service_accounts.name')}
          hint={t('app.service_accounts.name_hint')}
          bind:value={newName}
          isRequired
          error={missing.name && !newName.trim() ? t(missing.name) : undefined}
        />
        <p class="quiet small">{t('app.service_accounts.then_grant')}</p>
      </Stack>
    </form>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => { if (addGuard.dismiss()) { isAdding = false; reset(); } }}>{t('app.workspace.cancel')}</Button>
    {/snippet}
  </Dialog>
  <DiscardDialog guard={addGuard} />
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .waiting { display: flex; align-items: center; gap: var(--sp-100); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .roles { display: inline-flex; flex-wrap: wrap; gap: var(--sp-050); }
  form { margin: 0; }
</style>
