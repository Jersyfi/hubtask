<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Moving a workspace to another plan, or to none (UC-INS-13 checks 4, 7; multi-tenancy.md §4.3):
  // the confirmation names what changes before anything does (P-04) - what rests from now, the
  // limits the workspace would be over, the exceptions that stay - in that order, from the server's
  // own dry run of the move, so the dialog says exactly what the platform would be answered.
  //
  // The plan it is on is not offered: a move there changes nothing. The move carries the version
  // the list read, so a workspace somebody changed meanwhile is refused rather than moved over.

  import { Banner, Button, Dialog, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { AdminTenant, TenantPlanMoved } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import { plans } from '../data/plans.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    tenant: AdminTenant | undefined;
    onclose: () => void;
  }

  const { tenant, onclose }: Props = $props();

  /** The empty choice is "no plan"; the plan the workspace is on is not a choice. */
  const NONE = '-';

  let chosen = $state('');
  let preview = $state<TenantPlanMoved | undefined>(undefined);
  let isPreviewing = $state(false);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** The dry run asked last: an answer to an earlier choice arrives too late to be shown. */
  let asked = 0;

  $effect(() => {
    if (tenant) {
      asked++;
      chosen = '';
      preview = undefined;
      failure = undefined;
    }
  });

  const options = $derived([
    ...plans.all.filter((plan) => plan.name !== tenant?.plan).map((plan) => ({ value: plan.name, label: plan.display_name })),
    ...(tenant?.plan ? [{ value: NONE, label: t('app.instance.plan_none') }] : []),
  ]);
  const target = $derived(chosen === NONE ? null : chosen);
  const targetName = $derived(chosen === NONE ? t('app.instance.plan_none') : (plans.named(chosen)?.display_name ?? chosen));

  function problem(cause: unknown): ReturnType<typeof renderProblem> {
    return cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }

  async function choose(value: string): Promise<void> {
    const ask = ++asked;
    chosen = value;
    preview = undefined;
    failure = undefined;
    if (!tenant || value === '') return;
    isPreviewing = true;
    try {
      const answer = await plans.move(tenant.id, value === NONE ? null : value, tenant.version, true);
      if (ask === asked) preview = answer;
    } catch (cause) {
      if (ask === asked) failure = problem(cause);
    } finally {
      if (ask === asked) isPreviewing = false;
    }
  }

  async function confirm(): Promise<void> {
    if (!tenant || chosen === '') return;
    isWorking = true;
    failure = undefined;
    try {
      await plans.move(tenant.id, target, tenant.version, false);
      onclose();
    } catch (cause) {
      // The preview no longer stands for what would happen: a confirm offered again would repeat
      // the same refusal.
      preview = undefined;
      failure = problem(cause);
    } finally {
      isWorking = false;
    }
  }

  const quotaName = (quota: string): string =>
    messages.has(`app.quotas.name.${quota}`) ? t(`app.quotas.name.${quota}`) : quota;
  const resting = $derived((preview?.features_resting ?? []).map((feature) => t(`app.instance.feature.${feature.toLowerCase()}`)));
  const exceeded = $derived(
    (preview?.limits_exceeded ?? []).map((over) =>
      t('app.instance.move_exceeded_one', { name: quotaName(over.quota), used: String(over.used), limit: String(over.limit) }),
    ),
  );
  const kept = $derived((preview?.exceptions_kept ?? []).map(quotaName));
</script>

<Dialog
  title={chosen === ''
    ? t('app.instance.move_choose_title', { name: tenant?.display_name ?? '' })
    : t('app.instance.move_title', { name: tenant?.display_name ?? '', plan: targetName })}
  isOpen={tenant !== undefined}
  dismissLabel={t('app.instance.cancel')}
  onClose={onclose}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={onclose}>{t('app.instance.cancel')}</Button>
    {#if preview}
      <Button tone="primary" isBusy={isWorking} busyLabel={t('app.instance.working')} onclick={() => void confirm()}>
        {target === null ? t('app.instance.move_confirm_none') : t('app.instance.move_confirm', { plan: targetName })}
      </Button>
    {/if}
  {/snippet}
  <Stack gap="150">
    <Select
      label={t('app.instance.move_choose')}
      placeholder={t('app.instance.move_choose_placeholder')}
      {options}
      value={chosen}
      onchange={(event) => void choose((event.currentTarget as HTMLSelectElement).value)}
    />
    {#if failure}
      <Banner tone="danger" title={failure.message} />
    {:else if isPreviewing}
      <p class="quiet"><Spinner label={t('app.instance.move_reading')} /> <span>{t('app.instance.move_reading')}</span></p>
    {:else if preview}
      <ul class="changes" data-testid="move-changes">
        {#if resting.length > 0}
          <li><strong>{t('app.instance.move_resting_label')}</strong> {t('app.instance.move_resting', { features: resting.join(', ') })}</li>
        {/if}
        {#if exceeded.length > 0}
          <li><strong>{t('app.instance.move_exceeded_label')}</strong> {t('app.instance.move_exceeded', { limits: exceeded.join(', ') })}</li>
        {/if}
        {#if kept.length > 0}
          <li><strong>{t('app.instance.move_kept_label')}</strong> {t('app.instance.move_kept', { quotas: kept.join(', ') })}</li>
        {/if}
        {#if resting.length === 0 && exceeded.length === 0 && kept.length === 0}
          <li>{t('app.instance.move_nothing')}</li>
        {/if}
      </ul>
    {/if}
  </Stack>
</Dialog>

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .changes { margin: 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-050); }
</style>
