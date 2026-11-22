<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's status as a button with where it can go next (E-23, UC-FLW-06, UC-WRK-44): the
  // allowed steps with their target and what they ask, the blocked ones with their reason as
  // information (P-05), how done is reached, and *View workflow* read only for every reader; *Open
  // in designer* only with the right to the structure. Without a workflow the level holds: open and
  // done in a list, the board's columns on a board (UC-FLW-06 check 5); the next step is *Done*. A step that asks for fields opens a small
  // form first (UC-FLW-04 check 3). It replaces the row of step buttons (StepControl).
  import { tick, untrack } from 'svelte';
  import { Banner, Button, Dialog, Input, Stack, StatusButton } from '@hubtask/design-system/components';
  import type { WorkflowStep } from '@hubtask/design-system/components';
  import type { EntrySteps, ResourceState, StepVerdict, WorkItem } from '@hubtask/sync-engine';
  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { fieldName } from '../data/customfields.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { modules } from '../data/modules.svelte.ts';
  import { workflows } from '../data/workflows.svelte.ts';
  import { statusLabel } from '../data/workflows.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { roleLabel } from '../people/rolewords.ts';
  import { renderProblem } from '../problem.ts';
  import { defaultSteps, readSteps, stepsAskable } from './entrysteps.ts';
  import WorkflowPicture from './WorkflowPicture.svelte';

  interface Props {
    item: WorkItem;
    /** Whether a workflow runs for the entry. */
    runs?: boolean;
    /** The head's primary act: the workflow's next step, or *Done* without one. */
    next?: { label: string; icon?: 'check'; onclick: () => void };
    /**
     * A second step beside it where `offersSecond` asks for one (a ticket's *Wait for the
     * requester*): the first other allowed step that neither finishes nor goes back.
     */
    second?: { label: string; onclick: () => void };
    offersSecond?: boolean;
    /** Completing or reopening without a workflow: the same write as the row's checkbox. */
    oncomplete: () => void;
    /** Present: the reader may not change the entry now, and this says why. */
    disabledReason?: string;
    /** The id of the page's notice that says the reason already. */
    reasonSaidAt?: string;
    onnavigate?: (path: string) => void;
  }

  let { item, runs = $bindable(false), next = $bindable(undefined), second = $bindable(undefined), offersSecond = false, oncomplete, disabledReason, reasonSaidAt, onnavigate }: Props = $props();

  // Keyed on the entry by its caller, so the entry's id is fixed for its life. Where workflows are off
  // the steps are not asked (the server refuses them) and the default flow holds.
  const itemId = untrack(() => item.id);
  let placeOn = $state<readonly string[] | null | undefined>(undefined);
  // The entry moved to another collection keeps its id, and so this component: its place is read again.
  $effect(() => {
    const collectionId = item.collection_id;
    return untrack(() => {
      placeOn = undefined;
      return modules.here(collectionId, (next) => {
        if (next.status === 'ready') placeOn = next.data.modules_on;
        else if (next.status === 'failed') placeOn = null;
      });
    });
  });
  const askable = $derived(stepsAskable(manifest.value?.modules_on, placeOn));
  let stepsState = $state<ResourceState<EntrySteps> | undefined>(undefined);
  $effect(() => {
    if (askable !== true) return;
    return untrack(() => workflows.steps(itemId, (next) => (stepsState = next)));
  });
  const refreshSteps = () => (askable === true ? workflows.refreshSteps(itemId) : Promise.resolve());
  const answer = $derived(askable === true && stepsState?.status === 'ready' ? stepsState.data : undefined);

  /** A form key a step names (`due_date`, `custom_fields.<key>`) in the reader's words. */
  function formKeyName(key: string): string {
    if (key === 'due_date') return t('app.entry_forms.own_due_date');
    if (!key.startsWith('custom_fields.')) return key;
    const field = key.slice('custom_fields.'.length);
    return fieldName(customFields.of(item.collection_id).find((each) => each.key === field), field);
  }

  // What a step asks for in its small form, what it needs filled (✓ where it is), and whether it
  // waits for an approval: one line under the step in the menu and on its card in the picture.
  function asksLine(step: Pick<StepVerdict, 'asks' | 'requires' | 'missing' | 'approval_levels'>): string | undefined {
    const asked = step.asks.filter((key) => key.startsWith('custom_fields.')).map(formKeyName);
    const missing = new Set(step.missing ?? []);
    const needed = (step.requires ?? []).map((key) =>
      missing.has(key) ? formKeyName(key) : t('app.workflows.requires_met', { field: formKeyName(key) }));
    const parts = [
      ...(asked.length > 0 ? [t('app.workflows.asks', { fields: asked.join(', ') })] : []),
      ...(needed.length > 0 ? [t('app.workflows.requires', { fields: needed.join(', ') })] : []),
      ...((step.approval_levels ?? 0) > 0 ? [t('app.workflows.needs_approval')] : []),
    ];
    return parts.length > 0 ? parts.join(' · ') : undefined;
  }

  type BlockedStep = NonNullable<EntrySteps['blocked']>[number];
  function blockedReason(step: BlockedStep): string {
    if (step.reason !== 'workflows.step_who_unmet') return t('app.workflows.blocked_condition');
    const who = [
      ...step.roles.map((role: string) => roleLabel(role)),
      ...step.people.map((id: string) => accounts.nameOf(id) ?? t('app.people.unnamed')),
      ...step.groups.map((id: string) => groups.nameOf(id) ?? t('app.people.unnamed')),
    ];
    return who.length > 0 ? t('app.workflows.blocked_who', { who: who.join(', ') }) : t('app.workflows.blocked_permission');
  }

  const model = $derived(
    answer
      ? readSteps(answer, item.completion?.is_completed ?? false, {
          status: (status) => statusLabel(status, t),
          asks: asksLine,
          reason: blockedReason,
          doneRoute: (names) => t('app.workflows.done_route', { steps: names.join(' → ') }),
        })
      : askable === false
        ? defaultSteps(item.completion?.is_completed ?? false, { status: (status) => statusLabel(status, t) })
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
    if (model?.columns) {
      void moveInto(step);
      return;
    }
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

  // A column move is the board's drop (`:drop`): the server completes into the done column and
  // reopens out of it, so the client never couples completion to the column.
  async function moveInto(step: WorkflowStep) {
    isTaking = true;
    try {
      await items.drop(item.id, step.to.id, item.version);
      toast.show({ text: t('app.workflows.step_taken', { name: step.label }), tone: 'success' });
      void refreshSteps();
    } catch (error) {
      const rendered = renderProblem(error as never, messages);
      refusal = { message: rendered.message, details: [...rendered.fields.values(), ...(rendered.wholes ?? [])], fields: rendered.fields };
    } finally {
      isTaking = false;
    }
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
      void refreshSteps();
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
    untrack(() => void refreshSteps());
  });

  $effect(() => {
    runs = Boolean(model && !model.byDefault);
  });

  $effect(() => {
    second = undefined;
    if (!model || disabledReason !== undefined || (askable !== false && answer?.steps === undefined)) {
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
    const other = offersSecond && !model.columns
      ? model.steps.find((each) => each !== step && each.blockedReason === undefined && each.to.category === 'progress')
      : undefined;
    if (other) second = { label: other.label, onclick: () => choose(other) };
  });

  const allowedNow = $derived(new Map((answer?.steps ?? []).filter((step) => step.allowed !== false).map((step) => [step.key, asksLine(step)])));
  const blockedNow = $derived(new Map((answer?.blocked ?? []).map((step) => [step.key, blockedReason(step)])));
  // The workflow's name for the picture's title: read only while the picture is open.
  $effect(() => (isPictureOpen ? workflows.open() : undefined));
  const workflowName = $derived(answer?.workflow ? workflows.list?.data.find((each) => each.key === answer.workflow)?.name : undefined);
</script>

{#if model && (answer || askable === false)}
  <span class="status" data-steps data-status={model.current.id}>
    <StatusButton
      current={model.current}
      steps={model.steps}
      statuses={model.statuses}
      label={t('app.workflows.status_button', { status: model.current.label })}
      menuTitle={model.columns ? t('app.workflows.columns_title') : model.byDefault ? t('app.workflows.default_title') : t('app.workflows.from_status', { status: model.current.label })}
      doneNote={t('app.workflows.done_note')}
      doneHint={model.doneHint}
      workflow={answer?.picture && !model.byDefault ? { label: t('app.workflows.view'), onview: () => (isPictureOpen = true) } : undefined}
      {disabledReason}
      {reasonSaidAt}
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
  <Dialog size="l" isOpen={true} title={workflowName ? t('app.workflows.picture_named', { name: workflowName }) : t('app.workflows.picture_title')} dismissLabel={t('app.workflows.close')} onClose={() => (isPictureOpen = false)}>
    <p class="counts">{t('app.workflows.picture_counts', { statuses: String(picture.statuses.length), steps: String(picture.steps.length) })}</p>
    <WorkflowPicture {picture} current={model.current.id} allowed={allowedNow} blocked={blockedNow} />
    {#snippet actions()}
      {#if answer.may_design && answer.workflow}
        {@const key = answer.workflow}
        <Button tone="secondary" onclick={() => { isPictureOpen = false; onnavigate?.(`/administration/workflows/${key}`); }}>{t('app.workflows.open_designer')}</Button>
      {/if}
    {/snippet}
  </Dialog>
{/if}

<style>
  .status { display: inline-flex; }
  .refusal { margin: 0; color: var(--text-danger); flex-basis: 100%; }
  .refusal p, .refusal ul { margin: 0; }
  .rest { margin: 0; padding-inline-start: var(--sp-200); }
  .counts { margin: 0 0 var(--sp-200); font-size: var(--fs-075); color: var(--text-secondary); }
</style>
