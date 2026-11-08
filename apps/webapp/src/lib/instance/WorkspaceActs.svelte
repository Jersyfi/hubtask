<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A workspace's acts and their dialogs, the same from the list's row and from its record page
  // (page-system §9.4): suspending and deleting each a Dialog S, opening the password and an export
  // Dialogs M, the ten limits a Dialog L; moving between plans and changing the address their own.
  //
  // **Every act that costs something says so where it is done.** The deletion asks for the display
  // name to be typed - the server compares it, and a deletion nobody typed the name of is a deletion
  // somebody clicked past. **Opening the password for one workspace** (ADR-0078 §3) is the lever
  // for a provider that is switched on but broken: a day unless said otherwise, a week at most, with
  // who asked and why. **The operator's own workspace** is warned about, because suspending or
  // deleting it ends the operator's own session at once.

  import { untrack } from 'svelte';

  import { Banner, Button, Dialog, Input, Select, Stack } from '@hubtask/design-system/components';
  import type { AdminTenant, TenantQuotas, WorkspaceNearQuota } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import ChangeAddressDialog from '../address/ChangeAddressDialog.svelte';
  import { OWN_TARGET, exportTargetChoices } from '../backup/targets.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { instance } from '../data/instance.svelte.ts';
  import { plans } from '../data/plans.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import MovePlanDialog from './MovePlanDialog.svelte';
  import { isBelowUse, useOf } from './quotause.ts';

  interface Props {
    /** The workspaces read, to say at the field that an address is taken. */
    workspaces: readonly AdminTenant[];
    /** The operator's own workspace, warned about. */
    ownId?: string;
    /** Each workspace's use near its limits, by id. */
    nearById: ReadonlyMap<string, WorkspaceNearQuota>;
    /** The act in progress, for the row's spinner: a workspace's id, or `close:` and its id. */
    working?: string;
    /** A refusal of an act without a dialog: resuming, calling a deletion off, closing the password. */
    failure?: ReturnType<typeof renderProblem>;
    /** After a deletion was asked for. */
    ondeleted?: () => void;
  }

  let { workspaces, ownId, nearById, working = $bindable(), failure = $bindable(), ondeleted }: Props = $props();

  /** The opening's bounds, as the contract states them: a day unless said otherwise, a week at most. */
  const OPENING_DEFAULT_HOURS = 24;
  const OPENING_MAXIMUM_HOURS = 168;
  /** A deletion's grace as the contract states it (multi-tenancy.md §5): fixed, not configured. */
  const DELETION_GRACE_DAYS = 30;

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
    'mailboxes',
  ] as const;

  type Kind = 'suspend' | 'delete' | 'export' | 'quotas' | 'open-password';
  /** Which act is open, and on which workspace. One at a time: two dialogs would be two answers. */
  let acting = $state<{ kind: Kind; tenant: AdminTenant } | undefined>(undefined);
  let moving = $state<AdminTenant | undefined>(undefined);
  let changing = $state<AdminTenant | undefined>(undefined);

  let confirmation = $state('');
  /** An installation target's identifier, or OWN_TARGET for one of the workspace's own, typed. */
  let targetId = $state('');
  let targetChoice = $state(OWN_TARGET);
  let quotas = $state<TenantQuotas>({});
  let hours = $state(String(OPENING_DEFAULT_HOURS));
  let requester = $state('');
  let reason = $state('');

  $effect(() => {
    if (!instance.isElevated) return;
    // The instance level too, so the limits dialog can say what an empty field actually means.
    const stops = untrack(() => [instance.openSettings(), instance.openTargets()]);
    return () => stops.forEach((stop) => stop());
  });

  const isOwn = $derived(acting !== undefined && acting.tenant.id === ownId);

  function instanceDefault(name: string): string | undefined {
    const settings = instance.settings.status === 'ready' ? instance.settings.data : undefined;
    const entry = settings?.quotas?.[name];
    if (!entry?.set) return undefined;
    return String(entry.value);
  }

  /** What an empty limit falls back to: the plan's, unless the installation locks its own. */
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

  function open(kind: Kind, tenant: AdminTenant): void {
    acting = { kind, tenant };
    failure = undefined;
    confirmation = '';
    targetId = '';
    targetChoice = instance.targets.find((each) => each.enabled !== false)?.id ?? OWN_TARGET;
    quotas = {};
    hours = String(OPENING_DEFAULT_HOURS);
    requester = '';
    reason = '';
  }

  /** One of `workspaceActs`' items, on one workspace. */
  export function act(id: string, workspace: AdminTenant): void {
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
        open(id, workspace);
        return;
    }
  }

  async function run(id: string, call: () => Promise<void>): Promise<void> {
    working = id;
    failure = undefined;
    try {
      await call();
      const wasDeleting = acting?.kind === 'delete';
      acting = undefined;
      if (wasDeleting) ondeleted?.();
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      working = undefined;
    }
  }

  function readQuota(name: string, raw: string): void {
    const trimmed = raw.trim();
    quotas = { ...quotas, [name]: trimmed === '' ? undefined : Number(trimmed) };
  }
