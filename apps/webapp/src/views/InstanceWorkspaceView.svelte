<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One workspace at the installation as its record page (page-system §7.2, §9.4): its standing,
  // its limits and what is in use, its plan and its own domains as regions; every act the state
  // permits in the head's ⋯, suspending and deleting in the danger zone, each opening its dialog.
  //
  // **Never a row from inside the workspace.** The `tenant` row, its standing, its limits - the
  // same row the list shows, read from the installation's own list of workspaces.

  import { untrack } from 'svelte';

  import { Badge, EmptyState, RecordPage, Spinner, Stack, type RecordList } from '@hubtask/design-system/components';
  import type { DangerAct } from '@hubtask/design-system/components';
  import type { AdminTenant } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import OwnDomainsTable from '../lib/instance/OwnDomainsTable.svelte';
  import WorkspaceActs from '../lib/instance/WorkspaceActs.svelte';
  import { workspaceActs } from '../lib/instance/workspaceacts.ts';
  import { limitStanding } from '../lib/instance/quotause.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { workspace as own } from '../lib/data/workspace.svelte.ts';
  import { plans } from '../lib/data/plans.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const LIST = '/instance/workspaces';

  $effect(() => {
    if (!instance.isElevated) return;
    const stops = untrack(() => [instance.openWorkspaces(), instance.openNearQuota(), ...(manifest.isMultiTenant ? [plans.open()] : [])]);
    return () => stops.forEach((stop) => stop());
  });
  $effect(() => untrack(() => own.open()));

  const reading = $derived(instance.workspacesState);
  const tenant = $derived(instance.workspaces.find((each) => each.id === id));
  const isMulti = $derived(manifest.isMultiTenant);
  const hasPlans = $derived(isMulti && plans.all.length > 0);
  const nearById = $derived(new Map(instance.nearQuota.map((entry) => [entry.tenant_id, entry])));
  const near = $derived(nearById.get(id));
  const standing = $derived(limitStanding(near));
  const title = $derived(tenant?.display_name ?? t('app.instance.workspace_kind'));
  $effect(() => page.entitle(title));

  let acts = $state<WorkspaceActs | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const DANGER = new Set(['suspend', 'delete']);
  const items = $derived(tenant ? workspaceActs(tenant, { isMulti, hasPlans }) : []);
  const dangerActs = $derived<DangerAct[]>(
    items
      .filter((item) => DANGER.has(item.id))
      .map((item) => ({
        id: item.id,
        title: t(`app.instance.danger_${item.id}_title`),
        cost: item.id === 'delete' ? t('app.instance.delete_cost', { days: '30' }) : t('app.instance.suspend_cost'),
        actionLabel: `${item.label}…`,
      })),
  );

  const toneOf = (status: AdminTenant['status']) => (status === 'ACTIVE' ? 'success' : status === 'SUSPENDED' ? 'warning' : 'danger');
  const planName = (name: string | null | undefined): string =>
    name ? (plans.named(name)?.display_name ?? name) : t('app.instance.plan_none');
  const KNOWN_LIMITS = new Set([
    'api_requests_per_minute', 'items', 'media_bytes', 'automation_runs_per_hour', 'webhook_targets',
    'export_jobs', 'ai_tokens_per_day', 'ai_own_tokens_per_day', 'ai_arrival_proposals_per_day', 'accounts', 'mailboxes',
  ]);
  const limitName = (quota: string) => (KNOWN_LIMITS.has(quota) ? t(`app.quotas.name.${quota}`) : quota);

  const lists = $derived<RecordList[]>(
    tenant
      ? [
          { id: 'standing', title: t('app.instance.column_state') },
          {
            id: 'limits',
            title: t('app.instance.quotas'),
            count: standing ? t(`app.instance.limit_${standing}`) : undefined,
            add: { label: t('app.instance.quotas_change'), icon: 'pencil', onclick: () => acts?.act('quotas', tenant!) },
          },
          ...(hasPlans
            ? [{
                id: 'plan',
                title: t('app.instance.column_plan'),
                add: tenant.purge_started_at ? undefined : { label: t('app.instance.move_action'), icon: 'arrow-right' as const, onclick: () => acts?.act('move', tenant!) },
              }]
            : []),
          { id: 'domains', title: t('app.instance.own_domains'), count: String(instance.ownDomains.filter((row) => row.tenant_id === id).length) },
        ]
      : [],
  );

  const facts = $derived(
    tenant
      ? {
          title: t('app.instance.workspace_facts'),
          rows: [
            { label: t('app.instance.workspace_address'), value: tenant.host ?? tenant.slug },
            { label: t('app.instance.column_since'), value: formatDateTime(tenant.created_at, messages.locale) },
            ...(tenant.default_locale ? [{ label: t('app.instance.workspace_locale'), value: tenant.default_locale }] : []),
            ...(tenant.default_time_zone ? [{ label: t('app.instance.workspace_time_zone'), value: tenant.default_time_zone }] : []),
            ...(tenant.external_reference ? [{ label: t('app.instance.workspace_reference'), value: tenant.external_reference }] : []),
          ],
        }
      : undefined,
  );
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <RecordPage
      record={{
        kind: t('app.instance.workspace_kind'),
        title,
        back: { label: t('app.instance.workspaces'), href: LIST, onnavigate: () => onnavigate(LIST) },
        status: tenant ? { label: t(`app.instance.state_${tenant.status.toLowerCase()}`), tone: toneOf(tenant.status) } : undefined,
        facts: tenant ? [tenant.host ?? tenant.slug, tenant.id === own.workspace?.id ? t('app.instance.own_workspace') : undefined].filter(Boolean).join(' · ') : undefined,
        menu: tenant && items.some((item) => !DANGER.has(item.id))
          ? {
              label: t('app.instance.workspace_actions', { name: title }),
              items: items.filter((item) => !DANGER.has(item.id)).map((item) => ({ ...item, hasSeparatorBefore: false })),
              onselect: (item: string) => acts?.act(item, tenant!),
            }
          : undefined,
      }}
      {lists}
      {facts}
      danger={tenant && dangerActs.length > 0 ? { title: t('app.page.danger_zone'), acts: dangerActs, onact: (act: string) => acts?.act(act, tenant!) } : undefined}
      refused={door ?? (reading.status === 'failed' || (reading.status === 'ready' && !tenant) ? notHere : undefined)}
    >
      {#snippet list(item: RecordList)}
        {#if tenant && item.id === 'standing'}
          <Stack gap="100">
            {#if failure}<FailureNote problem={failure} />{/if}
            <p class="line">
              <Badge tone={toneOf(tenant.status)}>{t(`app.instance.state_${tenant.status.toLowerCase()}`)}</Badge>
              {#if working}<Spinner label={t('app.instance.working')} />{/if}
            </p>
            {#if tenant.password_opening}
              <!-- An operator's opening of the password, while it stands: until when, and for whom -
                   the facts somebody asking "why can people sign in by password there" needs first. -->
              <p class="line">
                <Badge tone="warning">{t('app.instance.password_open')}</Badge>
                <span class="quiet">{t('app.instance.password_open_until', { at: formatDateTime(tenant.password_opening.until, messages.locale), requester: tenant.password_opening.requester })}</span>
              </p>
            {/if}
            {#if tenant.purge_started_at}
              <!-- The removal has begun: from here on the deletion cannot be called off. -->
              <p class="quiet">{t('app.instance.purge_started', { at: formatDateTime(tenant.purge_started_at, messages.locale) })}</p>
            {:else if tenant.purge_after}
              <p class="quiet">{t('app.instance.purge_after', { at: formatDateTime(tenant.purge_after, messages.locale) })}</p>
            {/if}
            {#if tenant.legal_hold}
              <!-- That a legal hold stands - never which hold: that stays the workspace's (P-01). -->
              <p class="line">
                <Badge tone="warning">{t('app.instance.legal_hold')}</Badge>
                <span class="quiet">{tenant.status === 'PENDING_DELETION' ? t('app.instance.legal_hold_waits') : t('app.instance.legal_hold_note')}</span>
              </p>
            {/if}
          </Stack>
        {:else if tenant && item.id === 'limits'}
          {#if near && near.quotas.length > 0}
            <ul class="lines" data-near-quota>
              {#each near.quotas as each (each.quota)}
                <li>
                  <span>{limitName(each.quota)}</span>
                  <Badge tone={(each.ratio ?? 0) >= 1 ? 'danger' : 'warning'}>
                    {t('app.instance.near_quota_standing', { name: limitName(each.quota), used: each.used ?? 0, limit: each.limit, percent: Math.round((each.ratio ?? 0) * 100) })}
                  </Badge>
                </li>
              {/each}
            </ul>
          {:else}
            <p class="quiet">{t('app.instance.limits_calm')}</p>
          {/if}
        {:else if tenant && item.id === 'plan'}
          <p class="line">{planName(tenant.plan)}</p>
        {:else if item.id === 'domains'}
          <OwnDomainsTable tenantId={id} />
        {/if}
      {/snippet}
    </RecordPage>
  {/snippet}
</InstanceGate>

<WorkspaceActs bind:this={acts} workspaces={instance.workspaces} ownId={own.workspace?.id} {nearById} bind:working bind:failure ondeleted={() => onnavigate(LIST)} />

{#snippet notHere()}
  {#if reading.status === 'failed'}
    <FailureNote problem={renderProblem(reading.error, messages)} />
  {:else}
    <EmptyState kind="settled" title={t('app.instance.workspace_not_here')} />
  {/if}
{/snippet}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .line { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .lines { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  .lines li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-050) var(--sp-100); }
</style>
