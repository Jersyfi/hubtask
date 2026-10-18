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

  import { untrack } from 'svelte';

  import { Banner, Button, Dialog, Input, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { Plan, PlanFeature } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { plans } from '../lib/data/plans.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => page.entitle(t('app.instance.plans')));
  $effect(() => {
    if (!manifest.isMultiTenant) return;
    const stops = untrack(() => [plans.open(), instance.openWorkspaces(), instance.openSettings()]);
    return () => stops.forEach((stop) => stop());
  });

  const reading = $derived(plans.state);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const settings = $derived(instance.settings.status === 'ready' ? instance.settings.data : undefined);
  const enforced = $derived(settings?.is_enforced_from_file === true);
  const withoutPlan = $derived(instance.workspaces.filter((workspace) => !workspace.plan).length);

  const columns = $derived([
    { id: 'plan', label: t('app.instance.plans_col_plan') },
    { id: 'name', label: t('app.instance.plans_col_name') },
    { id: 'workspaces', label: t('app.instance.plans_col_workspaces'), align: 'end' as const },
    { id: 'left_out', label: t('app.instance.plans_col_left_out') },
  ]);

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
    isWorking = true;
    failure = undefined;
    try {
      const created = await plans.create({ name: name.trim(), display_name: displayName.trim() });
      isCreating = false;
      onnavigate(`/instance/plans/${encodeURIComponent(created.name)}`);
    } catch (cause) {
      failure = cause instanceof TransportError
        ? renderProblem(cause, messages)
        : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.instance.plans')}
    isTitleInBar={viewport.isCompact}
    primary={manifest.isMultiTenant && !enforced && reading.status === 'ready'
      ? { label: t('app.instance.plans_new'), icon: 'plus', opener: 'new-plan', onclick: openCreate }
      : undefined}
  />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      {#if !manifest.isMultiTenant}
        <p class="prose">{t('app.instance.plans_single')}</p>
      {:else}
        <p class="prose">{t('app.instance.plans_intro')}</p>
        {#if enforced}
          <Banner tone="info" title={t('app.instance.source_file_enforced')}>{settings?.source ?? ''}</Banner>
        {/if}

        {#if reading.status === 'loading' || reading.status === 'idle'}
          <p class="quiet"><Spinner label={t('app.instance.reading')} /> <span>{t('app.instance.reading')}</span></p>
        {:else if unreadable}
          <Banner tone="danger" title={unreadable.message}>
            {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
          </Banner>
        {:else}
          {#if plans.all.length === 0}
            <Banner tone="info">{t('app.instance.plans_none')}</Banner>
          {:else}
            <Table label={t('app.instance.plans')} isLabelHidden {columns}>
              {#each plans.all as plan (plan.name)}
                <tr>
                  <td><a href={`/instance/plans/${encodeURIComponent(plan.name)}`}>{plan.display_name}</a></td>
                  <td><code>{plan.name}</code></td>
                  <td class="count">{plan.workspaces}</td>
                  <td>{leftOutWords(plan)}</td>
                </tr>
              {/each}
            </Table>
          {/if}
          {#if instance.workspacesState.status === 'ready'}
            <p class="quiet" data-testid="without-plan">{t('app.instance.plans_without', { count: withoutPlan })}</p>
          {/if}
        {/if}
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<Dialog
  title={t('app.instance.plan_create_title')}
  isOpen={isCreating}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (isCreating = false)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isCreating = false)}>{t('app.instance.cancel')}</Button>
    <Button
      tone="primary"
      isBusy={isWorking}
      busyLabel={t('app.instance.working')}
      disabledReason={name.trim() === '' || displayName.trim() === '' ? t('app.instance.plan_create_needs') : undefined}
      onclick={() => void create()}
    >
      {t('app.instance.plan_create')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure && !failure.fields.size}
      <Banner tone="danger" title={failure.message} />
    {/if}
    <Input
      label={t('app.instance.plan_display_name')}
      bind:value={displayName}
      error={failure?.fields.get('/display_name')}
      isRequired
    />
    <Input
      label={t('app.instance.plan_short_name')}
      hint={t('app.instance.plan_short_name_hint')}
      bind:value={name}
      error={failure?.fields.get('/name')}
      isRequired
    />
  </Stack>
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); display: flex; align-items: center; gap: var(--sp-100); }

  .count { font-variant-numeric: tabular-nums; text-align: end; }

  code { font-family: var(--font-mono); font-size: var(--fs-075); }
</style>
