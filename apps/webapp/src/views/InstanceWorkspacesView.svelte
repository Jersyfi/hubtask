<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's workspaces and their whole lifecycle (ADR-0070 §5), as a list page
  // (page-system §7.1, §9.4): *New workspace* the head's primary, opening its create page; the
  // short name the search, state, plan and limits as chips; a row opening the workspace's record
  // page, its ⋯ every act the row's state permits.
  //
  // **Everything the control plane can do, this screen can do.** An operator who can only read here
  // and has to reach for a terminal to act is an operator for whom the dashboard is a status page.
  //
  // **And never a row from inside a workspace.** The `tenant` row, its standing, its limits: the
  // tenant boundary is a database policy rather than a role, and this screen does not go around it.
  //
  // **The operator's own workspace is marked**, because suspending or deleting it ends the
  // operator's own session at once. **In single mode** the one workspace is the installation: no
  // *New workspace*, *Suspend* or *Delete*, which the server refuses there.

  import { untrack } from 'svelte';

  import { Badge, Button, DataTable, EmptyState, FilterBar, ListPage, Radio, Spinner, type DataColumn, type Filter } from '@hubtask/design-system/components';
  import type { AdminTenant } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import OwnDomainsTable from '../lib/instance/OwnDomainsTable.svelte';
  import WorkspaceActs from '../lib/instance/WorkspaceActs.svelte';
  import { workspaceActs } from '../lib/instance/workspaceacts.ts';
  import { countByLimit, limitStanding, narrowedByLimit, type LimitFilter } from '../lib/instance/quotause.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance, type WorkspaceFilter } from '../lib/data/instance.svelte.ts';
  import { workspace as own } from '../lib/data/workspace.svelte.ts';
  import { plans } from '../lib/data/plans.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    if (!instance.isElevated) return;
    return untrack(() => instance.openNearQuota());
  });

  // The rows: a page at a time of the narrowing in force, never every workspace at once. The short
  // name is applied a moment after the last keystroke rather than on each.
  let findSlug = $state('');
  let findStatus = $state('');
  let findPlan = $state('');
  let filter = $state<WorkspaceFilter>({});
  $effect(() => {
    const next: WorkspaceFilter = {
      slug: findSlug,
      status: (findStatus || undefined) as WorkspaceFilter['status'],
      plan: findPlan,
    };
    const timer = setTimeout(() => { filter = next; }, findSlug === (untrack(() => filter.slug) ?? '') ? 0 : 300);
    return () => clearTimeout(timer);
  });
  $effect(() => {
    if (!instance.isElevated) return;
    const narrowing = filter;
    return untrack(() => instance.openWorkspacePage(narrowing));
  });
  /** Near or at a limit: narrowed on the rows read, since the list's own read has no such filter. */
  let findLimit = $state<LimitFilter>('');
  const isNarrowed = $derived(Boolean(filter.slug?.trim() || filter.status || filter.plan || findLimit));
  let loadingMore = $state(false);
  async function more(): Promise<void> {
    loadingMore = true;
    try {
      await instance.loadMoreWorkspaces();
    } finally {
      loadingMore = false;
    }
  }

  // Plans exist only where there are several workspaces; a single installation has none.
  $effect(() => {
    if (!instance.isElevated) return;
    if (!manifest.isMultiTenant) return;
    return untrack(() => plans.open());
  });
  const hasPlans = $derived(manifest.isMultiTenant && plans.all.length > 0);
  const planName = (name: string | null | undefined): string =>
    name ? (plans.named(name)?.display_name ?? name) : t('app.instance.plan_none');

  $effect(() => untrack(() => own.open()));
  const ownId = $derived(own.workspace?.id);
  const isMulti = $derived(manifest.isMultiTenant);

  const reading = $derived(instance.workspacePage);
  const workspaces = $derived(reading.status === 'ready' ? reading.data.data : []);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const nearById = $derived(new Map(instance.nearQuota.map((entry) => [entry.tenant_id, entry])));
  const shownWorkspaces = $derived(narrowedByLimit(workspaces, nearById, findLimit));
  const limitedInAll = $derived(findLimit ? countByLimit(instance.nearQuota, findLimit) : 0);

  const columns = $derived<DataColumn[]>([
    { id: 'workspace', label: t('app.instance.column_workspace'), priority: 1 },
    { id: 'state', label: t('app.instance.column_state'), priority: 2, isStatus: true },
    ...(hasPlans ? [{ id: 'plan', label: t('app.instance.column_plan'), priority: 3 as const }] : []),
    { id: 'since', label: t('app.instance.column_since'), priority: 3 },
  ]);

  const STATES = ['ACTIVE', 'SUSPENDED', 'PENDING_DELETION'] as const;
  const anyOr = (label: string, choice: string | undefined) => `${label}: ${choice ?? t('app.instance.find_any')}`;
  const chips = $derived<Filter[]>([
    { id: 'state', label: anyOr(t('app.instance.find_status'), findStatus ? t(`app.instance.state_${findStatus.toLowerCase()}`) : undefined), isActive: findStatus !== '' },
    ...(hasPlans
      ? [{ id: 'plan', label: anyOr(t('app.instance.find_plan'), findPlan === '' ? undefined : findPlan === '-' ? t('app.instance.plan_none') : planName(findPlan)), isActive: findPlan !== '' }]
      : []),
    { id: 'limit', label: anyOr(t('app.instance.find_limit'), findLimit === 'near' ? t('app.instance.find_limit_near') : findLimit === 'at' ? t('app.instance.limit_at') : undefined), isActive: findLimit !== '' },
  ]);
  const activeFilters = $derived(chips.filter((chip) => chip.isActive).length);
  function clearFilters(): void {
    findStatus = '';
    findPlan = '';
    findLimit = '';
  }

  let acts = $state<WorkspaceActs | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const toneOf = (status: AdminTenant['status']) => (status === 'ACTIVE' ? 'success' : status === 'SUSPENDED' ? 'warning' : 'danger');
  const recordOf = (each: AdminTenant) => `/instance/workspaces/${each.id}`;

  $effect(() => page.entitle(t('app.instance.workspaces')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage
      head={{
        title: t('app.instance.workspaces'),
        breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.workspaces') }),
        primary: isMulti
          ? { label: t('app.instance.workspace_new'), icon: 'plus', onclick: () => onnavigate('/instance/workspaces/new') }
          : undefined,
      }}
      refused={door ?? (unreadable ? failed : undefined)}
    >
      {#snippet intro()}<p class="quiet">{t('app.instance.workspaces_intro')}</p>{/snippet}
      {#snippet filters()}
        <FilterBar
          searchLabel={t('app.instance.find_slug_search')}
          clearSearchLabel={t('app.page.clear_search')}
          bind:search={findSlug}
          filters={chips}
          moreLabel={t('app.page.more_filters')}
          filtersLabel={t('app.page.filters', { count: String(activeFilters) })}
          sheetTitle={t('app.page.filters', { count: '0' })}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          showLabel={t('app.page.show_results', { count: String(shownWorkspaces.length) })}
          clearLabel={t('app.page.clear')}
          onclear={clearFilters}
          countLabel={reading.status === 'ready' ? t('app.instance.workspaces_found', { count: String(shownWorkspaces.length) }) : undefined}
        >
          {#snippet picker(chip: Filter)}
            {#if chip.id === 'state'}
              <Radio
                label={t('app.instance.find_status')}
                bind:value={findStatus}
                options={[{ value: '', label: t('app.instance.find_any') }, ...STATES.map((value) => ({ value, label: t(`app.instance.state_${value.toLowerCase()}`) }))]}
              />
            {:else if chip.id === 'plan'}
              <Radio
                label={t('app.instance.find_plan')}
                bind:value={findPlan}
                options={[
                  { value: '', label: t('app.instance.find_any') },
                  { value: '-', label: t('app.instance.plan_none') },
                  ...plans.all.map((plan) => ({ value: plan.name, label: plan.display_name })),
                ]}
              />
            {:else}
              <Radio
                label={t('app.instance.find_limit')}
                bind:value={() => findLimit, (value) => (findLimit = value as LimitFilter)}
                options={[
                  { value: '', label: t('app.instance.find_any') },
                  { value: 'near', label: t('app.instance.find_limit_near') },
                  { value: 'at', label: t('app.instance.limit_at') },
                ]}
              />
            {/if}
          {/snippet}
        </FilterBar>
      {/snippet}

      {#if failure}<FailureNote problem={failure} />{/if}
      {#if findLimit && instance.hasMoreWorkspaces && shownWorkspaces.length < limitedInAll}
        <p class="quiet small" data-limit-more>{t('app.instance.find_limit_more', { shown: String(shownWorkspaces.length), count: String(limitedInAll) })}</p>
      {/if}
      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else}
        <DataTable
          label={t('app.instance.workspaces')}
          {columns}
          rows={shownWorkspaces}
          key={(each: AdminTenant) => each.id}
          href={recordOf}
          rowLabel={(each: AdminTenant) => each.display_name}
          menu={(each: AdminTenant) => ({ label: t('app.instance.workspace_actions', { name: each.display_name }), items: workspaceActs(each, { isMulti, hasPlans }) })}
          onmenu={(each: AdminTenant, id: string) => acts?.act(id, each)}
          actionsLabel={t('app.instance.column_actions')}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          shownLabel={workspaces.length === 0 ? undefined : t('app.page.showing', { count: String(shownWorkspaces.length) })}
          more={instance.hasMoreWorkspaces
            ? { label: t('app.instance.workspaces_more'), isBusy: loadingMore, hasMore: true, onLoadMore: () => void more() }
            : undefined}
        >
          {#snippet cell(each: AdminTenant, column: DataColumn)}
            {#if column.id === 'workspace'}
              {@const standing = limitStanding(nearById.get(each.id))}
              <span class="name">{each.display_name}</span>
              <!-- The main address - the assigned host where hosts are assigned; the short name
                   where a workspace has no main row yet. -->
              <span class="address">{each.host ?? each.slug}</span>
              {#if each.id === ownId || standing || each.password_opening || each.legal_hold}
                <span class="marks">
                  {#if each.id === ownId}<Badge tone="neutral">{t('app.instance.own_workspace')}</Badge>{/if}
                  <!-- Counts only, from the installation's own answer: which limit and how far stand
                       on the record page and on the overview. -->
                  {#if standing}<Badge tone={standing === 'at' ? 'danger' : 'warning'}>{t(`app.instance.limit_${standing}`)}</Badge>{/if}
                  {#if each.password_opening}<Badge tone="warning">{t('app.instance.password_open')}</Badge>{/if}
                  <!-- That a legal hold stands - never which hold: that stays the workspace's (P-01).
                       It is why Delete is not offered (P-05). -->
                  {#if each.legal_hold}<Badge tone="warning">{t('app.instance.legal_hold')}</Badge>{/if}
                </span>
              {/if}
            {:else if column.id === 'state'}
              <span class="marks">
                <Badge tone={toneOf(each.status)}>{t(`app.instance.state_${each.status.toLowerCase()}`)}</Badge>
                {#if working === each.id || working === `close:${each.id}`}<Spinner label={t('app.instance.working')} />{/if}
              </span>
              {#if each.password_opening}
                <!-- Until when, and for whom: what somebody asking "why can people sign in by
                     password there" needs before anything else. -->
                <span class="slug">{t('app.instance.password_open_until', { at: formatDateTime(each.password_opening.until, messages.locale), requester: each.password_opening.requester })}</span>
              {/if}
              {#if each.legal_hold}
                <span class="slug">{each.status === 'PENDING_DELETION' ? t('app.instance.legal_hold_waits') : t('app.instance.legal_hold_note')}</span>
              {/if}
              {#if each.purge_started_at}
                <!-- The removal has begun: from here on the deletion cannot be called off. -->
                <span class="slug">{t('app.instance.purge_started', { at: formatDateTime(each.purge_started_at, messages.locale) })}</span>
              {:else if each.purge_after}
                <span class="slug">{t('app.instance.purge_after', { at: formatDateTime(each.purge_after, messages.locale) })}</span>
              {/if}
            {:else if column.id === 'plan'}
              {planName(each.plan)}
            {:else}
              <span class="slug">{formatDateTime(each.created_at, messages.locale)}</span>
            {/if}
          {/snippet}
          {#snippet empty()}
            {#if isNarrowed}
              <EmptyState kind="filtered" title={t('app.instance.workspaces_none_found')}>
                {#snippet action()}
                  {#if findLimit && instance.hasMoreWorkspaces}
                    <!-- The rows read so far hold none; the ones not read yet may. -->
                    <Button onclick={more} isBusy={loadingMore}>{t('app.instance.workspaces_more')}</Button>
                  {:else}
                    <Button onclick={() => { clearFilters(); findSlug = ''; }}>{t('app.page.clear')}</Button>
                  {/if}
                {/snippet}
              </EmptyState>
            {:else}
              <EmptyState kind="unused" title={t('app.instance.workspaces_none')} />
            {/if}
          {/snippet}
        </DataTable>
        <!-- UC-INS-15 check 10: below the workspaces, as the approved screen places it; absent
             where no workspace set up an own domain. -->
        <OwnDomainsTable />
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

<WorkspaceActs bind:this={acts} workspaces={workspaces} {ownId} {nearById} bind:working bind:failure />

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .small { font-size: var(--fs-100); }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .address { display: block; font-family: var(--font-mono); font-size: var(--fs-075); color: var(--text-subtle); overflow-wrap: anywhere; }
  .marks { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); margin-block-start: var(--sp-050); }
</style>
