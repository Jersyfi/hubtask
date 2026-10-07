<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who operates this installation (ADR-0070 §1).
  //
  // **An empty register is the private installation.** Nothing configured, and the owner is the
  // operator exactly as they were before the table existed — which is the honest reading of "this
  // installation never said who runs it", and what the screen says rather than showing an empty
  // list as if something were missing.
  //
  // **The last operator cannot remove themselves.** The database refuses it in the statement rather
  // than in a read-then-write, so two operators removing each other at the same moment cannot both
  // win; this screen shows the refusal rather than predicting it.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, PageHeader, Select, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
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
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  $effect(() => {
    // The workspaces too: the form offers them by name rather than asking for a slug from memory,
    // and this screen is behind the same door that already read them.
    const stopOperators = untrack(() => instance.openOperators());
    const stopWorkspaces = untrack(() => instance.openWorkspaces());
    return () => {
      stopOperators();
      stopWorkspaces();
    };
  });

  const reading = $derived(instance.operatorsState);
  const operators = $derived(instance.operators);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  const columns = $derived([
    { id: 'account', label: t('app.instance.column_account') },
    { id: 'workspace', label: t('app.instance.column_home') },
    { id: 'added', label: t('app.instance.column_added') },
    { id: 'actions', label: t('app.instance.column_actions'), isLabelHidden: true },
  ]);

  async function run(call: () => Promise<void>): Promise<void> {
    isWorking = true;
    failure = undefined;
    try {
      await call();
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
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
              <td class="mono">{operator.account_id}</td>
              <td class="mono">{operator.tenant_id}</td>
              <td class="quiet-cell">{formatDateTime(operator.added_at, messages.locale)}</td>
              <td>
                <Button
                  tone="subtle"
                  isBusy={isWorking}
                  busyLabel={t('app.instance.working')}
                  onclick={() => void run(() => instance.removeOperator(operator.account_id))}
                >
                  {t('app.instance.operator_remove')}
                </Button>
              </td>
            </tr>
          {/each}
        </Table>
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
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-cell { color: var(--text-secondary); font-size: var(--fs-100); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  form { margin: 0; max-inline-size: 52ch; }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
</style>
