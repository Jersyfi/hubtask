<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What taking a template asks before the one press that makes its hub (user-experience.md §3.2,
  // §5.2): its questions with their defaults, a `GROUP` question's group made in place, its sample
  // entries (on by default), and what it sets up in words - the workspace part's plan where the
  // workspace has not taken it, since the hub part can only be planned once the hub exists. The
  // starter and *Create hub* ask the same, so they share this.

  import { untrack } from 'svelte';

  import { Button, Checkbox, Input, Select, Spinner } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplatePlan, TransportError } from '@hubtask/sync-engine';

  import { config, needsAcknowledge } from '../data/config.svelte.ts';
  import { answersOf, changeWords } from '../data/configtemplates.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { groups as workspaceGroups } from '../data/groups.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import TemplatePreview from '../settings/TemplatePreview.svelte';

  interface Props {
    template: ConfigTemplate;
    /** Whether the workspace part is planned and shown: false where the workspace took it already. */
    plansWorkspace: boolean;
    samplesOffered: boolean;
    answers: Record<string, string>;
    withSamples: boolean;
    acknowledged: boolean;
    /** Out: the plan is being read, so the press waits. */
    isPlanning?: boolean;
    /** Out: the plan hides data or changes permissions, so the press waits for `acknowledged`. */
    mustAcknowledge?: boolean;
    /** Out: a refusal of the plan or of making a group, in words. */
    failure?: string;
  }

  let {
    template, plansWorkspace, samplesOffered,
    answers = $bindable(), withSamples = $bindable(), acknowledged = $bindable(),
    isPlanning = $bindable(false), mustAcknowledge = $bindable(false), failure = $bindable(),
  }: Props = $props();

  let plan = $state<ConfigTemplatePlan>();

  // Planned anew for each template and each answer, by value: the catalogue read again hands a new
  // object for the same template, which is no reason to plan again. An answer that lands after a
  // newer one is dropped.
  const templateKey = $derived(template.key);
  const givenText = $derived(JSON.stringify(answersOf(template.questions, answers)));
  let asked = 0;
  $effect(() => {
    const key = templateKey;
    const given = JSON.parse(givenText) as Record<string, string>;
    if (!plansWorkspace) {
      untrack(() => { plan = undefined; isPlanning = false; });
      return;
    }
    const mine = ++asked;
    untrack(() => {
      isPlanning = true;
      failure = undefined;
    });
    config.planTemplate(key, undefined, undefined, given, false).then(
      (next) => { if (mine === asked) plan = next; },
      (error: unknown) => {
        if (mine !== asked) return;
        plan = undefined;
        failure = renderProblem(error as TransportError, messages).message;
      },
    ).finally(() => { if (mine === asked) isPlanning = false; });
  });
  $effect(() => {
    mustAcknowledge = plan?.flags ? needsAcknowledge(plan.flags) : false;
  });

  const asksGroup = $derived(template.questions?.some((question) => question.kind === 'GROUP') ?? false);
  $effect(() => {
    if (!asksGroup) return;
    return untrack(() => workspaceGroups.open());
  });

  // A GROUP question offers to make the group in place (§3.2): where it is created it is the answer.
  const mayCreateGroup = $derived(holdsHere('MANAGE_MEMBERS') === true);
  let groupDraft = $state<Record<string, string>>({});
  let groupBusy = $state<string>();
  async function createGroup(key: string) {
    const name = (groupDraft[key] ?? '').trim();
    if (name === '') return;
    groupBusy = key;
    failure = undefined;
    try {
      const group = await workspaceGroups.create(name);
      groupDraft = { ...groupDraft, [key]: '' };
      answer(key, group.id);
    } catch (error) {
      failure = renderProblem(error as TransportError, messages).message;
    } finally {
      groupBusy = undefined;
    }
  }

  function answer(key: string, value: string) {
    answers = { ...answers, [key]: value };
  }

  function changeText(change: ConfigTemplatePlan['changes'][number]): string {
    const words = changeWords(change);
    return t(words.code, { section: t(words.section), key: words.module ? t(words.module) : words.key });
  }
</script>

{#if template.questions && template.questions.length > 0}
  <div class="questions" data-template-questions>
    <p class="hint">{t('app.templates.questions_about')}</p>
    {#each template.questions as question (question.key)}
      {@const label = t(`app.templates.question.${question.key}`)}
      {#if question.kind === 'TEXT'}
        <Input {label} value={answers[question.key] ?? question.default ?? ''} data-question={question.key}
          onchange={(event: Event) => answer(question.key, (event.currentTarget as HTMLInputElement).value)} />
      {:else}
        <Select {label} value={answers[question.key] ?? question.default ?? ''} data-question={question.key}
          hint={question.required ? undefined : t('app.templates.question_optional')}
          options={[
            { value: '', label: t('app.templates.question_open') },
            ...(question.kind === 'GROUP'
              ? workspaceGroups.all.map((group) => ({ value: group.id, label: group.name }))
              : question.options.map((option) => ({ value: option, label: option }))),
          ]}
          onchange={(event: Event) => answer(question.key, (event.currentTarget as HTMLSelectElement).value)} />
        {#if question.kind === 'GROUP' && mayCreateGroup}
          <div class="create-group" data-create-group={question.key}>
            <Input label={t('app.templates.create_group_name')} value={groupDraft[question.key] ?? ''}
              oninput={(event: Event) => (groupDraft = { ...groupDraft, [question.key]: (event.currentTarget as HTMLInputElement).value })} />
            <Button tone="secondary" size="sm" isBusy={groupBusy === question.key}
              disabledReason={(groupDraft[question.key] ?? '').trim() === '' ? t('app.templates.create_group_name_first') : undefined}
              onclick={() => createGroup(question.key)}>{t('app.templates.create_group')}</Button>
          </div>
        {/if}
      {/if}
    {/each}
  </div>
{/if}
{#if samplesOffered}
  <Checkbox label={t('app.templates.samples')} hint={t('app.templates.samples_hint')} checked={withSamples} data-template-samples
    onchange={(event: Event) => (withSamples = (event.currentTarget as HTMLInputElement).checked)} />
{/if}
<section class="plan" aria-label={t('app.starter.plan')} data-starter-plan>
  <h3>{t('app.starter.plan')}</h3>
  <TemplatePreview {template} isUnfolded />
  {#if isPlanning}
    <Spinner label={t('app.starter.plan_reading')} />
  {:else if plan && (plan.refusals ?? []).length === 0}
    <ul class="changes" data-plan-changes>
      {#each plan.changes ?? [] as change (change.path)}<li>{changeText(change)}</li>{/each}
    </ul>
    {#if mustAcknowledge}
      <Checkbox label={t('app.changes.acknowledge')} checked={acknowledged}
        onchange={(event: Event) => (acknowledged = (event.currentTarget as HTMLInputElement).checked)} />
    {/if}
  {/if}
</section>

<style>
  .hint { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }
  .questions { display: grid; gap: var(--sp-100); }
  .create-group { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .plan { display: grid; gap: var(--sp-100); }
  .plan h3 { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .changes { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); }
</style>