</script>

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
  size="s"
  title={t('app.instance.suspend_title', { name: acting?.tenant.display_name ?? '' })}
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
      onclick={() => void run('suspend', () => instance.shift(acting?.tenant.id ?? '', 'suspend'))}
    >
      {t('app.instance.suspend_now')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <!-- What it does to the people in it, before the button. -->
    <Banner tone="warning">{t('app.instance.suspend_cost')}</Banner>
    {#if isOwn}
      <FailureNote problem={t('app.instance.suspend_own')} />
    {/if}
  </Stack>
</Dialog>

<Dialog
  size="s"
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
          instance.requestDeletion(acting?.tenant.id ?? '', confirmation.trim()),
        )}
    >
      {t('app.instance.delete_now')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <!-- What it costs, before the button and not in a help page. -->
    <Banner tone="warning">{t('app.instance.delete_cost', { days: String(DELETION_GRACE_DAYS) })}</Banner>
    {#if isOwn}
      <FailureNote problem={t('app.instance.delete_own')} />
    {/if}
    <Input
      label={t('app.instance.delete_confirm_label', { name: acting?.tenant.display_name ?? '' })}
      hint={t('app.instance.delete_confirm_hint')}
      bind:value={confirmation}
      autocomplete="off"
      spellcheck={false}
      isRequired
    />
  </Stack>
</Dialog>

<Dialog
  size="m"
  title={t('app.instance.password_open_title', { name: acting?.tenant.display_name ?? '' })}
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
          instance.openPassword(acting?.tenant.id ?? '', {
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
    {#if failure}<FailureNote problem={failure} />{/if}
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
  size="m"
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
          instance.exportWorkspace(acting?.tenant.id ?? '', targetChoice === OWN_TARGET ? targetId.trim() : targetChoice),
        )}
    >
      {t('app.instance.export_start')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
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
  size="l"
  title={t('app.instance.quotas_title', { name: acting?.tenant.display_name ?? '' })}
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
      onclick={() => void run('quotas', () => instance.setQuotas(acting?.tenant.id ?? '', quotas))}
    >
      {t('app.instance.quotas_save')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
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
      {@const used = useOf(nearById.get(acting?.tenant.id ?? ''), name)}
      {#if used !== undefined}
        <p class="use" data-quota-use={name}>
          {t('app.instance.quota_in_use', { used })}
          {#if isBelowUse(quotas[name] ?? undefined, used)}
            <span class="below">{t('app.instance.quota_below_use')}</span>
          {/if}
        </p>
      {/if}
    {/each}
  </Stack>
</Dialog>

<style>
  .use { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .below { display: block; color: var(--text-warning); }
  .prose { margin: 0; max-inline-size: 60ch; }
</style>
