<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's status as a button with where it can go next (E-23, UC-FLW-06, UC-WRK-44): the
  // allowed steps with their target and what they ask, the blocked ones with their reason as
  // information (P-05), how done is reached, and *View workflow* read only for every reader; *Open
  // in designer* only with the right to the structure. Without a workflow the default holds and
  // the next step is *Done*, which completes the entry. A step that asks for fields opens a small
  // form first (UC-FLW-04 check 3). It replaces the row of step buttons (StepControl).
  import { tick, untrack } from 'svelte';
  import { Banner, Button, Dialog, Input, Stack, StateLozenge, StatusButton } from '@hubtask/design-system/components';
  import type { WorkflowStep } from '@hubtask/design-system/components';
  import type { EntrySteps, StepVerdict, WorkItem } from '@hubtask/sync-engine';
  import { accounts } from '../data/accounts.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { fieldName } from '../data/customfields.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { workflows } from '../data/workflows.svelte.ts';
  import { statusLabel } from '../data/workflows.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { roleLabel } from '../people/rolewords.ts';
  import { renderProblem } from '../problem.ts';
  import { categoryOf, readSteps } from './entrysteps.ts';

  interface Props {
    item: WorkItem;
    /** Whether a workflow runs for the entry. */
    runs?: boolean;
    /** The head's primary act: the workflow's next step, or *Done* without one. */
    next?: { label: string; icon?: 'check'; onclick: () => void };
    /** Completing or reopening without a workflow: the same write as the row's checkbox. */
    oncomplete: () => void;
    /** Present: the reader may not change the entry now, and this says why. */
    disabledReason?: string;
    onnavigate?: (path: string) => void;
  }

  let { item, runs = $bindable(false), next = $bindable(undefined), oncomplete, disabledReason, onnavigate }: Props = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<EntrySteps>(untrack(() => `/items/${item.id}/steps`));
  const answer = $derived(read.state.status === 'ready' ? read.state.data : undefined);

  function asksLine(step: StepVerdict): string | undefined {
    const fields = step.asks
      .filter((key) => key.startsWith('custom_fields.'))
      .map((key) => key.slice('custom_fields.'.length))
      .map((key) => fieldName(customFields.of(item.collection_id).find((each) => each.key === key), key));
    const parts = [
      ...(fields.length > 0 ? [t('app.workflows.asks', { fields: fields.join(', ') })] : []),
      ...((step.approval_levels ?? 0) > 0 ? [t('app.workflows.needs_approval')] : []),
    ];
    return parts.length > 0 ? parts.join(' · ') : undefined;
  }

  const model = $derived(
    answer
      ? readSteps(answer, item.completion?.is_completed ?? false, {
          status: (status) => statusLabel(status, t),
          asks: asksLine,
          reason: (step) => {
            if (step.reason !== 'workflows.step_who_unmet') return t('app.workflows.blocked_condition');
            const who = [
              ...step.roles.map((role: string) => roleLabel(role)),
              ...step.people.map((id: string) => accounts.nameOf(id) ?? t('app.people.unnamed')),
              ...step.groups.map((id: string) => groups.nameOf(id) ?? t('app.people.unnamed')),
            ];
            return who.length > 0 ? t('app.workflows.blocked_who', { who: who.join(', ') }) : t('app.workflows.blocked_permission');
          },
          doneRoute: (names) => t('app.workflows.done_route', { steps: names.join(' → ') }),
        })
      : undefined,
  );
  const verdicts = $derived(new Map((answer?.steps ?? []).map((step) => [step.key, step])));

  let isTaking = $state(false);
  let refusal = $state<{ message: string; details: readonly string[]; fields: ReadonlyMap<string, string> } | undefined>(undefined);
  let asking = $state<StepVerdict | undefined>(undefined);
  let values = $state<Record<string, string>>({});
  let askForm = $state<HTMLElement | undefined>(undefined);
  let isPictureOpen = $state(false);

  const fieldPath = (key: string) => `/custom_fields/${key}`;
  const askedFields = (step: StepVerdict) =>
    step.asks.filter((key) => key.startsWith('custom_fields.')).map((key) => key.slice('custom_fields.'.length));

  /** What the refusal says that no field of the small form shows: other fields, failing checks. */
  function notAtAField(said: { details: readonly string[]; fields: ReadonlyMap<string, string> }, step: StepVerdict): string[] {
    const shown = new Set(askedFields(step).map(fieldPath));
    const atFields = [...said.fields.values()];
    return [
      ...[...said.fields].filter(([path]) => !shown.has(path)).map(([, words]) => words),
      ...said.details.filter((words) => !atFields.includes(words)),
    ];
  }

  function choose(step: WorkflowStep) {
    refusal = undefined;
    if (model?.byDefault) {
      oncomplete();
      return;
    }
    const verdict = verdicts.get(step.id);
    if (!verdict) return;
    const asked = askedFields(verdict);
    if (asked.length > 0) {
      values = Object.fromEntries(asked.map((key) => [key, String(item.custom_fields?.[key] ?? '')]));
      asking = verdict;
      return;
    }
    void take(verdict, step.label);
  }

  async function take(step: StepVerdict, name: string) {
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
      toast.show({ text: t('app.workflows.step_taken', { name }), tone: 'success' });
      void read.refresh();
    } catch (error) {
      const rendered = renderProblem(error as never, messages);
      refusal = { message: rendered.message, details: [...rendered.fields.values(), ...(rendered.wholes ?? [])], fields: rendered.fields };
      if (asking) {
        await tick();
        askForm?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
      }
    } finally {
      isTaking = false;
    }
  }

  // The status moves when the entry does (another tab, a board drop): ask again.
  let seenVersion = untrack(() => item.version);
  $effect(() => {
    if (item.version === seenVersion) return;
    seenVersion = item.version;
    untrack(() => void read.refresh());
  });

  $effect(() => {
    runs = Boolean(model && !model.byDefault);
  });

  $effect(() => {
    if (!model || disabledReason !== undefined || answer?.steps === undefined) {
      next = undefined;
      return;
    }
    if (model.byDefault) {
      next = item.completion?.is_completed ? undefined : { label: t('app.workflows.complete'), icon: 'check', onclick: () => oncomplete() };
      return;
    }
    const step = model.steps.find((each) => each.isNext && each.blockedReason === undefined)
      ?? model.steps.find((each) => each.blockedReason === undefined && each.to.category !== 'todo' && each.to.category !== model.current.category);
    next = step ? { label: step.label, onclick: () => choose(step) } : undefined;
  });

  const ORDER = ['BACKLOG', 'TODO', 'IN_PROGRESS', 'DONE'] as const;
  const reachable = $derived(new Set((model?.steps ?? []).filter((step) => step.blockedReason === undefined).map((step) => step.to.id)));
  const pictureName = (key: string) => {
    const status = answer?.picture?.statuses.find((each) => each.key === key);
    return status ? statusLabel(status, t) : key;
  };
