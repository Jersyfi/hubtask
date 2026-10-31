<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The *Templates* gallery (configuration/templates.md, ui.md §4): every template the reader may
  // take, by its four origins, with its version, the version the workspace took and *Update
  // available*. Taking or updating is a plan first - the dialog shows what it changes, and an
  // update lists each change the template made, a change the workspace made itself unticked until
  // the person ticks it - and the apply is exactly that plan, recording `template@version`. In a
  // hub's settings, the templates with a hub part: the hub takes that part for itself, once the
  // workspace took the template (templates.md §1).

  import { untrack } from 'svelte';
  import { Banner, Button, Checkbox, Dialog, Input, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplateList, ConfigTemplatePlan, ResourceState } from '@hubtask/sync-engine';

  import { config, needsAcknowledge } from '../data/config.svelte.ts';
  import { answersOf, byOrigin, offerOf, takenFirst } from '../data/configtemplates.ts';
  import { groups as workspaceGroups } from '../data/groups.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import PlanReach from './PlanReach.svelte';

  interface Props {
    /** A hub taking a template's hub part; omitted is the workspace. */
    hubId?: string;
  }

  const { hubId }: Props = $props();

  let list = $state<ResourceState<ConfigTemplateList>>();
  $effect(() => {
    const hub = hubId;
    return untrack(() => config.templates((next) => (list = next), hub));
  });

  const groups = $derived(list?.status === 'ready' ? byOrigin(list.data.data) : []);
  const refusal = $derived(list?.status === 'failed' ? renderProblem(list.error, messages) : undefined);

  /** Hubtask's templates are named by their key, in the reader's language. */
  const nameOf = (template: ConfigTemplate) =>
    template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;

  let chosen = $state<ConfigTemplate>();
  /** The answers to the chosen template's questions, by key. */
  let answers = $state<Record<string, string>>({});
  const asksGroup = $derived(chosen?.questions?.some((question) => question.kind === 'GROUP') ?? false);
  $effect(() => {
    if (!asksGroup) return;
    return untrack(() => workspaceGroups.open());
  });
  let plan = $state<ConfigTemplatePlan>();
  let take = $state<string[]>([]);
  let isOpen = $state(false);
  let acknowledged = $state(false);
  let busy = $state<string>();

  async function open(template: ConfigTemplate) {
    busy = template.key;
    chosen = template;
    answers = {};
    acknowledged = false;
    try {
      plan = await config.planTemplate(template.key, undefined, hubId, answersOf(template.questions, answers));
      take = takenFirst(plan.upgrade);
      isOpen = true;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      busy = undefined;
    }
  }

  async function choose(path: string, isTaken: boolean) {
    if (!chosen) return;
    take = isTaken ? [...take, path] : take.filter((each) => each !== path);
    try {
      plan = await config.planTemplate(chosen.key, take, hubId, answersOf(chosen.questions, answers));
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  /** An answer changes what the template brings, so the plan is asked again. */
  async function answer(key: string, value: string) {
    if (!chosen) return;
    answers = { ...answers, [key]: value };
    try {
      plan = await config.planTemplate(chosen.key, take, hubId, answersOf(chosen.questions, answers));
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  async function apply() {
    if (!plan?.plan_hash || !chosen) return;
    busy = 'apply';
    try {
      await config.applyTemplate(plan, acknowledged);
      toast.show({ text: t('app.templates.taken', { name: nameOf(chosen), version: chosen.version }), tone: 'success' });
      isOpen = false;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      busy = undefined;
    }
  }

  const counts = (each: { added: number; changed: number; removed: number }) =>
    t('app.changes.counts', { added: each.added, changed: each.changed, removed: each.removed });
</script>

<section class="block" aria-labelledby="templates-gallery" data-templates>
  <h2 id="templates-gallery">{t(hubId ? 'app.templates.hub_gallery' : 'app.templates.gallery')}</h2>
  <p class="quiet">{t(hubId ? 'app.templates.hub_about' : 'app.templates.about')}</p>
  {#if refusal}
    <Banner tone="danger" title={refusal.message} />
  {:else if list?.status !== 'ready'}
    <Spinner label={t('app.templates.reading')} />
  {:else if hubId && groups.length === 0}
    <p class="quiet" data-no-hub-templates>{t('app.templates.hub_none')}</p>
  {:else}
    {#each groups as group (group.origin)}
      <div class="origin" data-origin={group.origin}>
        <h3>{t(`app.templates.origin.${group.origin}`)}</h3>
        <ul class="cards">
          {#each group.templates as template (template.key)}
            {@const offer = offerOf(template)}
            <li class="card" data-template={template.key}>
              <span class="name">{nameOf(template)}</span>
              <code class="note">{template.provenance}</code>
              {#if offer === 'UPDATE'}
                <span class="update" data-update-available>{t('app.templates.update_available', { from: template.taken_version ?? 0, to: template.version })}</span>
              {:else if offer === 'IN_USE'}
                <span class="note">{t('app.templates.in_use', { version: template.version })}</span>
              {/if}
              {#if offer !== 'IN_USE'}
                <Button size="sm" tone={offer === 'UPDATE' ? 'primary' : 'secondary'} isBusy={busy === template.key}
                  data-take={template.key} onclick={() => open(template)}>
                  {t(offer === 'UPDATE' ? 'app.templates.update' : 'app.templates.take')}
                </Button>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  {/if}
</section>

{#if plan && chosen}
  <Dialog bind:isOpen title={t('app.templates.plan_title', { name: nameOf(chosen), version: chosen.version })} dismissLabel={t('app.changes.cancel')}>
    <Stack gap="150">
      {#if chosen.questions && chosen.questions.length > 0}
        <div class="questions" data-template-questions>
          <p class="quiet">{t('app.templates.questions_about')}</p>
          {#each chosen.questions as question (question.key)}
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
            {/if}
          {/each}
        </div>
      {/if}
      {#if plan.from_version}
        <p class="quiet">{t('app.templates.upgrade_about', { from: plan.from_version, to: chosen.version })}</p>
        {#if plan.upgrade.length === 0}
          <p class="quiet">{t('app.templates.upgrade_none')}</p>
        {:else}
          <ul class="changes" data-upgrade>
            {#each plan.upgrade as change (change.path)}
              <li data-upgrade-change={change.path}>
                <Checkbox
                  label={change.path}
                  hint={change.state === 'CONFLICT' ? t('app.templates.conflict') : t('app.templates.template_changed')}
                  checked={take.includes(change.path)}
                  onchange={(event: Event) => choose(change.path, (event.currentTarget as HTMLInputElement).checked)}
                />
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
      {#if plan.refusals.length > 0}
        <Banner tone="danger" title={t('app.templates.refused')}>
          <ul class="changes">
            {#each plan.refusals as refused (refused.path + refused.code)}
              <li><code>{refused.path}</code> {refused.code}</li>
            {/each}
          </ul>
        </Banner>
      {:else}
        <p class="quiet">{counts(plan.counts)}</p>
        <ul class="changes" data-plan-changes>
          {#each plan.changes as change (change.path)}
            <li><code>{change.op}</code> {change.path}</li>
          {/each}
        </ul>
        <PlanReach reach={plan.reach} />
        {#if needsAcknowledge(plan.flags)}
          <Checkbox label={t('app.changes.acknowledge')} checked={acknowledged}
            onchange={(event: Event) => (acknowledged = (event.currentTarget as HTMLInputElement).checked)} />
        {/if}
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.changes.cancel')}</Button>
      {#if plan?.plan_hash}
        <Button tone="primary" data-apply-template isBusy={busy === 'apply'}
          disabledReason={plan && needsAcknowledge(plan.flags) && !acknowledged ? t('app.changes.acknowledge_first') : undefined}
          onclick={apply}>{t('app.changes.apply')}</Button>
      {/if}
    {/snippet}
  </Dialog>
{/if}

<style>
  .block { display: flex; flex-direction: column; gap: var(--sp-150); }
  .block h2 { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .origin h3 { margin: 0 0 var(--sp-100); font-size: var(--fs-075); color: var(--text-secondary); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .cards {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr));
    gap: var(--sp-150);
  }
  .card {
    display: flex;
    flex-direction: column;
    align-items: start;
    gap: var(--sp-050);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .name { font-weight: var(--fw-medium); }
  .note { color: var(--text-subtle); font-size: var(--fs-075); }
  .update { color: var(--text-brand); font-size: var(--fs-075); }
  .questions { display: grid; gap: var(--sp-100); }
  .changes { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
</style>
