<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Operators as a list page (page-system §7.1, §9.4): *Add an operator* the head's primary and
  // *Add a service account* beside it, each a Dialog M; removing in a row's ⋯, a Dialog S - asked
  // twice for oneself. An operator has no lists of its own, so a row opens no record page.
  // Who operates this installation (ADR-0070 §1, UC-INS-05).
  //
  // **Names, never identifiers.** Each row is a person or a machine as its workspace names it, and
  // the reader's own row says so; the register's function answers the names, and only those of
  // accounts it holds.
  //
  // **Adding cannot lock the adder out.** On an empty register the owner of the only workspace is
  // the operator, and the first entry would end that; the server enters the person adding as well,
  // and the screen says so before the button.
  //
  // **Removing asks, and removing oneself asks twice.** The second question is the one that says
  // what happens: the installation closes to you the moment you confirm. The last operator has no
  // *Remove* at all - the database refuses it, so the control would offer what cannot be done.
  import { untrack } from 'svelte';
  import { EmptyState, Banner, Button, DataTable, Dialog, FilterBar, Input, ListPage, Select, Spinner, Stack, type DataColumn } from '@hubtask/design-system/components';
  import type { Operator } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';
  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { isComplete, type Missing } from '../lib/forms/missing.ts';
  import { marked } from '../lib/forms/press.ts';

  /** What an *Add* found missing when pressed, marked at the field (UC-WRK-32 check 3). */
  let missing = $state<Missing>({});
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let workspace = $state('');
  let email = $state('');
  let machine = $state('');
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The row whose removal is being asked about, and how far the asking has got. */
  let removing = $state<{ operator: Operator; step: 'first' | 'again' } | undefined>(undefined);

  $effect(() => {
    if (!instance.isElevated) return;
    // The workspaces too: the form offers them by name rather than asking for a slug from memory,
    // and this screen is behind the same door that already read them. The service accounts are
    // the reader's own workspace's, read with the permission that manages its members.
    const stopOperators = untrack(() => instance.openOperators());
    const stopWorkspaces = untrack(() => instance.openWorkspaces());
    const stopMachines = untrack(() => serviceAccounts.open());
    return () => {
      stopOperators();
      stopWorkspaces();
      stopMachines();
    };
  });

  const reading = $derived(instance.operatorsState);
  const operators = $derived(instance.operators);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  const me = $derived(actor.account?.id);
  const isFirst = $derived(reading.status === 'ready' && operators.length === 0);
  const isLast = $derived(operators.length <= 1);

  const machines = $derived(serviceAccounts.state);
  /** The reader's workspace's service accounts that are not operators yet. */
  const candidates = $derived(
    serviceAccounts.all.filter((each) => !operators.some((operator) => operator.account_id === each.id)),
  );
  const machinesRefused = $derived(
    machines.status === 'failed' ? renderProblem(machines.error, messages) : undefined,
  );

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.instance.column_name'), priority: 1 },
    { id: 'address', label: t('app.instance.column_address'), priority: 2 },
    { id: 'workspace', label: t('app.instance.column_home'), priority: 3 },
    { id: 'added', label: t('app.instance.column_added'), priority: 3 },
  ]);

  let search = $state('');
  const shown = $derived(operators.filter((operator) => matchesSearch(search, [nameOf(operator), addressOf(operator), workspaceOf(operator)])));

  let isAdding = $state(false);
  let isAddingMachine = $state(false);
  let addForm = $state<HTMLFormElement | undefined>(undefined);
  let machineForm = $state<HTMLFormElement | undefined>(undefined);
  function openAdd(machineToo: boolean): void {
    failure = undefined;
    missing = {};
    if (machineToo) isAddingMachine = true;
    else isAdding = true;
  }

  function nameOf(operator: Operator): string {
    return operator.display_name ?? operator.account_id;
  }

  function addressOf(operator: Operator): string {
    if (operator.email) return operator.email;
    if (operator.sign_in_name) return operator.sign_in_name;
    return operator.kind === 'SERVICE_ACCOUNT' ? t('app.instance.operator_service_account') : '';
  }

  function workspaceOf(operator: Operator): string {
    if (operator.workspace_name && operator.workspace_slug) {
      return `${operator.workspace_name} (${operator.workspace_slug})`;
    }
    return operator.workspace_slug ?? operator.workspace_name ?? '';
  }

  async function run(call: () => Promise<void>): Promise<boolean> {
    isWorking = true;
    failure = undefined;
    try {
      await call();
      return true;
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
      return false;
    } finally {
      isWorking = false;
    }
  }

  async function confirmRemoval(): Promise<void> {
    const asked = removing;
    if (asked === undefined) return;
    const isSelf = asked.operator.account_id === me;
    if (isSelf && asked.step === 'first') {
      removing = { operator: asked.operator, step: 'again' };
      return;
    }
    removing = undefined;
    const removed = await run(() => instance.removeOperator(asked.operator.account_id));
    if (!removed) return;
    announcer.say(t('app.instance.operator_removed', { name: nameOf(asked.operator) }));
    if (isSelf) {
      // The installation is closed to the reader from this moment: the manifest says so, and the
      // area is left rather than drawn with reads that are refused.
      void manifest.refresh();
      onnavigate('/');
    }
  }

  $effect(() => page.entitle(t('app.instance.operators')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage
      head={{
        title: t('app.instance.operators'),
        breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.operators') }),
        primary: { label: t('app.instance.operator_add_title'), icon: 'plus', onclick: () => openAdd(false) },
        secondary: [{ label: t('app.instance.operator_service_title'), onclick: () => openAdd(true) }],
      }}
      refused={door ?? (unreadable ? failed : undefined)}
    >
      {#snippet intro()}<p class="quiet">{t('app.instance.operators_intro')}</p>{/snippet}
      {#snippet filters()}
        <FilterBar
          searchLabel={t('app.instance.operators_search')}
          clearSearchLabel={t('app.page.clear_search')}
          bind:search
          countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(operators.length) }) : undefined}
        />
      {/snippet}

      {#if failure && !isAdding && !isAddingMachine}<FailureNote problem={failure} />{/if}
      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else}
        <DataTable
          label={t('app.instance.operators')}
          {columns}
          rows={shown}
          key={(operator: Operator) => operator.account_id}
          rowLabel={nameOf}
          menu={(operator: Operator) => ({
            label: t('app.page.more_for', { name: nameOf(operator) }),
            // The last operator's Remove stays in its place, unavailable, and says why (UC-WRK-32
            // check 1) rather than under the table.
            items: [{ id: 'remove', label: `${t('app.instance.operator_remove')}…`, isDestructive: true, disabledReason: isLast ? t('app.instance.operator_last_note') : undefined }],
          })}
          onmenu={(operator: Operator) => (removing = { operator, step: 'first' })}
          actionsLabel={t('app.page.row_actions')}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          shownLabel={operators.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
        >
          {#snippet cell(operator: Operator, column: DataColumn)}
            {#if column.id === 'name'}
              <span class="name">{nameOf(operator)}</span>
              {#if operator.account_id === me}<span class="you">{t('app.instance.operator_you')}</span>{/if}
            {:else if column.id === 'address'}
              <span class="address">{addressOf(operator)}</span>
            {:else if column.id === 'workspace'}
              {workspaceOf(operator)}
            {:else}
              <span class="quiet-cell">
                {formatDateTime(operator.added_at, messages.locale)}
                {#if operator.added_by_name}<span class="by">{t('app.instance.operator_added_by', { name: operator.added_by_name })}</span>{/if}
              </span>
            {/if}
          {/snippet}
          {#snippet empty()}
            {#if operators.length === 0}
              <EmptyState kind="unused" title={t('app.instance.operators_empty')} />
            {:else}
              <EmptyState kind="filtered" title={t('app.instance.operators_none_found')}>
                {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
              </EmptyState>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<Dialog
  size="m"
  title={t('app.instance.operator_add_title')}
  isOpen={isAdding}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (isAdding = false)}
  primary={{ label: t('app.instance.operator_add'), isBusy: isWorking, busyLabel: t('app.instance.working'), onclick: () => addForm?.requestSubmit() }}
>
  <form
    bind:this={addForm}
    onsubmit={(event) => {
      event.preventDefault();
      const slug = workspace.trim();
      const address = email.trim();
      missing = marked(event, [
        { kind: 'field', key: 'workspace', code: 'app.forms.choose', isFilled: slug !== '' },
        { kind: 'field', key: 'email', code: 'app.forms.required', isFilled: address !== '' },
      ]);
      if (!isComplete(missing)) return;
      void run(async () => {
        await instance.addOperator(slug, address);
        workspace = '';
        email = '';
        isAdding = false;
      });
    }}
  >
    <Stack gap="150">
      {#if failure && isAdding}<FailureNote problem={failure} />{/if}
      {#if isFirst}<Banner tone="info">{t('app.instance.operator_first_note')}</Banner>{/if}
      <!-- The workspace's address and the address somebody signs in with: the two things an
           operator actually knows. -->
      <Select
        label={t('app.instance.operator_workspace_label')}
        hint={t('app.instance.operator_workspace_hint')}
        placeholder={t('app.instance.operator_workspace_placeholder')}
        bind:value={workspace}
        options={instance.workspaces.map((each) => ({ value: each.slug, label: `${each.display_name} (${each.slug})` }))}
        isRequired
        error={missing.workspace && !workspace.trim() ? t(missing.workspace) : undefined}
      />
      <Input
        label={t('app.instance.operator_email_label')}
        hint={t('app.instance.operator_email_hint')}
        bind:value={email}
        type="email"
        autocomplete="off"
        spellcheck={false}
        isRequired
        error={missing.email && !email.trim() ? t(missing.email) : undefined}
      />
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isAdding = false)}>{t('app.instance.cancel')}</Button>
  {/snippet}
</Dialog>

<!-- A service account has no address to type, so it is chosen from the list of the reader's own
     workspace (UC-INS-06 check 1) - never from another workspace's accounts. -->
<Dialog
  size="m"
  title={t('app.instance.operator_service_title')}
  isOpen={isAddingMachine}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (isAddingMachine = false)}
  primary={candidates.length > 0 ? { label: t('app.instance.operator_service_add'), isBusy: isWorking, busyLabel: t('app.instance.working'), onclick: () => machineForm?.requestSubmit() } : undefined}
>
  <form
    bind:this={machineForm}
    onsubmit={(event) => {
      event.preventDefault();
      missing = marked(event, [{ kind: 'field', key: 'machine', code: 'app.forms.choose', isFilled: machine !== '' }]);
      if (!isComplete(missing)) return;
      void run(async () => {
        await instance.addOperatorById(machine);
        machine = '';
        isAddingMachine = false;
      });
    }}
  >
    <Stack gap="150">
      <p class="prose">{t('app.instance.operator_service_intro')}</p>
      {#if failure && isAddingMachine}<FailureNote problem={failure} />{/if}
      {#if isFirst}<Banner tone="info">{t('app.instance.operator_first_note')}</Banner>{/if}
      {#if machines.status === 'loading' || machines.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else if machinesRefused}
        <p class="quiet">{machinesRefused.message}</p>
      {:else if candidates.length === 0}
        <EmptyState kind="unused" title={t('app.instance.operator_service_none')} />
      {:else}
        <Select
          label={t('app.instance.operator_service_label')}
          placeholder={t('app.instance.operator_service_placeholder')}
          bind:value={machine}
          options={candidates.map((each) => ({ value: each.id, label: each.display_name }))}
          isRequired
          error={missing.machine && !machine ? t(missing.machine) : undefined}
        />
      {/if}
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isAddingMachine = false)}>{t('app.instance.cancel')}</Button>
  {/snippet}
</Dialog>

<Dialog
  size="s"
  title={removing?.operator.account_id === me
    ? removing?.step === 'again'
      ? t('app.instance.operator_remove_self_again_title')
      : t('app.instance.operator_remove_self_title')
    : t('app.instance.operator_remove_title', { name: removing ? nameOf(removing.operator) : '' })}
  isOpen={removing !== undefined}
  dismissLabel={t('app.instance.operator_remove_cancel')}
  onClose={() => (removing = undefined)}
>
  {#snippet actions()}
    <Button onclick={() => (removing = undefined)}>{t('app.instance.operator_remove_cancel')}</Button>
    <Button tone="danger" onclick={() => void confirmRemoval()}>
      {removing?.operator.account_id === me && removing?.step === 'again'
        ? t('app.instance.operator_remove_self_again_confirm')
        : t('app.instance.operator_remove_confirm')}
    </Button>
  {/snippet}
  {#if removing}
    {#if removing.operator.account_id !== me}
      {t('app.instance.operator_remove_body', {
        name: nameOf(removing.operator),
        workspace: workspaceOf(removing.operator),
      })}
    {:else if removing.step === 'first'}
      {t('app.instance.operator_remove_self_body')}
    {:else}
      {t('app.instance.operator_remove_self_again_body')}
    {/if}
  {/if}
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-cell { color: var(--text-secondary); font-size: var(--fs-100); }
  .name { font-weight: var(--fw-medium); }
  .address { overflow-wrap: anywhere; }
  .by { display: block; }

  /* The reader's own row, beside the name: true of at most one row. */
  .you {
    margin-inline-start: var(--sp-100);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-full);
    background: var(--bg-surface-pressed);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-regular);
  }

  form { margin: 0; }
</style>