</script>

{#if model && answer}
  <span class="status" data-steps data-status={model.current.id}>
    <StatusButton
      current={model.current}
      steps={model.steps}
      statuses={model.statuses}
      label={t('app.workflows.status_button', { status: model.current.label })}
      menuTitle={model.byDefault ? t('app.workflows.default_title') : t('app.workflows.from_status', { status: model.current.label })}
      doneNote={t('app.workflows.done_note')}
      doneHint={model.doneHint}
      workflow={answer.picture && !model.byDefault ? { label: t('app.workflows.view'), onview: () => (isPictureOpen = true) } : undefined}
      {disabledReason}
      onstep={choose}
      closeLabel={t('app.workflows.close')}
      grabLabel={t('app.workflows.close')}
    />
  </span>
  {#if refusal && !asking}
    <div class="refusal" role="alert">
      <p>{refusal.message}</p>
      {#if refusal.details.length > 0}<ul>{#each refusal.details as detail (detail)}<li>{detail}</li>{/each}</ul>{/if}
    </div>
  {/if}
{/if}

{#if asking}
  {@const asked = asking}
  {@const askedName = model?.steps.find((each) => each.id === asked.key)?.label ?? asked.key}
  <Dialog size="s" isOpen={true} title={t('app.workflows.step_asks', { name: askedName })} dismissLabel={t('app.workflows.cancel')} onClose={() => (asking = undefined)}>
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
      <Button tone="primary" data-confirm-step isBusy={isTaking} onclick={() => take(asked, askedName)}>{askedName}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isPictureOpen && answer?.picture && model}
  {@const picture = answer.picture}
  <!-- The designer's picture, read only (E-23): one column per category, the entry's status
       outlined, the targets it can reach now marked, each step with who may and what it waits for. -->
  <Dialog size="l" isOpen={true} title={t('app.workflows.picture_title')} dismissLabel={t('app.workflows.close')} onClose={() => (isPictureOpen = false)}>
    <div class="picture" data-workflow-picture>
      {#each ORDER.filter((category) => picture.statuses.some((each) => each.category === category)) as category (category)}
        <ol class="column" aria-label={t(`workflows.category.${category.toLowerCase()}`)}>
          {#each picture.statuses.filter((each) => each.category === category) as status (status.key)}
            <li class="node" data-current={status.key === model.current.id ? '' : undefined} data-reachable={reachable.has(status.key) ? '' : undefined}>
              <StateLozenge label={statusLabel(status, t)} vocabulary="category" state={categoryOf(status.category)} size="sm" />
              {#if status.key === model.current.id}<span class="here">{t('app.workflows.picture_here')}</span>{/if}
            </li>
          {/each}
        </ol>
      {/each}
    </div>
    <ul class="edges">
      {#each picture.steps as step (step.key)}
        <li data-reachable={reachable.has(step.to) && (step.from.length === 0 || step.from.includes(model.current.id)) ? '' : undefined}>
          <span class="edge-name">{step.name ?? pictureName(step.to)}</span>
          <span class="edge-way">{step.from.length === 0 ? t('app.workflows.picture_any') : step.from.map(pictureName).join(', ')} → {pictureName(step.to)}</span>
          {#if step.limited}<span class="edge-mark">{t('app.workflows.picture_limited')}</span>{/if}
          {#if step.approval}<span class="edge-mark">{t('app.workflows.needs_approval')}</span>{/if}
        </li>
      {/each}
    </ul>
    {#snippet actions()}
      {#if answer.may_design && answer.workflow}
        {@const key = answer.workflow}
        <Button tone="secondary" onclick={() => { isPictureOpen = false; onnavigate?.(`/administration/workflows/${key}`); }}>{t('app.workflows.open_designer')}</Button>
      {/if}
      <Button tone="primary" onclick={() => (isPictureOpen = false)}>{t('app.workflows.close')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .status { display: inline-flex; }
  .refusal { margin: 0; color: var(--text-danger); flex-basis: 100%; }
  .refusal p, .refusal ul { margin: 0; }
  .rest { margin: 0; padding-inline-start: var(--sp-200); }
  .picture { display: grid; grid-auto-flow: column; grid-auto-columns: minmax(0, 1fr); gap: var(--sp-200); }
  .column { display: flex; flex-direction: column; gap: var(--sp-100); margin: 0; padding: var(--sp-100); list-style: none; border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .node { display: flex; flex-direction: column; align-items: flex-start; gap: var(--sp-025); padding: var(--sp-050); border: var(--bw-hairline) dashed var(--border-subtle); border-radius: var(--r-sm); background: var(--bg-surface); }
  .node[data-reachable] { border-style: solid; border-color: var(--accent-primary); }
  .node[data-current] { border: var(--bw-thick) solid var(--text-primary); }
  .here { font-size: var(--fs-075); color: var(--text-subtle); }
  .edges { display: flex; flex-direction: column; gap: var(--sp-050); margin: var(--sp-200) 0 0; padding: 0; list-style: none; font-size: var(--fs-075); }
  .edges li { display: flex; flex-wrap: wrap; gap: var(--sp-100); color: var(--text-subtle); }
  .edges li[data-reachable] { color: var(--text-primary); }
  .edge-name { font-weight: var(--fw-medium); color: var(--text-primary); }
  .edge-mark { padding-inline: var(--sp-050); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-sm); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) { .picture { grid-auto-flow: row; } }
</style>
