<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's status and the steps the reader may take from it (UC-FLW-03 checks 1 and 3). Only
  // the steps the server offers are drawn; a step that asks for fields opens a small form for them
  // first (UC-FLW-04 check 3), and a refusal says, per field or check, what is still missing. Where
  // no workflow runs for the entry, nothing is drawn.

  import { tick, untrack } from 'svelte';
  import { Banner, Button, Dialog, Input, LabelChip, Stack } from '@hubtask/design-system/components';
  import type { EntrySteps, StepVerdict, WorkItem } from '@hubtask/sync-engine';

  import { customFields } from '../data/customfields.svelte.ts';
  import { fieldName } from '../data/customfields.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { statusLabel, workflows } from '../data/workflows.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
    /** Whether a workflow runs for the entry, for the head's marks to leave out what it replaces. */
    runs?: boolean;
  }

  let { item, runs = $bindable(false) }: Props = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<EntrySteps>(untrack(() => `/items/${item.id}/steps`));
  $effect(() => untrack(() => workflows.open()));

  const answer = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const workflow = $derived(workflows.list?.data.find((each) => each.key === answer?.workflow));
  const status = $derived(workflow?.published.statuses.find((each) => each.key === answer?.status));
  $effect(() => {
    runs = Boolean(answer?.workflow && status);
  });

  let refusal = $state<{ message: string; details: readonly string[]; fields: ReadonlyMap<string, string> } | undefined>(undefined);
  let askForm = $state<HTMLElement | undefined>(undefined);

  /** Where the server names an asked field, its error is drawn at that field (§5.10). */
  const fieldPath = (key: string) => `/custom_fields/${key}`;

  /** What the refusal says that no field of the small form shows: other fields, failing checks. */
  function notAtAField(said: { details: readonly string[]; fields: ReadonlyMap<string, string> }, step: StepVerdict): string[] {
    const shown = new Set(askedFields(step).map(fieldPath));
    const atFields = [...said.fields.values()];
    return [
      ...[...said.fields].filter(([path]) => !shown.has(path)).map(([, words]) => words),
      ...said.details.filter((words) => !atFields.includes(words)),
    ];
  }
  let isTaking = $state(false);

  function stepName(step: StepVerdict): string {
    if (step.name) return step.name;
    const target = workflow?.published.statuses.find((each) => each.key === step.to);
    return target ? statusLabel(target, t) : step.to;
  }

  /** The asked fields the step's small form shows: the custom fields among them. */
  function askedFields(step: StepVerdict): readonly string[] {
    return step.asks.filter((key) => key.startsWith('custom_fields.')).map((key) => key.slice('custom_fields.'.length));
  }

  let asking = $state<StepVerdict | undefined>(undefined);
  let values = $state<Record<string, string>>({});

  function choose(step: StepVerdict) {
    refusal = undefined;
    const asked = askedFields(step);
    if (asked.length > 0) {
      values = Object.fromEntries(asked.map((key) => [key, String(item.custom_fields?.[key] ?? '')]));
      asking = step;
      return;
    }
    void take(step);
  }

  async function take(step: StepVerdict) {
    isTaking = true;
    try {
      let version = item.version;
      for (const [key, value] of Object.entries(values)) {
        if (String(item.custom_fields?.[key] ?? '') === value) continue;
        const written = await customFields.setValue(item.id, key, value === '' ? null : value, version);
        version = written.version;
      }
      await workflows.take({ id: item.id, version }, { step: step.key });
      asking = undefined;
      values = {};
      toast.show({ text: t('app.workflows.step_taken', { name: stepName(step) }), tone: 'success' });
    } catch (error) {
      const rendered = renderProblem(error as never, messages);
      refusal = { message: rendered.message, details: [...rendered.fields.values(), ...(rendered.wholes ?? [])], fields: rendered.fields };
      if (asking) {
        // Focus goes to the first asked field the refusal names (WCAG 3.3.1).
        await tick();
        askForm?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
      }
    } finally {
      isTaking = false;
    }
  }
</script>

{#if answer?.workflow && status}
  <section class="steps" aria-label={t('app.workflows.status_section')} data-steps>
    <p class="status">
      <span data-status={status.key}><LabelChip name={statusLabel(status, t)} colorToken={status.color_token} /></span>
    </p>
    {#if answer.steps.length > 0}
      <div class="actions" role="group" aria-label={t('app.workflows.steps_label')}>
        {#each answer.steps as step (step.key)}
          <Button tone="secondary" size="sm" data-step={step.key} isBusy={isTaking} onclick={() => choose(step)}>
            {stepName(step)}
          </Button>
        {/each}
      </div>
    {/if}
    {#if refusal && !asking}
      <div class="refusal" role="alert">
        <p>{refusal.message}</p>
        {#if refusal.details.length > 0}
          <ul>{#each refusal.details as detail (detail)}<li>{detail}</li>{/each}</ul>
        {/if}
      </div>
    {/if}
  </section>
{/if}

{#if asking}
  {@const asked = asking}
  <Dialog size="s"
    isOpen={true}
    title={t('app.workflows.step_asks', { name: stepName(asked) })}
    dismissLabel={t('app.workflows.cancel')}
    onClose={() => { asking = undefined; }}
  >
    <div bind:this={askForm}><Stack gap="200">
      {#each askedFields(asked) as key (key)}
        <Input label={fieldName(customFields.of(item.collection_id).find((each) => each.key === key), key)} bind:value={values[key]} error={refusal?.fields.get(fieldPath(key))} />
      {/each}
      {#if refusal}
        {@const rest = notAtAField(refusal, asked)}
        <Banner tone="danger">
          {refusal.message}
          {#if rest.length > 0}<ul class="rest">{#each rest as words (words)}<li>{words}</li>{/each}</ul>{/if}
        </Banner>
      {/if}
    </Stack></div>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (asking = undefined)}>{t('app.workflows.cancel')}</Button>
      <Button tone="primary" data-confirm-step isBusy={isTaking} onclick={() => take(asked)}>
        {stepName(asked)}
      </Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  /* In the entry's head (user-experience.md §5.6): the status, then the steps from it, on one line
     where they fit; a refusal takes a line of its own under them. */
  .steps { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .status { margin: 0; }

  .steps > .refusal { flex-basis: 100%; }

  .rest { margin: 0; padding-inline-start: var(--sp-200); }

  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  .refusal { margin: 0; color: var(--text-danger); }

  .refusal p, .refusal ul { margin: 0; }
</style>
