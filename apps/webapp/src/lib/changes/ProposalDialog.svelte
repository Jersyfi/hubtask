<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An agent's proposal as a Dialog M (page-system §9.3): what it changes, planned afresh against
  // the configuration in force as the reader and writing nothing (configuration.md §3), what it
  // reaches, and *Apply* only once what it hides is acknowledged.
  import { Button, Checkbox, Dialog, PlanSummary, Spinner, Stack } from '@hubtask/design-system/components';
  import type { ConfigPlan, ConfigProposal } from '@hubtask/sync-engine';

  import { config, needsAcknowledge } from '../data/config.svelte.ts';
  import { planLines } from '../data/configtemplates.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import PlanReach from '../settings/PlanReach.svelte';

  interface Props {
    proposal: ConfigProposal | undefined;
    mayChange: boolean;
    templateName: (provenance: string) => string;
    onClose: () => void;
  }

  const { proposal, mayChange, templateName, onClose }: Props = $props();

  let planned = $state<ConfigPlan>();
  let planFailure = $state<string>();
  let acknowledged = $state(false);
  let busy = $state(false);

  $effect(() => {
    const current = proposal;
    planned = undefined;
    planFailure = undefined;
    acknowledged = false;
    if (!current) return;
    config.planProposal(current).then(
      (plan) => { if (proposal === current) planned = plan; },
      (error) => { if (proposal === current) planFailure = renderProblem(error as never, messages).message; },
    );
  });

  const counts = (each: { added: number; changed: number; removed: number }) =>
    t('app.changes.counts', { added: each.added, changed: each.changed, removed: each.removed });

  async function decide(decision: 'APPLY' | 'DECLINE'): Promise<void> {
    if (!proposal) return;
    busy = true;
    try {
      await config.decide(proposal.id, decision, acknowledged);
      toast.show({ text: t(decision === 'APPLY' ? 'app.changes.applied' : 'app.changes.declined'), tone: 'success' });
      onClose();
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      busy = false;
    }
  }
</script>

<Dialog
  size="m"
  isOpen={proposal !== undefined}
  title={proposal ? t('app.changes.proposal', { at: formatDateTime(proposal.created_at, messages.locale) }) : ''}
  dismissLabel={t('app.changes.cancel')}
  {onClose}
  primary={proposal && mayChange
    ? {
        label: t('app.changes.apply'),
        isBusy: busy,
        disabledReason: needsAcknowledge(proposal) && !acknowledged ? t('app.changes.acknowledge_first') : undefined,
        onclick: () => void decide('APPLY'),
      }
    : undefined}
>
  {#if proposal}
    <Stack gap="150">
      <p class="quiet" data-proposal={proposal.id}>
        {counts(proposal)}
        {#if proposal.template}· {t('app.changes.from_template', { name: templateName(proposal.template) })}{/if}
        {#if proposal.hides_data}· <span class="flag">{t('app.changes.hides_data')}</span>{/if}
        {#if proposal.changes_permissions}· <span class="flag">{t('app.changes.changes_permissions')}</span>{/if}
      </p>
      {#if planFailure}
        <FailureNote problem={planFailure} />
      {:else if !planned}
        <p class="quiet"><Spinner label={t('app.changes.reading')} /></p>
      {:else if planned.refusals.length > 0}
        <FailureNote problem={t('app.changes.preview_refused')}>
          <PlanSummary label={t('app.changes.plan_label')} lines={planLines({ changes: [], refusals: planned.refusals }, t)} emptyLabel={t('app.changes.plan_empty')} />
        </FailureNote>
      {:else}
        <PlanSummary label={t('app.changes.plan_label')} lines={planLines(planned, t)} emptyLabel={t('app.changes.plan_empty')} />
        <PlanReach reach={planned.reach} />
      {/if}
      {#if mayChange && needsAcknowledge(proposal)}
        <Checkbox label={t('app.changes.acknowledge')} bind:checked={acknowledged} />
      {/if}
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.changes.cancel')}</Button>
    {#if mayChange}
      <Button tone="danger" data-decline={proposal?.id} isBusy={busy} onclick={() => void decide('DECLINE')}>{t('app.changes.decline')}</Button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .flag { color: var(--text-danger); }
</style>
