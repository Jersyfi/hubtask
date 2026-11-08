<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Installation → Plans* (UC-INS-13 check 7, multi-tenancy.md §4.2): the plans this installation
  // defines, how many workspaces are on each and what each leaves out, the workspaces on none, and
  // creating one. Multi mode only: a single installation has nothing to put on a plan, and this
  // screen says so rather than offering a plan nobody can use (P-05).
  //
  // **The short name is the platform's.** It is what a purchase platform maps its products to, so
  // it is asked once, at creation, and said to be fixed (multi-tenancy.md §4.3).
  //
  // **A file that enforces the level enforces the plans.** The values show with the file's path and
  // no control: a save the server refuses would be a control that lies.

  import { tick, untrack } from 'svelte';

  import { Banner, Button, DataTable, Dialog, EmptyState, FilterBar, Input, ListPage, Spinner, Stack, type DataColumn, type PageAction } from '@hubtask/design-system/components';
  import { actionsOfHead } from '../lib/palette/actions.ts';
  import { commands } from '../lib/palette/commands.svelte.ts';
  import type { Plan, PlanFeature } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { plans } from '../lib/data/plans.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { check, focusFirst, isComplete, type Missing } from '../lib/forms/missing.ts';

  /** What *Create* found missing when pressed, marked at the field (UC-WRK-32 check 3). */
  let missing = $state<Missing>({});
  import { matchesSearch } from '../lib/page/list.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => page.entitle(t('app.instance.plans')));
  $effect(() => {
    if (!instance.isElevated) return;
    if (!manifest.isMultiTenant) return;
    const stops = untrack(() => [plans.open(), instance.openWorkspaces(), instance.openSettings()]);
    return () => stops.forEach((stop) => stop());
  });

  const reading = $derived(plans.state);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const settings = $derived(instance.settings.status === 'ready' ? instance.settings.data : undefined);
  const enforced = $derived(settings?.is_enforced_from_file === true);
  const withoutPlan = $derived(instance.workspaces.filter((workspace) => !workspace.plan).length);

  const columns = $derived<DataColumn[]>([
    { id: 'plan', label: t('app.instance.plans_col_plan'), priority: 1 },
    { id: 'workspaces', label: t('app.instance.plans_col_workspaces'), align: 'end', priority: 2 },
    { id: 'left_out', label: t('app.instance.plans_col_left_out'), priority: 3 },
  ]);

  // The search narrows the plans loaded, by their names.
  let search = $state('');
  const shown = $derived(plans.all.filter((plan) => matchesSearch(search, [plan.display_name, plan.name])));
  const recordOf = (plan: Plan) => `/instance/plans/${encodeURIComponent(plan.name)}`;

  /** What a plan leaves out, in words, in the domain's order. */
  const leftOutWords = (plan: Plan): string =>
    (plan.left_out ?? []).length === 0
      ? t('app.instance.plans_left_out_none')
      : (plan.left_out ?? []).map((feature: PlanFeature) => t(`app.instance.feature_short.${feature.toLowerCase()}`)).join(', ');

  let isCreating = $state(false);
  let name = $state('');
  let displayName = $state('');
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  function openCreate(): void {
    name = '';
    displayName = '';
    failure = undefined;
    isCreating = true;
  }

  async function create(): Promise<void> {
    missing = check([
      { kind: 'field', key: 'displayName', code: 'app.forms.required', isFilled: displayName.trim() !== '' },
      { kind: 'field', key: 'name', code: 'app.forms.required', isFilled: name.trim() !== '' },
    ]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(document));
      return;
    }
    isWorking = true;
    failure = undefined;
    try {
      const created = await plans.create({ name: name.trim(), display_name: displayName.trim() });
      isCreating = false;
      onnavigate(`/instance/plans/${encodeURIComponent(created.name)}`);
    } catch (cause) {
      failure = cause instanceof TransportError
        ? renderProblem(cause, messages)
        : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }

  /** The head's one action, handed to the head and to the palette alike (UC-WRK-27 check 2). */
  const headPrimary = $derived<PageAction | undefined>(
    manifest.isMultiTenant && !enforced && reading.status === 'ready'
      ? { label: t('app.instance.plans_new'), icon: 'plus', opener: 'new-plan', onclick: openCreate }
      : undefined,
  );
  $effect(() => commands.offer('page', actionsOfHead('page', { primary: headPrimary })));

  let createForm = $state<HTMLFormElement | undefined>(undefined);
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage head={{ title: t('app.instance.plans'), breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.plans') }), primary: headPrimary }} refused={door ?? (unreadable ? failed : undefined)}>
      {#snippet intro()}
        <Stack gap="100">
          <p class="quiet">{t(manifest.isMultiTenant ? 'app.instance.plans_intro' : 'app.instance.plans_single')}</p>
          {#if enforced}<Banner tone="info" title={t('app.instance.source_file_enforced')}>{settings?.source ?? ''}</Banner>{/if}
          {#if manifest.isMultiTenant && instance.workspacesState.status === 'ready'}
            <p class="quiet" data-testid="without-plan">{t('app.instance.plans_without', { count: withoutPlan })}</p>
          {/if}
        </Stack>
      {/snippet}
      {#snippet filters()}
        {#if manifest.isMultiTenant}
          <FilterBar
            searchLabel={t('app.instance.plans_search')}
            clearSearchLabel={t('app.page.clear_search')}
            bind:search
            countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(plans.all.length) }) : undefined}
          />
        {/if}
      {/snippet}

      {#if manifest.isMultiTenant}
        {#if reading.status === 'loading' || reading.status === 'idle'}
          <p class="quiet"><Spinner label={t('app.instance.reading')} /> <span>{t('app.instance.reading')}</span></p>
        {:else}
          <DataTable
            label={t('app.instance.plans')}
            {columns}
            rows={shown}
            key={(plan: Plan) => plan.name}
            href={recordOf}
            rowLabel={(plan: Plan) => plan.display_name}
            shownLabel={plans.all.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
          >
            {#snippet cell(plan: Plan, column: DataColumn)}
              {#if column.id === 'plan'}
                <span class="name">{plan.display_name}</span>
                <span class="small">{plan.name}</span>
              {:else if column.id === 'workspaces'}
                <span class="count">{plan.workspaces}</span>
              {:else}
                <span class="quiet">{leftOutWords(plan)}</span>
              {/if}
            {/snippet}
            {#snippet empty()}
              {#if plans.all.length === 0}
                <EmptyState kind="unused" title={t('app.instance.plans_none')} />
              {:else}
                <EmptyState kind="filtered" title={t('app.instance.plans_none_found')}>
                  {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
                </EmptyState>
              {/if}
            {/snippet}
          </DataTable>
        {/if}
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<Dialog
  size="m"
  title={t('app.instance.plan_create_title')}
  isOpen={isCreating}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (isCreating = false)}
  primary={{ label: t('app.instance.plan_create'), isBusy: isWorking, busyLabel: t('app.instance.working'), onclick: () => createForm?.requestSubmit() }}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isCreating = false)}>{t('app.instance.cancel')}</Button>
  {/snippet}
  <form bind:this={createForm} onsubmit={(event) => { event.preventDefault(); void create(); }}>
    <Stack gap="150">
      {#if failure && !failure.fields.size}
        <FailureNote problem={failure} />
      {/if}
      <Input
        label={t('app.instance.plan_display_name')}
        bind:value={displayName}
        error={failure?.fields.get('/display_name') ?? (missing.displayName && !displayName.trim() ? t(missing.displayName) : undefined)}
        isRequired
      />
      <Input
        label={t('app.instance.plan_short_name')}
        hint={t('app.instance.plan_short_name_hint')}
        bind:value={name}
        error={failure?.fields.get('/name') ?? (missing.name && !name.trim() ? t(missing.name) : undefined)}
        isRequired
      />
    </Stack>
  </form>
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); display: flex; align-items: center; gap: var(--sp-100); }
  .name { display: block; font-weight: var(--fw-medium); }
  .small { display: block; color: var(--text-subtle); font-size: var(--fs-075); }
  .count { font-variant-numeric: tabular-nums; }
  form { margin: 0; }
</style>
