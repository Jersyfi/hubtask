<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Roll back…* as a Dialog M: **a rollback is a plan first** - what it changes, what it reaches
  // and what must be acknowledged - and applies exactly that plan by its hash.
  import { Button, Checkbox, Dialog, PlanSummary, Spinner, Stack } from '@hubtask/design-system/components';
  import type { ConfigPlan } from '@hubtask/sync-engine';

  import { config, needsAcknowledge } from '../data/config.svelte.ts';
  import { planLines } from '../data/configtemplates.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import PlanReach from '../settings/PlanReach.svelte';
  import { countsOf } from './words.ts';

  interface Props {
    /** The version to return to; closed while undefined. */
    version: number | undefined;
    onClose: () => void;
  }

  const { version, onClose }: Props = $props();

  let plan = $state<ConfigPlan>();
  let acknowledged = $state(false);
  let failure = $state<string>();
  let busy = $state(false);

  $effect(() => {
    const asked = version;
    plan = undefined;
    failure = undefined;
    acknowledged = false;
    if (asked === undefined) return;
    config.planRollback(asked).then(
      (next) => { if (version === asked) plan = next; },
      (error) => { if (version === asked) failure = renderProblem(error as never, messages).message; },
    );
  });

  async function confirm(): Promise<void> {
    if (version === undefined || !plan?.plan_hash) return;
    busy = true;
    try {
      await config.rollback(version, plan.plan_hash, acknowledged);
      toast.show({ text: t('app.changes.rolled_back', { version }), tone: 'success' });
      onClose();
    } catch (error) {
      // Said inside the dialog, which stays open on the plan (user-experience.md §5.10).
      failure = renderProblem(error as never, messages).message;
    } finally {
      busy = false;
    }
  }
</script>

<Dialog
  size="m"
  isOpen={version !== undefined}
  title={version !== undefined ? t('app.changes.rollback_title', { version }) : ''}
  dismissLabel={t('app.changes.cancel')}
  {onClose}
  primary={plan?.plan_hash
    ? {
        label: t('app.changes.roll_back'),
        isBusy: busy,
        disabledReason: needsAcknowledge(plan.flags) && !acknowledged ? t('app.changes.acknowledge_first') : undefined,
        onclick: () => void confirm(),
      }
    : undefined}
>
  <Stack gap="150">
    {#if !plan && !failure}
      <Spinner label={t('app.changes.reading')} />
    {:else if plan && plan.refusals.length > 0}
      <FailureNote problem={t('app.changes.rollback_refused')}>
        <PlanSummary label={t('app.changes.plan_label')} lines={planLines({ changes: [], refusals: plan.refusals }, t)} emptyLabel={t('app.changes.plan_empty')} />
      </FailureNote>
    {:else if plan}
      <p class="quiet">{countsOf(plan.counts)}</p>
      <PlanSummary label={t('app.changes.plan_label')} lines={planLines(plan, t)} emptyLabel={t('app.changes.plan_empty')} />
      <PlanReach reach={plan.reach} />
      {#if needsAcknowledge(plan.flags)}<Checkbox label={t('app.changes.acknowledge')} bind:checked={acknowledged} />{/if}
    {/if}
    {#if failure}<FailureNote problem={failure} />{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.changes.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
