<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
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
  import { Banner, Button, Dialog, Input, PageHeader, Select, Spinner, Stack, Table } from '@hubtask/design-system/components';
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
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

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

  const columns = $derived([
    { id: 'name', label: t('app.instance.column_name') },
    { id: 'address', label: t('app.instance.column_address') },
    { id: 'workspace', label: t('app.instance.column_home') },
    { id: 'added', label: t('app.instance.column_added') },
    { id: 'actions', label: t('app.instance.column_actions'), isLabelHidden: true },
  ]);

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
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
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

<Stack gap="300">
  <PageHeader title={t('app.instance.operators')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.operators_intro')}</p>

      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else if operators.length === 0}
        <Banner tone="info">{t('app.instance.operators_empty')}</Banner>
      {:else}
        <Table label={t('app.instance.operators')} isLabelHidden columns={columns}>
          {#each operators as operator (operator.account_id)}
            <tr>
              <th scope="row" class="name">
                {nameOf(operator)}
                {#if operator.account_id === me}
                  <span class="you">{t('app.instance.operator_you')}</span>
                {/if}
              </th>
              <td class="address">{addressOf(operator)}</td>
              <td>{workspaceOf(operator)}</td>
              <td class="quiet-cell">
                {formatDateTime(operator.added_at, messages.locale)}
                {#if operator.added_by_name}
                  <span class="by">{t('app.instance.operator_added_by', { name: operator.added_by_name })}</span>
                {/if}
              </td>
              <td>
                {#if !isLast}
                  <Button
                    tone="subtle"
                    isBusy={isWorking}
                    busyLabel={t('app.instance.working')}
                    onclick={() => (removing = { operator, step: 'first' })}
                  >
                    {t('app.instance.operator_remove')}
                  </Button>
                {/if}
              </td>
            </tr>
          {/each}
        </Table>
        {#if isLast}
          <p class="quiet">{t('app.instance.operator_last_note')}</p>
        {/if}
      {/if}

      <form
        class="panel"
        onsubmit={(event) => {
          event.preventDefault();
          const slug = workspace.trim();
          const address = email.trim();
          if (slug === '' || address === '') return;
          void run(async () => {
            await instance.addOperator(slug, address);
            workspace = '';
            email = '';
          });
        }}
      >
        <Stack gap="150">
          <h2 class="section">{t('app.instance.operator_add_title')}</h2>
          {#if isFirst}
            <Banner tone="info">{t('app.instance.operator_first_note')}</Banner>
          {/if}
          <!-- The workspace's address and the address somebody signs in with: the two things an
               operator actually knows. The workspaces screen lists the first, and the second is
               what the person themselves would tell them. -->
          <Select
            label={t('app.instance.operator_workspace_label')}
            hint={t('app.instance.operator_workspace_hint')}
            placeholder={t('app.instance.operator_workspace_placeholder')}
            bind:value={workspace}
            options={instance.workspaces.map((each) => ({
              value: each.slug,
              label: `${each.display_name} (${each.slug})`,
            }))}
            isRequired
          />
          <Input
            label={t('app.instance.operator_email_label')}
            hint={t('app.instance.operator_email_hint')}
            bind:value={email}
            type="email"
            autocomplete="off"
            spellcheck={false}
            isRequired
          />
          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.instance.working')}
            >
              {t('app.instance.operator_add')}
            </Button>
          </div>
        </Stack>
      </form>

      <!-- A service account has no address to type, so it is chosen from the list of the reader's
           own workspace (UC-INS-06 check 1) - never from another workspace's accounts. -->
      <form
        class="panel"
        onsubmit={(event) => {
          event.preventDefault();
          if (machine === '') return;
          void run(async () => {
            await instance.addOperatorById(machine);
            machine = '';
          });
        }}
      >
        <Stack gap="150">
          <h2 class="section">{t('app.instance.operator_service_title')}</h2>
          <p class="prose">{t('app.instance.operator_service_intro')}</p>
          {#if isFirst}
            <Banner tone="info">{t('app.instance.operator_first_note')}</Banner>
          {/if}
          {#if machines.status === 'loading' || machines.status === 'idle'}
            <p class="quiet">
              <Spinner label={t('app.instance.reading')} />
              <span>{t('app.instance.reading')}</span>
            </p>
          {:else if machinesRefused}
            <p class="quiet">{machinesRefused.message}</p>
          {:else if candidates.length === 0}
            <p class="quiet">{t('app.instance.operator_service_none')}</p>
          {:else}
            <Select
              label={t('app.instance.operator_service_label')}
              placeholder={t('app.instance.operator_service_placeholder')}
              bind:value={machine}
              options={candidates.map((each) => ({ value: each.id, label: each.display_name }))}
              isRequired
            />
            <div>
              <Button
                type="submit"
                tone="primary"
                isBusy={isWorking}
                busyLabel={t('app.instance.working')}
              >
                {t('app.instance.operator_service_add')}
              </Button>
            </div>
          {/if}
        </Stack>
      </form>
    </Stack>
  </InstanceGate>
</Stack>

<Dialog
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
  .name { font-weight: var(--fw-medium); text-align: start; }
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

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  form { margin: 0; max-inline-size: 52ch; }
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
</style>
