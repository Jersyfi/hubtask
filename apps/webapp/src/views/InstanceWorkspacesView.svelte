<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's workspaces and their whole lifecycle (ADR-0070 §5).
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
  //
  // **Opening the password for one workspace** (ADR-0078 §3) is the lever for a provider that is
  // switched on but broken: a day unless said otherwise, a week at most, with who asked and why. The
  // row says while one stands, and closes it with one press.
  //
  // **The operator's own workspace is marked**, because suspending or deleting it ends the
  // operator's own session at once; both dialogs say so there. **In single mode** the one workspace
  // is the installation: no *New workspace*, *Suspend* or *Delete*, which the server refuses there.
  // *Cancel deletion* stands on a row pending deletion until the removal has begun.

  import { tick, untrack } from 'svelte';

  import { Badge, Banner, Button, Dialog, IconButton, Input, Menu, OneTimeSecret, PageHeader, Select, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { MenuItem } from '@hubtask/design-system/components';
  import type { AdminTenant, ProvisionedTenant, TenantQuotas } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { OWN_TARGET, exportTargetChoices } from '../lib/backup/targets.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { workspace as own } from '../lib/data/workspace.svelte.ts';
  import { plans } from '../lib/data/plans.svelte.ts';
  import MovePlanDialog from '../lib/instance/MovePlanDialog.svelte';
  import ChangeAddressDialog from '../lib/address/ChangeAddressDialog.svelte';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { focusFirst } from '../lib/forms/missing.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  /** The opening's bounds, as the contract states them: a day unless said otherwise, a week at most. */
  const OPENING_DEFAULT_HOURS = 24;
  const OPENING_MAXIMUM_HOURS = 168;

  /** Which act is open, and on which workspace. One at a time: two dialogs would be two answers. */
  let acting = $state<
    | { kind: 'new' | 'suspend' | 'delete' | 'export' | 'quotas' | 'open-password'; tenant?: AdminTenant }
    | undefined
  >(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The owner's way in, from the one answer that carries it. Held by the screen, never the store. */
  let provisioned = $state<ProvisionedTenant | undefined>(undefined);

  // The new workspace's fields.
  let slug = $state('');
  /** The host a new workspace answers at, where the installation assigns hosts (ADR-0095 §6). */
  let host = $state('');
  let displayName = $state('');
  let ownerEmail = $state('');
  /** The plan a new workspace starts on; empty for none. */
  let newPlan = $state('');
  /** Whether the plan field was preset yet: the plans may arrive after the dialog opened. */
  let isPlanPreset = $state(false);
  let ownerName = $state('');
  // What a deletion must be told, and what an export and the quotas need.
  let confirmation = $state('');
  let targetId = $state('');
  /** An installation target's identifier, or OWN_TARGET for one of the workspace's own, typed. */
  let targetChoice = $state(OWN_TARGET);
  let quotas = $state<TenantQuotas>({});
  // What an opening of the password is told: how long, who asked and why.
  let hours = $state(String(OPENING_DEFAULT_HOURS));
  let requester = $state('');
  let reason = $state('');

  $effect(() => {
    if (!instance.isElevated) return;
    // The instance level too, so the limits dialog can say what an empty field actually means:
    // "the installation's default" is not a helpful sentence without the number.
    const stops = untrack(() => [instance.openWorkspaces(), instance.openSettings(), instance.openTargets()]);
    return () => stops.forEach((stop) => stop());
  });
  // A New workspace opened before the plans were read is preset once they are, as it would have been.
  $effect(() => {
    if (acting?.kind !== 'new' || isPlanPreset || plans.state.status !== 'ready') return;
    isPlanPreset = true;
    newPlan = untrack(() => (plans.all.length === 1 ? (plans.all[0]?.name ?? '') : ''));
  });
  // The plans, where there are any to put a workspace on (multi-tenancy.md §4.2).
  $effect(() => {
    if (!instance.isElevated) return;
    if (!manifest.isMultiTenant) return;
    return untrack(() => plans.open());
  });
  /** The workspace whose plan is being changed, while the dialog is open. */
  let moving = $state<AdminTenant | undefined>(undefined);
  /** The workspace whose address is being changed (multi-tenancy.md §3.1). */
  let changing = $state<AdminTenant | undefined>(undefined);
  const hasPlans = $derived(manifest.isMultiTenant && plans.all.length > 0);
  const planName = (name: string | null | undefined): string =>
    name ? (plans.named(name)?.display_name ?? name) : t('app.instance.plan_none');

  /** The installation's own ceiling for a quota, where it set one. */
  function instanceDefault(name: string): string | undefined {
    const settings = instance.settings.status === 'ready' ? instance.settings.data : undefined;
    const entry = settings?.quotas?.[name];
    if (!entry?.set) return undefined;
    return String(entry.value);
  }

  /**
   * What an empty exception falls back to for this workspace: its plan's value where the plan sets
   * one the installation does not lock, the installation's own otherwise. The same reading as the
   * server's `quota.Levels.planApplies` (multi-tenancy.md §4.2) - kept beside it, since a
   * placeholder that names another number than the one in force would be the dialog lying.
   */
  function fallbackOf(name: string, tenant: AdminTenant | undefined): { value: string; plan?: string } | undefined {
    const settings = instance.settings.status === 'ready' ? instance.settings.data : undefined;
    const plan = tenant?.plan ? plans.named(tenant.plan) : undefined;
    const planned = plan?.quotas?.[name];
    if (planned?.set && planned.value !== undefined && planned.value !== null && settings?.quotas?.[name]?.locked !== true) {
      return { value: String(planned.value), plan: plan?.display_name };
    }
    const own = instanceDefault(name);
    return own === undefined ? undefined : { value: own };
  }

  // The operator's own workspace, to mark its row: the one whose suspension or deletion ends the
  // session this screen runs in.
  $effect(() => untrack(() => own.open()));
  const ownId = $derived(own.workspace?.id);
  const isOwn = $derived(acting?.tenant !== undefined && acting.tenant.id === ownId);
  const isMulti = $derived(manifest.isMultiTenant);

  const reading = $derived(instance.workspacesState);
  const workspaces = $derived(instance.workspaces);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  /** The columns, in the order the rows render their cells. */
  const columns = $derived([
    { id: 'workspace', label: t('app.instance.column_workspace') },
    { id: 'state', label: t('app.instance.column_state') },
    ...(hasPlans ? [{ id: 'plan', label: t('app.instance.column_plan') }] : []),
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
    'ai_arrival_proposals_per_day',
    'accounts',
  ] as const;

  function open(
    kind: 'new' | 'suspend' | 'delete' | 'export' | 'quotas' | 'open-password',
    tenant?: AdminTenant,
  ): void {
    acting = { kind, tenant };
    failure = undefined;
    confirmation = '';
    targetId = '';
    targetChoice = instance.targets.find((each) => each.enabled !== false)?.id ?? OWN_TARGET;
    quotas = {};
    hours = String(OPENING_DEFAULT_HOURS);
    requester = '';
    reason = '';
    if (kind === 'new') {
      slug = '';
      host = '';
      displayName = '';
      ownerEmail = '';
      ownerName = '';
      // One plan is the installation's plan; among several, none is the choice nobody made.
      newPlan = plans.all.length === 1 ? (plans.all[0]?.name ?? '') : '';
      isPlanPreset = plans.state.status === 'ready';
    }
  }

  /**
   * What may be done to a workspace, in its state: the way back while its removal has not begun,
   * resuming a suspended one, moving and limits, the export, the password opening, and deletion last
   * and marked - absent where the server would refuse it (P-05).
   */
  function actionsOf(workspace: AdminTenant): MenuItem[] {
    const items: MenuItem[] = [];
    if (workspace.status === 'ACTIVE' && isMulti) items.push({ id: 'suspend', label: t('app.instance.suspend') });
    // The way back, also past the deadline while a legal hold keeps the workspace pending; absent
    // once the removal has begun, which the server would refuse.
    if (workspace.status === 'PENDING_DELETION' && !workspace.purge_started_at) {
      items.push({ id: 'cancel-deletion', label: t('app.instance.cancel_deletion') });
    }
    if (workspace.status === 'SUSPENDED') items.push({ id: 'resume', label: t('app.instance.resume') });
    // A change of address, in multi mode and never for a workspace leaving, which the server refuses.
    if (isMulti && workspace.status !== 'PENDING_DELETION') {
      items.push({ id: 'change-address', label: t('app.instance.address_action') });
    }
    // Every workspace but one whose removal has begun can be moved, a suspended or pending one
    // included: its plan applies as it is used again.
    if (hasPlans && !workspace.purge_started_at) items.push({ id: 'move', label: t('app.instance.move_action') });
    items.push({ id: 'quotas', label: t('app.instance.quotas') });
    items.push({ id: 'export', label: t('app.instance.export') });
    // Only an active workspace opens: a suspended one refuses its people before any password is
    // asked for, and the server refuses to open it.
    if (workspace.password_opening) items.push({ id: 'close-password', label: t('app.instance.password_close') });
    else if (workspace.status === 'ACTIVE') items.push({ id: 'open-password', label: t('app.instance.password_open_action') });
    if (isMulti && workspace.status !== 'PENDING_DELETION' && !workspace.legal_hold) {
      items.push({ id: 'delete', label: t('app.instance.delete'), isDestructive: true, hasSeparatorBefore: true });
    }
    return items;
  }

  function act(id: string, workspace: AdminTenant): void {
    switch (id) {
      case 'cancel-deletion':
        void run(workspace.id, () => instance.shift(workspace.id, 'cancel-deletion'));
        return;
      case 'resume':
        void run(workspace.id, () => instance.shift(workspace.id, 'resume'));
        return;
      case 'close-password':
        void run(`close:${workspace.id}`, () => instance.closePassword(workspace.id));
        return;
      case 'move':
        moving = workspace;
        return;
      case 'change-address':
        changing = workspace;
        return;
      case 'suspend':
      case 'quotas':
      case 'export':
      case 'open-password':
      case 'delete':
        open(id as Parameters<typeof open>[0], workspace);
        return;
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

  /**
   * Whether the address being typed is one an existing workspace already answers at.
   *
   * Answered from the table this screen already holds rather than by asking the server on every
   * keystroke: the operator is looking at every workspace on the installation, so the one fact
   * needed is already here. The server still refuses a collision — two operators typing at once is
   * a race no client can win — and this is the half that means nobody meets that refusal after
   * filling in four fields.
   */
  const slugTaken = $derived(
    slug.trim() !== '' &&
      workspaces.some((each) => each.slug.toLowerCase() === slug.trim().toLowerCase()),
  );
  /** The host as a row holds it: lower case, without the root's dot. */
  const typedHost = $derived(host.trim().toLowerCase().replace(/\.$/, ''));
  const hostTaken = $derived(
    manifest.hasAssignedHosts && typedHost !== '' && workspaces.some((each) => each.host === typedHost),
  );
  /**
   * The short name the server makes from the host where none is typed - its first label, and the
   * next ones while that is too short - shown as the field's placeholder so the operator sees it
   * before it is made. The server's rule; this is only its preview.
   */
  const shortNameOfHost = $derived.by(() => {
    // The server makes it from the host as a browser sends it, letters beyond ASCII encoded.
    // Only for something that could be a host: no scheme, port, path or space.
    if (!/^[^\s/:@?#*_]+\.[^\s/:@?#*_]+$/.test(typedHost)) return '';
    let ascii = '';
    try {
      ascii = typedHost === '' ? '' : new URL(`http://${typedHost}`).hostname;
    } catch {
      return '';
    }
    let name = '';
    for (const label of ascii.split('.')) {
      name = name === '' ? label : `${name}-${label}`;
      const made = name.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40).replace(/-+$/, '');
      if (made.length >= 3) return made;
    }
    return '';
  });

  function readQuota(name: string, raw: string): void {
    const trimmed = raw.trim();
    // An empty field is the installation's default, not zero: the contract's absent number and a
    // zero are different limits, and only one of them is "unset".
    quotas = { ...quotas, [name]: trimmed === '' ? undefined : Number(trimmed) };
  }

  $effect(() => page.entitle(t('app.instance.workspaces')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.workspaces')} isTitleInBar={viewport.isCompact} breadcrumb={installationTrail({ id: 'screen', label: t('app.instance.workspaces') })} />

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
        {#if isMulti}
          <div>
            <Button tone="primary" onclick={() => open('new')}>{t('app.instance.workspace_new')}</Button>
          </div>
        {/if}

        {#if workspaces.length === 0}
          <Banner tone="info">{t('app.instance.workspaces_none')}</Banner>
        {:else}
          <Table label={t('app.instance.workspaces')} isLabelHidden columns={columns}>
            {#each workspaces as workspace (workspace.id)}
              <tr>
                <td>
                  <span class="name">{workspace.display_name}</span>
                  <!-- The main address - the assigned host where hosts are assigned; the short name
                       where a workspace has no main row yet. -->
                  <span class="address">{workspace.host ?? workspace.slug}</span>
                  {#if workspace.id === ownId}
                    <span class="mark"><Badge tone="neutral">{t('app.instance.own_workspace')}</Badge></span>
                  {/if}
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
                  {#if workspace.password_opening}
                    <!-- An operator's opening of the password, while it stands: until when, and for
                         whom - the facts somebody asking "why can people sign in by password there"
                         needs before anything else. -->
                    <Badge tone="warning">{t('app.instance.password_open')}</Badge>
                    <span class="slug">
                      {t('app.instance.password_open_until', {
                        at: formatDateTime(workspace.password_opening.until, messages.locale),
                        requester: workspace.password_opening.requester,
                      })}
                    </span>
                  {/if}
                  {#if workspace.purge_started_at}
                    <!-- The removal has begun: from here on the deletion cannot be called off. -->
                    <span class="slug">
                      {t('app.instance.purge_started', { at: formatDateTime(workspace.purge_started_at, messages.locale) })}
                    </span>
                  {:else if workspace.purge_after}
                    <!-- The grace, where one stands: the number an operator needs before deciding
                         whether there is still time to call the deletion off. -->
                    <span class="slug">
                      {t('app.instance.purge_after', { at: formatDateTime(workspace.purge_after, messages.locale) })}
                    </span>
                  {/if}
                  {#if workspace.legal_hold}
                    <!-- That a legal hold stands, and what it means here - never which hold: that
                         stays the workspace's (P-01). It is why Delete is not offered (P-05). -->
                    <Badge tone="warning">{t('app.instance.legal_hold')}</Badge>
                    <span class="slug">
                      {workspace.status === 'PENDING_DELETION'
                        ? t('app.instance.legal_hold_waits')
                        : t('app.instance.legal_hold_note')}
                    </span>
                  {/if}
                </td>
                {#if hasPlans}
                  <td>{planName(workspace.plan)}</td>
                {/if}
                <td class="slug">{formatDateTime(workspace.created_at, messages.locale)}</td>
                <td>
                  <!-- One menu per row: nine verbs side by side were a wall of buttons on every row and
                       a column no phone could hold. The items are what the row's state permits. -->
                  <div class="row">
                    <Menu
                      label={t('app.instance.workspace_actions', { name: workspace.display_name })}
                      items={actionsOf(workspace)}
                      placement={{ side: 'block-end', align: 'end' }}
                      onselect={(id) => act(id, workspace)}
                    >
                      {#snippet trigger(props)}
                        <IconButton icon="ellipsis" label={t('app.instance.workspace_actions', { name: workspace.display_name })} size="sm" {...props} />
                      {/snippet}
                    </Menu>
                    {#if working === workspace.id || working === `close:${workspace.id}`}
                      <Spinner label={t('app.instance.working')} />
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

<MovePlanDialog tenant={moving} onclose={() => (moving = undefined)} />
<ChangeAddressDialog
  subject={changing ? { id: changing.id, name: changing.display_name, slug: changing.slug, host: changing.host ?? undefined } : undefined}
  isTaken={(typed) =>
    workspaces.some((each) =>
      each.id !== changing?.id &&
      (manifest.hasAssignedHosts ? each.host === typed : each.slug.toLowerCase() === typed),
    )}
  isAssigned={manifest.hasAssignedHosts}
  change={(body) => instance.changeAddress(changing?.id ?? '', body)}
  onclose={() => (changing = undefined)}
/>

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
      onclick={() => {
        // A taken address is said at its field; pressed, focus goes there and nothing is sent
        // (UC-WRK-32 check 3) - never a switched-off button repeating the field.
        if (slugTaken || hostTaken) {
          void tick().then(() => focusFirst(document));
          return;
        }
        void run('new', async () => {
          provisioned = await instance.provision({
            ...(manifest.hasAssignedHosts ? { host: host.trim() } : {}),
            ...(slug.trim() === '' ? {} : { slug: slug.trim() }),
            display_name: displayName.trim(),
            owner_email: ownerEmail.trim(),
            ...(ownerName.trim() === '' ? {} : { owner_display_name: ownerName.trim() }),
            ...(newPlan === '' ? {} : { plan: newPlan }),
          });
        });
      }}
    >
      {t('app.instance.workspace_create')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <p class="prose">{t('app.instance.workspace_new_intro')}</p>
    {#if manifest.hasAssignedHosts}
      <Input
        label={t('app.instance.host_label')}
        hint={t('app.instance.host_hint')}
        error={hostTaken ? t('app.instance.host_taken', { host: typedHost }) : failure?.fields.get('/host')}
        bind:value={host}
        face="code"
        autocomplete="off"
        spellcheck={false}
        isRequired
      />
      <Input
        label={t('app.instance.short_name_label')}
        hint={t('app.instance.short_name_hint')}
        error={slugTaken ? t('app.instance.slug_taken', { slug: slug.trim() }) : failure?.fields.get('/slug')}
        bind:value={slug}
        placeholder={shortNameOfHost}
        face="code"
        autocomplete="off"
        spellcheck={false}
      />
    {:else}
      <Input
        label={t('app.instance.slug_label')}
        hint={t('app.instance.slug_hint')}
        error={slugTaken ? t('app.instance.slug_taken', { slug: slug.trim() }) : failure?.fields.get('/slug')}
        bind:value={slug}
        autocomplete="off"
        spellcheck={false}
        isRequired
      />
    {/if}
    <Input label={t('app.instance.name_label')} bind:value={displayName} autocomplete="off" isRequired />
    <Input label={t('app.instance.owner_email_label')} hint={t('app.instance.owner_email_hint')} bind:value={ownerEmail} type="email" autocomplete="off" isRequired />
    <Input label={t('app.instance.owner_name_label')} bind:value={ownerName} autocomplete="off" />
    {#if hasPlans}
      <Select
        label={t('app.instance.new_plan_label')}
        hint={t('app.instance.new_plan_hint')}
        options={[
          { value: '', label: t('app.instance.plan_none') },
          ...plans.all.map((plan) => ({ value: plan.name, label: plan.display_name })),
        ]}
        bind:value={newPlan}
        error={failure?.fields.get('/plan')}
      />
    {/if}
  </Stack>
</Dialog>

<Dialog
  title={t('app.instance.suspend_title', { name: acting?.tenant?.display_name ?? '' })}
  isOpen={acting?.kind === 'suspend'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.keep_running')}</Button>
    <Button
      tone="danger"
      isBusy={working === 'suspend'}
      busyLabel={t('app.instance.working')}
      onclick={() => void run('suspend', () => instance.shift(acting?.tenant?.id ?? '', 'suspend'))}
    >
      {t('app.instance.suspend_now')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <!-- What it does to the people in it, before the button. -->
    <Banner tone="warning">{t('app.instance.suspend_cost')}</Banner>
    {#if isOwn}
      <Banner tone="danger">{t('app.instance.suspend_own')}</Banner>
    {/if}
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
    {#if isOwn}
      <Banner tone="danger">{t('app.instance.delete_own')}</Banner>
    {/if}
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
  title={t('app.instance.password_open_title', { name: acting?.tenant?.display_name ?? '' })}
  isOpen={acting?.kind === 'open-password'}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (acting = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (acting = undefined)}>{t('app.instance.cancel')}</Button>
    <Button
      tone="primary"
      isBusy={working === 'open-password'}
      busyLabel={t('app.instance.working')}
      onclick={() =>
        void run('open-password', () =>
          instance.openPassword(acting?.tenant?.id ?? '', {
            hours: Number(hours.trim() === '' ? OPENING_DEFAULT_HOURS : hours.trim()),
            requester: requester.trim(),
            reason: reason.trim(),
          }),
        )}
    >
      {t('app.instance.password_open_now')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <!-- What it does, before the button: every account there that holds a password gets in with it,
         whatever the workspace decided - and its administrators are told. -->
    <Banner tone="warning">{t('app.instance.password_open_cost')}</Banner>
    <Input
      label={t('app.instance.password_open_hours_label')}
      hint={t('app.instance.password_open_hours_hint', { maximum: String(OPENING_MAXIMUM_HOURS) })}
      error={failure?.fields.get('/hours')}
      value={hours}
      oninput={(event) => (hours = (event.currentTarget as HTMLInputElement).value)}
      type="number"
      inputmode="numeric"
      min="1"
      max={String(OPENING_MAXIMUM_HOURS)}
      autocomplete="off"
    />
    <Input
      label={t('app.instance.password_open_requester_label')}
      hint={t('app.instance.password_open_requester_hint')}
      error={failure?.fields.get('/requester')}
      bind:value={requester}
      autocomplete="off"
      isRequired
    />
    <Input
      label={t('app.instance.password_open_reason_label')}
      hint={t('app.instance.password_open_reason_hint')}
      error={failure?.fields.get('/reason')}
      bind:value={reason}
      autocomplete="off"
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
          instance.exportWorkspace(acting?.tenant?.id ?? '', targetChoice === OWN_TARGET ? targetId.trim() : targetChoice),
        )}
    >
      {t('app.instance.export_start')}
    </Button>
  {/snippet}
  <Stack gap="150">
    <p class="prose">{t('app.instance.export_intro')}</p>
    <!-- The installation's targets by name only - where one points is the operator's targets
         screen's, not this choice's - and a workspace's own target by its identifier. -->
    <Select
      label={t('app.instance.export_target_label')}
      hint={t('app.instance.export_target_hint')}
      bind:value={targetChoice}
      options={[
        ...exportTargetChoices(instance.targets),
        { value: OWN_TARGET, label: t('app.instance.export_target_own') },
      ]}
    />
    {#if targetChoice === OWN_TARGET}
      <Input
        label={t('app.instance.export_target_id_label')}
        hint={t('app.instance.export_target_id_hint')}
        bind:value={targetId}
        error={failure?.fields.get('/target_id')}
        autocomplete="off"
        spellcheck={false}
        isRequired
      />
    {/if}
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
      <!-- The placeholder is the number an empty field falls back to - the plan's or the
           installation's - rather than only saying that it falls back. -->
      {@const fallback = fallbackOf(name, acting?.tenant)}
      <Input
        label={t(`app.instance.quota.${name}`)}
        hint={fallback === undefined
          ? t('app.instance.quota_product_default')
          : fallback.plan !== undefined
            ? t('app.instance.quota_plan_default', { plan: fallback.plan, value: fallback.value })
            : t('app.instance.quota_default', { value: fallback.value })}
        placeholder={fallback?.value}
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
  .address {
    display: block;
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    color: var(--text-subtle);
    overflow-wrap: anywhere;
  }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .mark { display: block; margin-block-start: var(--sp-050); white-space: nowrap; }
</style>
