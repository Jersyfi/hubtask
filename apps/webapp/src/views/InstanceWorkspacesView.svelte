<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's workspaces and their whole lifecycle (SI-17, ADR-0070 §5).
  //
  // **Everything the control plane can do, this screen can do.** §5.5 lists it: "Liste, Zustand,
  // Anlegen, Sperren, Fortsetzen, Löschen, Export, Quoten: die Steuerungsebene, die es schon gibt,
  // zum Klicken." An operator who can only read here and has to reach for a terminal to act is an
  // operator for whom the dashboard is a status page — and the API is the product, with three doors
  // onto it rather than one door and two windows.
  //
  // **And never a row from inside a workspace.** The `tenant` row, its standing, its limits. The
  // tenant boundary is a database policy rather than a role, and this screen does not go around it:
  // what an operator needs to run an installation is counts, states and limits.
  //
  // **Every act that costs something says so where it is done.** The deletion asks for the display
  // name to be typed — the server compares it, and a deletion nobody typed the name of is a deletion
  // somebody clicked past. The owner's redemption token is shown once, through `OneTimeSecret`, and
  // dies with the panel.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Dialog, Input, OneTimeSecret, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { AdminTenant, ProvisionedTenant, TenantQuotas } from '@hubtask/sync-engine';
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

  /** Which act is open, and on which workspace. One at a time: two dialogs would be two answers. */
  let acting = $state<{ kind: 'new' | 'delete' | 'export' | 'quotas'; tenant?: AdminTenant } | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The owner's way in, from the one answer that carries it. Held by the screen, never the store. */
  let provisioned = $state<ProvisionedTenant | undefined>(undefined);

  // The new workspace's fields.
  let slug = $state('');
  let displayName = $state('');
  let ownerEmail = $state('');
  let ownerName = $state('');
  // What a deletion must be told, and what an export and the quotas need.
  let confirmation = $state('');
  let targetId = $state('');
  let quotas = $state<TenantQuotas>({});

  $effect(() => {
    const stop = untrack(() => instance.openWorkspaces());
    return stop;
  });

  const reading = $derived(instance.workspacesState);
  const workspaces = $derived(instance.workspaces);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  /** The columns, in the order the rows render their cells. */
  const columns = $derived([
    { id: 'workspace', label: t('app.instance.column_workspace') },
    { id: 'state', label: t('app.instance.column_state') },
    { id: 'since', label: t('app.instance.column_since') },
    { id: 'actions', label: t('app.instance.column_actions'), isLabelHidden: true },
  ]);

  /** The quota fields, by the name the contract gives each. Drawn from the list, not by hand. */
  const QUOTAS = [
    'api_requests_per_minute',
    'items',
    'media_bytes',
    'automation_runs_per_hour',
    'webhook_targets',
    'export_jobs',
    'ai_tokens_per_day',
  ] as const;

  function open(kind: 'new' | 'delete' | 'export' | 'quotas', tenant?: AdminTenant): void {
    acting = { kind, tenant };
    failure = undefined;
    confirmation = '';
    targetId = '';
    quotas = {};
    if (kind === 'new') {
      slug = '';
      displayName = '';
      ownerEmail = '';
      ownerName = '';
    }
  }

  async function run(id: string, call: () => Promise<void>): Promise<void> {
    working = id;
    failure = undefined;
    try {
      await call();
      acting = undefined;
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      working = undefined;
    }
  }

  function readQuota(name: string, raw: string): void {
    const trimmed = raw.trim();
    // An empty field is the installation's default, not zero: the contract's absent number and a
    // zero are different limits, and only one of them is "unset".
    quotas = { ...quotas, [name]: trimmed === '' ? undefined : Number(trimmed) };
  }

  $effect(() => page.entitle(t('app.instance.workspaces')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.workspaces')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.workspaces_intro')}</p>

      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      {#if provisioned}
        <!-- The owner's way in, once. It is not in the store and not in the listing: it exists in
             the answer that created the workspace and nowhere else. -->
        <OneTimeSecret
          value={provisioned.owner_redemption_token}
          label={t('app.instance.owner_token_title')}
          hint={t('app.instance.owner_token_hint')}
          revealLabel={t('app.instance.reveal')}
          hideLabel={t('app.instance.hide')}
          copyLabel={t('app.instance.copy')}
          copiedLabel={t('app.instance.copied')}
          acknowledgementLabel={t('app.instance.owner_token_kept')}
          notAcknowledgedReason={t('app.instance.owner_token_keep_first')}
          dismissLabel={t('app.instance.done')}
          onDismiss={() => (provisioned = undefined)}
        />
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
      {:else}
        <div>
          <Button tone="primary" onclick={() => open('new')}>{t('app.instance.workspace_new')}</Button>
        </div>

        {#if workspaces.length === 0}
          <Banner tone="info">{t('app.instance.workspaces_none')}</Banner>
        {:else}
          <Table label={t('app.instance.workspaces')} isLabelHidden columns={columns}>
            {#each workspaces as workspace (workspace.id)}
              <tr>
                <td>
                  <span class="name">{workspace.display_name}</span>
                  <span class="slug">{workspace.slug}</span>
                </td>
                <td>
                  <Badge
                    tone={workspace.status === 'ACTIVE'
                      ? 'success'
                      : workspace.status === 'SUSPENDED'
                        ? 'warning'
                        : 'danger'}
                  >
                    {t(`app.instance.state_${workspace.status.toLowerCase()}`)}
                  </Badge>
                  {#if workspace.purge_after}
                    <!-- The grace, where one stands: the number an operator needs before deciding
                         whether there is still time to resume. -->
                    <span class="slug">
                      {t('app.instance.purge_after', { at: formatDateTime(workspace.purge_after, messages.locale) })}
                    </span>
                  {/if}
                </td>
                <td class="slug">{formatDateTime(workspace.created_at, messages.locale)}</td>
                <td>
                  <div class="row">
                    {#if workspace.status === 'ACTIVE'}
                      <Button
                        tone="subtle"
                        isBusy={working === workspace.id}
                        busyLabel={t('app.instance.working')}
                        onclick={() => void run(workspace.id, () => instance.shift(workspace.id, 'suspend'))}
                      >
                        {t('app.instance.suspend')}
                      </Button>
                    {:else if workspace.status === 'SUSPENDED'}
                      <Button
                        tone="subtle"
                        isBusy={working === workspace.id}
                        busyLabel={t('app.instance.working')}
                        onclick={() => void run(workspace.id, () => instance.shift(workspace.id, 'resume'))}
                      >
                        {t('app.instance.resume')}
                      </Button>
                    {/if}
                    <Button tone="subtle" onclick={() => open('quotas', workspace)}>
                      {t('app.instance.quotas')}
                    </Button>
                    <Button tone="subtle" onclick={() => open('export', workspace)}>
                      {t('app.instance.export')}
                    </Button>
                    {#if workspace.status !== 'PENDING_DELETION'}
                      <Button tone="subtle" onclick={() => open('delete', workspace)}>
                        {t('app.instance.delete')}
                      </Button>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </Table>
        {/if}
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<Dialog
  title={t('app.instance.workspace_new')}
  isOpen={acting?.kind === 'new'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.cancel')}</Button>
    <Button
      tone="primary"
      isBusy={working === 'new'}
      busyLabel={t('app.instance.working')}
      onclick={() =>
        void run('new', async () => {
          provisioned = await instance.provision({
            slug: slug.trim(),
            display_name: displayName.trim(),
            owner_email: ownerEmail.trim(),
            ...(ownerName.trim() === '' ? {} : { owner_display_name: ownerName.trim() }),
          });
        })}
    >
      {t('app.instance.workspace_create')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <p class="prose">{t('app.instance.workspace_new_intro')}</p>
    <Input label={t('app.instance.slug_label')} hint={t('app.instance.slug_hint')} bind:value={slug} autocomplete="off" spellcheck={false} isRequired />
    <Input label={t('app.instance.name_label')} bind:value={displayName} autocomplete="off" isRequired />
    <Input label={t('app.instance.owner_email_label')} hint={t('app.instance.owner_email_hint')} bind:value={ownerEmail} type="email" autocomplete="off" isRequired />
    <Input label={t('app.instance.owner_name_label')} bind:value={ownerName} autocomplete="off" />
  </Stack>
</Dialog>

<Dialog
  title={t('app.instance.delete_title')}
  isOpen={acting?.kind === 'delete'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.keep')}</Button>
    <Button
      tone="danger"
      isBusy={working === 'delete'}
      busyLabel={t('app.instance.working')}
      onclick={() =>
        void run('delete', () =>
          instance.requestDeletion(acting?.tenant?.id ?? '', confirmation.trim()),
        )}
    >
      {t('app.instance.delete_now')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <!-- What it costs, before the button and not in a help page. -->
    <Banner tone="warning">{t('app.instance.delete_cost')}</Banner>
    <Input
      label={t('app.instance.delete_confirm_label', { name: acting?.tenant?.display_name ?? '' })}
      hint={t('app.instance.delete_confirm_hint')}
      bind:value={confirmation}
      autocomplete="off"
      spellcheck={false}
      isRequired
    />
  </Stack>
</Dialog>

<Dialog
  title={t('app.instance.export_title')}
  isOpen={acting?.kind === 'export'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.cancel')}</Button>
    <Button
      tone="primary"
      isBusy={working === 'export'}
      busyLabel={t('app.instance.working')}
      onclick={() =>
        void run('export', () =>
          instance.exportWorkspace(acting?.tenant?.id ?? '', targetId.trim()),
        )}
    >
      {t('app.instance.export_start')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <p class="prose">{t('app.instance.export_intro')}</p>
    <Input
      label={t('app.instance.export_target_label')}
      hint={t('app.instance.export_target_hint')}
      bind:value={targetId}
      autocomplete="off"
      spellcheck={false}
      isRequired
    />
  </Stack>
</Dialog>

<Dialog
  title={t('app.instance.quotas_title', { name: acting?.tenant?.display_name ?? '' })}
  isOpen={acting?.kind === 'quotas'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.cancel')}</Button>
    <Button
      tone="primary"
      isBusy={working === 'quotas'}
      busyLabel={t('app.instance.working')}
      onclick={() => void run('quotas', () => instance.setQuotas(acting?.tenant?.id ?? '', quotas))}
    >
      {t('app.instance.quotas_save')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <p class="prose">{t('app.instance.quotas_intro')}</p>
    {#each QUOTAS as name (name)}
      <Input
        label={t(`app.instance.quota.${name}`)}
        value={quotas[name] === undefined ? '' : String(quotas[name])}
        oninput={(event) => readQuota(name, (event.currentTarget as HTMLInputElement).value)}
        type="number"
        inputmode="numeric"
        autocomplete="off"
      />
    {/each}
  </Stack>
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
</style>
