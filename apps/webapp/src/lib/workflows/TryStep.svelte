<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Trying a step against a chosen entry before it is published (UC-FLW-04 check 5): whether the
  // person may take it, which required fields are empty and how each check comes out - and
  // nothing changes. The draft is tried where there is one, as saved.

  import { Button, Select, Stack } from '@hubtask/design-system/components';
  import type { StepVerdict, Workflow, WorkflowShape, WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from '../entries/EntryPicker.svelte';
  import { statusLabel, stepLabel, workflows } from '../data/workflows.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    workflow: Workflow;
    /** The shape tried: the saved draft, else what is in force. */
    shape: WorkflowShape;
    isDraft: boolean;
  }

  const { workflow, shape, isDraft }: Props = $props();

  let step = $state('');
  let entry = $state<WorkItem | undefined>(undefined);
  let verdict = $state<StepVerdict | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let isTrying = $state(false);

  const stepOptions = $derived(shape.steps.map((each) => ({ value: each.key, label: stepLabel(each, shape, t) })));
  const nameOf = (key: string | undefined) => {
    const status = shape.statuses.find((each) => each.key === key);
    return status ? statusLabel(status, t) : (key ?? '');
  };

  async function attempt() {
    if (!step || !entry) return;
    isTrying = true;
    refusal = undefined;
    verdict = undefined;
    try {
      verdict = await workflows.tryStep(workflow.key, step, entry.id, isDraft);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isTrying = false;
    }
  }
</script>

<section class="panel" aria-labelledby="workflow-try" data-try-step>
  <Stack gap="150">
    <h3 id="workflow-try" class="title">{t('app.workflow_designer.try_title')}</h3>
    <p class="quiet">{t('app.workflow_designer.try_intro')}</p>
    <Select label={t('app.workflow_designer.try_step')} options={stepOptions} placeholder={t('app.workflow_designer.try_choose')} bind:value={step} />
    <EntryPicker chosen={entry} onchoose={(chosen) => { entry = chosen; verdict = undefined; }} />
    <div>
      <Button tone="secondary" size="sm" data-try isBusy={isTrying}
        disabledReason={!step || !entry ? t('app.workflow_designer.try_choose') : undefined} onclick={attempt}>
        {t('app.workflow_designer.try')}
      </Button>
    </div>
    {#if verdict}
      <ul class="verdict" data-verdict={verdict.holds ? 'holds' : 'refused'}>
        <li>{verdict.holds ? t('app.workflow_designer.try_holds') : t('app.workflow_designer.try_refused')}</li>
        {#if verdict.from !== undefined}
          <li>{verdict.leaves_from
            ? t('app.workflow_designer.try_leaves', { status: nameOf(verdict.from) })
            : t('app.workflow_designer.try_not_from', { status: nameOf(verdict.from) })}</li>
        {/if}
        <li>{verdict.allowed ? t('app.workflow_designer.try_allowed') : t('app.workflow_designer.try_not_allowed')}</li>
        {#each verdict.missing as field (field)}
          <li>{t('app.workflow_designer.try_missing', { field })}</li>
        {/each}
        {#each verdict.checks as check (check.key)}
          <li>{check.holds
            ? t('app.workflow_designer.try_check_holds', { check: check.message ?? check.key })
            : t('app.workflow_designer.try_check_fails', { check: check.message ?? check.key })}</li>
        {/each}
      </ul>
    {/if}
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  </Stack>
</section>

<style>
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .title { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .verdict { margin: 0; padding-inline-start: var(--sp-300); display: grid; gap: var(--sp-050); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
