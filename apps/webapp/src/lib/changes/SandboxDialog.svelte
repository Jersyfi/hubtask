<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Sandbox…* as a Dialog L (page-system §9.3, configuration.md §3): a copy where an agent changes
  // freely and nothing runs; promoting it applies what it changed since its copy, by the plan shown
  // here with what it reaches.
  import { Button, Checkbox, Dialog, EmptyState, PlanSummary, Spinner, Stack } from '@hubtask/design-system/components';
  import type { ConfigSandbox, ResourceState } from '@hubtask/sync-engine';

  import { config, needsAcknowledge } from '../data/config.svelte.ts';
  import { planLines } from '../data/configtemplates.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import PlanReach from '../settings/PlanReach.svelte';

  interface Props {
    isOpen: boolean;
    sandbox: ResourceState<ConfigSandbox> | undefined;
    mayChange: boolean;
  }

  let { isOpen = $bindable(false), sandbox, mayChange }: Props = $props();

  const promotion = $derived(sandbox?.status === 'ready' && sandbox.data.exists ? sandbox.data.promotion : undefined);
  const refusal = $derived(sandbox?.status === 'failed' ? renderProblem(sandbox.error, messages) : undefined);
  let acknowledgePromotion = $state(false);
  let sandboxBusy = $state(false);
  const counts = (each: { added: number; changed: number; removed: number }) =>
    t('app.changes.counts', { added: each.added, changed: each.changed, removed: each.removed });

  async function sandboxStep(step: () => Promise<unknown>, done: string): Promise<void> {
    sandboxBusy = true;
    try {
      await step();
      toast.show({ text: t(done), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      sandboxBusy = false;
    }
  }
</script>

<Dialog
  size="l"
  title={t('app.changes.sandbox')}
  bind:isOpen
  dismissLabel={t('app.page.close')}
  primary={mayChange && promotion?.plan_hash && promotion.changes.length > 0
    ? {
        label: t('app.changes.sandbox_promote'),
        isBusy: sandboxBusy,
        disabledReason: needsAcknowledge(promotion.flags) && !acknowledgePromotion ? t('app.changes.acknowledge_first') : undefined,
        onclick: () => void sandboxStep(() => config.promote(promotion?.plan_hash ?? '', acknowledgePromotion), 'app.changes.sandbox_promoted'),
      }
    : undefined}
>
  <Stack gap="150">
    <p class="quiet" data-sandbox>{t('app.changes.sandbox_explain')}</p>
    {#if refusal}
      <FailureNote problem={refusal} />
    {:else if sandbox?.status !== 'ready'}
      <Spinner label={t('app.changes.reading')} />
    {:else if !sandbox.data.exists}
      <EmptyState kind="unused" title={t('app.changes.sandbox_none')} />
    {:else}
      <p class="note">{t('app.changes.sandbox_state', { writes: sandbox.data.writes ?? 0, at: formatDateTime(sandbox.data.updated_at ?? '', messages.locale) })}</p>
      {#if promotion && promotion.refusals.length > 0}
        <FailureNote problem={t('app.changes.sandbox_refused')}>
          <PlanSummary label={t('app.changes.plan_label')} lines={planLines(promotion, t)} emptyLabel={t('app.changes.plan_empty')} />
        </FailureNote>
      {:else if promotion && promotion.changes.length === 0}
        <p class="quiet">{t('app.changes.sandbox_same')}</p>
      {:else if promotion}
        <p class="quiet">{counts(promotion.counts)}</p>
        <PlanSummary label={t('app.changes.plan_label')} lines={planLines(promotion, t)} emptyLabel={t('app.changes.plan_empty')} />
        <PlanReach reach={promotion.reach} />
        {#if mayChange && needsAcknowledge(promotion.flags)}
          <Checkbox label={t('app.changes.acknowledge')} bind:checked={acknowledgePromotion} />
        {/if}
      {/if}
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.page.close')}</Button>
    {#if mayChange && sandbox?.status === 'ready'}
      {#if !sandbox.data.exists}
        <Button tone="secondary" isBusy={sandboxBusy} data-sandbox-copy onclick={() => sandboxStep(() => config.resetSandbox(false), 'app.changes.sandbox_copied')}>{t('app.changes.sandbox_copy')}</Button>
      {:else}
        <Button tone="subtle" data-sandbox-reset onclick={() => sandboxStep(() => config.resetSandbox(false), 'app.changes.sandbox_copied')}>{t('app.changes.sandbox_reset')}</Button>
        <Button tone="danger" data-sandbox-discard onclick={() => sandboxStep(() => config.resetSandbox(true), 'app.changes.sandbox_discarded')}>{t('app.changes.sandbox_discard')}</Button>
      {/if}
    {/if}
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .note { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
