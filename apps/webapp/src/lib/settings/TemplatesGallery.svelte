<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The *Templates* gallery (configuration.md §6, ui.md §4): every template the reader may
  // take, by its four origins, with its version, the version the workspace took and *Update
  // available*. Taking or updating is a plan first - the dialog shows what it changes, and an
  // update lists each change the template made, a change the workspace made itself unticked until
  // the person ticks it - and the apply is exactly that plan, recording `template@version`. In a
  // hub's settings, the templates with a hub part: the hub takes that part for itself, once the
  // workspace took the template (configuration.md §6). In a collection's settings, the templates
  // with a collection part, which the collection takes under its own handle.

  import { untrack } from 'svelte';
  import { Banner, Button, Checkbox, Dialog, Input, Select, Spinner, Stack, TemplateCard, TemplateGallery, ViewSwitcher } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplateList, ConfigTemplatePlan, ResourceState, TransportError } from '@hubtask/sync-engine';

  import TemplatePreview from './TemplatePreview.svelte';
  import { config, needsAcknowledge } from '../data/config.svelte.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { answersOf, byOrigin, changeWords, galleryScope, modulesToSwitchOn, offerOf, offersSamples, refusalWords, takenFirst } from '../data/configtemplates.ts';
  import { modules } from '../data/modules.svelte.ts';
  import { groups as workspaceGroups } from '../data/groups.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import PlanReach from './PlanReach.svelte';
  import { afterTaking } from '../product/templates.ts';
  import { familiesOf, familyOf, switchedOn } from '../workspace/hubGallery.ts';

  interface Props {
    /** A hub taking a template's hub part; omitted is the workspace. */
    hubId?: string;
    /** A collection taking a template's collection part; its hub follows from it. */
    collectionId?: string;
    /** A template to open the plan of at once, as a product's first visit chose it (§4). */
    preselect?: string;
    /** Where a taken template's main product home is opened (§3.2); without it the page stays. */
    onnavigate?: (path: string) => void;
  }

  const { hubId, collectionId, preselect, onnavigate }: Props = $props();
  /** Which gallery this is: its heading, its words and what it says when it offers nothing. */
  const scope = $derived(galleryScope(hubId, collectionId));

  let list = $state<ResourceState<ConfigTemplateList>>();
  $effect(() => {
    const hub = hubId;
    const collection = collectionId;
    return untrack(() => config.templates((next) => (list = next), hub, collection));
  });

  // The family filter (user-experience.md §5.2): All, then each family the catalogue holds.
  let family = $state('all');
  const families = $derived(list?.status === 'ready' ? familiesOf(list.data.data) : []);
  const filterViews = $derived([
    { id: 'all', label: t('app.templates.family_all') },
    ...families.map((each) => ({ id: each, label: t(`app.workspace.hub_family.${each}`) })),
  ]);
  const shown = $derived(
    list?.status !== 'ready' ? [] : family === 'all' ? list.data.data : list.data.data.filter((each) => familyOf(each) === family),
  );
  const groups = $derived(byOrigin(shown));
  const refusal = $derived(list?.status === 'failed' ? renderProblem(list.error, messages) : undefined);

  /** Hubtask's templates are named by their key, in the reader's language. */
  const nameOf = (template: ConfigTemplate) =>
    template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;
  /** What a card says it does: Hubtask's own sentence, with the standard it follows; nothing for another's. */
  const summaryOf = (template: ConfigTemplate) =>
    template.origin === 'HUBTASK' ? t(`app.templates.summary.${template.key.toUpperCase()}`) : undefined;
  const productsOf = (template: ConfigTemplate): string[] =>
    switchedOn(template).map((module) => t(`app.modules.name_${module.toLowerCase()}`));

  let chosen = $state<ConfigTemplate>();
  /** The answers to the chosen template's questions, by key. */
  let answers = $state<Record<string, string>>({});
  /** Whether the hub takes the template's sample entries too, offered on its first take only. */
  let withSamples = $state(false);
  const samplesOffered = $derived(offersSamples(chosen, hubId));
  const asksGroup = $derived(chosen?.questions?.some((question) => question.kind === 'GROUP') ?? false);
  $effect(() => {
    if (!asksGroup) return;
    return untrack(() => workspaceGroups.open());
  });
  // A GROUP question offers to make the group in place (§3.2): where it is created it is the answer.
  const mayCreateGroup = $derived(holdsHere('MANAGE_MEMBERS') === true);
  let groupDraft = $state<Record<string, string>>({});
  async function createGroup(key: string) {
    const name = (groupDraft[key] ?? '').trim();
    if (name === '') return;
    busy = `group:${key}`;
    dialogFailure = undefined;
    try {
      const group = await workspaceGroups.create(name);
      groupDraft = { ...groupDraft, [key]: '' };
      await answer(key, group.id);
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    } finally {
      busy = undefined;
    }
  }
  let plan = $state<ConfigTemplatePlan>();
  /** The plan shown is the workspace's part, which this place's waits on; its own follows it. */
  let isWorkspaceFirst = $state(false);
  let take = $state<string[]>([]);
  let isOpen = $state(false);
  let acknowledged = $state(false);
  let busy = $state<string>();
  // Everything after the plan is asked happens with the dialog open; a refusal is said inside it,
  // which keeps the answers given (user-experience.md §5.10).
  let dialogFailure = $state<string>();

  async function open(template: ConfigTemplate) {
    busy = template.key;
    chosen = template;
    answers = {};
    withSamples = false;
    acknowledged = false;
    dialogFailure = undefined;
    isWorkspaceFirst = false;
    try {
      plan = await config.planTemplate(template.key, undefined, hubId, answersOf(template.questions, answers), false, collectionId);
      take = takenFirst(plan.upgrade);
      isOpen = true;
    } catch (error) {
      // A hub or a collection takes its part once the workspace took the template's own. Whoever
      // may change the workspace is shown that part's plan first, in the same dialog, rather than
      // a refusal (user-experience.md §5.2); anybody else reads why.
      if ((hubId || collectionId) && (error as TransportError).detailCode === 'config.template_workspace_first' && holdsHere('STRUCTURE') === true) {
        try {
          plan = await config.planTemplate(template.key, undefined, undefined, answersOf(template.questions, answers), false);
          take = takenFirst(plan.upgrade);
          isWorkspaceFirst = true;
          isOpen = true;
        } catch (inner) {
          toast.show({ text: renderProblem(inner as never, messages).message, tone: 'danger' });
        }
      } else {
        toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      }
    } finally {
      busy = undefined;
    }
  }

  // A template chosen on a product's empty home before its hub existed: its plan opens once the
  // list holds it, once. Unknown or no longer offered, the gallery is drawn as always.
  let isPreselected = false;
  $effect(() => {
    const key = preselect;
    if (!key || isPreselected || list?.status !== 'ready') return;
    const template = list.data.data.find((each) => each.key === key);
    isPreselected = true;
    if (template) untrack(() => void open(template));
  });

  async function choose(path: string, isTaken: boolean) {
    if (!chosen) return;
    take = isTaken ? [...take, path] : take.filter((each) => each !== path);
    dialogFailure = undefined;
    try {
      plan = await config.planTemplate(chosen.key, take, isWorkspaceFirst ? undefined : hubId, answersOf(chosen.questions, answers), !isWorkspaceFirst && withSamples, isWorkspaceFirst ? undefined : collectionId);
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    }
  }

  /** An answer changes what the template brings, so the plan is asked again. */
  async function answer(key: string, value: string) {
    if (!chosen) return;
    answers = { ...answers, [key]: value };
    dialogFailure = undefined;
    try {
      plan = await config.planTemplate(chosen.key, take, isWorkspaceFirst ? undefined : hubId, answersOf(chosen.questions, answers), !isWorkspaceFirst && withSamples, isWorkspaceFirst ? undefined : collectionId);
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    }
  }

  /** Taking the samples or not changes the plan, so it is asked again. */
  async function sampled(on: boolean) {
    if (!chosen) return;
    withSamples = on;
    dialogFailure = undefined;
    try {
      plan = await config.planTemplate(chosen.key, take, isWorkspaceFirst ? undefined : hubId, answersOf(chosen.questions, answers), !isWorkspaceFirst && withSamples, isWorkspaceFirst ? undefined : collectionId);
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    }
  }

  async function apply() {
    if (!plan?.plan_hash || !chosen) return;
    busy = 'apply';
    dialogFailure = undefined;
    try {
      await config.applyTemplate(plan, acknowledged);
      if (isWorkspaceFirst) {
        // The workspace's part is in force: the place's own is planned now, the answers kept.
        isWorkspaceFirst = false;
        acknowledged = false;
        plan = await config.planTemplate(chosen.key, undefined, hubId, answersOf(chosen.questions, answers), false, collectionId);
        take = takenFirst(plan.upgrade);
        return;
      }
      toast.show({ text: t('app.templates.taken', { name: nameOf(chosen) }), tone: 'success' });
      isOpen = false;
      // A collection's own part leaves the reader where they are; a hub's or the workspace's
      // opens the product the template is for.
      const next = collectionId ? undefined : afterTaking(chosen, hubId);
      if (next) onnavigate?.(next);
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    } finally {
      busy = undefined;
    }
  }

  /**
   * A template refused only because modules are off is one switch away: named in words, and offered
   * to switch on where the reader may change the workspace's shape - never a code or a path.
   */
  const needed = $derived(plan ? modulesToSwitchOn(plan.refusals) : []);
  const neededNames = $derived(new Intl.ListFormat(messages.locale, { type: 'conjunction' })
    .format(needed.map((key) => t(`app.modules.name_${key.toLowerCase()}`))));
  const maySwitch = $derived(holdsHere('STRUCTURE') === true);

  async function switchOnAndUse() {
    if (!chosen) return;
    busy = 'modules';
    dialogFailure = undefined;
    try {
      for (const key of needed) await modules.switch(key as Parameters<typeof modules.switch>[0], 'ON');
      plan = await config.planTemplate(chosen.key, take, isWorkspaceFirst ? undefined : hubId, answersOf(chosen.questions, answers), !isWorkspaceFirst && withSamples, isWorkspaceFirst ? undefined : collectionId);
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    } finally {
      busy = undefined;
    }
  }

  function refusalText(refused: ConfigTemplatePlan['refusals'][number]): string {
    const words = refusalWords(refused);
    const reason = words.module ? t(words.code, { module: t(words.module) }) : t(words.code, refused.params ?? {});
    return t('app.templates.refused_item', { section: t(words.section), reason });
  }

  function changeText(change: ConfigTemplatePlan['changes'][number]): string {
    const words = changeWords(change);
    return t(words.code, { section: t(words.section), key: words.module ? t(words.module) : words.key });
  }

  const counts = (each: { added: number; changed: number; removed: number }) =>
    t('app.changes.counts', { added: each.added, changed: each.changed, removed: each.removed });
</script>

<section class="block" aria-labelledby="templates-gallery" data-templates>
  <h2 id="templates-gallery">{t(scope === 'collection' ? 'app.templates.collection_gallery' : scope === 'hub' ? 'app.templates.hub_gallery' : 'app.templates.gallery')}</h2>
  <p class="quiet">{t(scope === 'collection' ? 'app.templates.collection_about' : scope === 'hub' ? 'app.templates.hub_about' : 'app.templates.about')}</p>
  {#if refusal}
    <Banner tone="danger" title={refusal.message} />
  {:else if list?.status !== 'ready'}
    <Spinner label={t('app.templates.reading')} />
  {:else if scope === 'collection' && groups.length === 0}
    <p class="quiet" data-no-collection-templates>{t('app.templates.collection_none')}</p>
  {:else if hubId && groups.length === 0}
    <p class="quiet" data-no-hub-templates>{t('app.templates.hub_none')}</p>
  {:else}
    {#if families.length > 1}
      <ViewSwitcher label={t('app.templates.families')} views={filterViews} selected={family} onselect={(id) => (family = id)} />
    {/if}
    {#each groups as group (group.origin)}
      <div data-origin={group.origin}>
        <TemplateGallery label={t(`app.templates.origin.${group.origin}`)}>
          {#each group.templates as template (template.key)}
            {@const offer = offerOf(template)}
            <TemplateCard
              name={nameOf(template)}
              summary={summaryOf(template)}
              products={productsOf(template)}
              data-template={template.key}
              note={offer === 'UPDATE'
                ? t('app.templates.update_available', { from: template.taken_version ?? 0, to: template.version })
                : offer === 'IN_USE'
                  ? t('app.templates.in_use', { version: template.version })
                  : undefined}
              isNoteNews={offer === 'UPDATE'}
              {details}
              action={offer === 'IN_USE' ? undefined : takeAct}
            />
            {#snippet details()}<TemplatePreview {template} />{/snippet}
            {#snippet takeAct()}
              <Button size="sm" tone={offer === 'UPDATE' ? 'primary' : 'secondary'} isBusy={busy === template.key}
                data-take={template.key} data-update-available={offer === 'UPDATE' ? '' : undefined} onclick={() => open(template)}>
                {t(offer === 'UPDATE' ? 'app.templates.update' : 'app.templates.take')}
              </Button>
            {/snippet}
          {/each}
        </TemplateGallery>
      </div>
    {/each}
  {/if}
</section>

{#if plan && chosen}
  <Dialog bind:isOpen title={nameOf(chosen)} dismissLabel={t('app.changes.cancel')}>
    <Stack gap="150">
      {#if isWorkspaceFirst}
        <Banner tone="info" title={t('app.templates.workspace_first_title')}>
          <p class="quiet" data-template-workspace-first>{t(collectionId ? 'app.templates.workspace_first_collection' : 'app.templates.workspace_first_hub')}</p>
        </Banner>
      {/if}
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
              {#if question.kind === 'GROUP' && mayCreateGroup}
                <div class="create-group" data-create-group={question.key}>
                  <Input label={t('app.templates.create_group_name')} value={groupDraft[question.key] ?? ''}
                    oninput={(event: Event) => (groupDraft = { ...groupDraft, [question.key]: (event.currentTarget as HTMLInputElement).value })} />
                  <Button tone="secondary" size="sm" isBusy={busy === `group:${question.key}`}
                    disabledReason={(groupDraft[question.key] ?? '').trim() === '' ? t('app.templates.create_group_name_first') : undefined}
                    onclick={() => createGroup(question.key)}>{t('app.templates.create_group')}</Button>
                </div>
              {/if}
            {/if}
          {/each}
        </div>
      {/if}
      {#if samplesOffered && !isWorkspaceFirst}
        <Checkbox label={t('app.templates.samples')} hint={t('app.templates.samples_hint')} checked={withSamples}
          data-template-samples
          onchange={(event: Event) => sampled((event.currentTarget as HTMLInputElement).checked)} />
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
      {#if needed.length > 0}
        <Banner tone="info" title={t('app.templates.needs_modules_title')}>
          <p class="quiet" data-template-needs-modules>{t('app.templates.needs_module', { module: neededNames })}
            {maySwitch ? t('app.templates.needs_modules_body') : t('app.templates.needs_modules_ask')}</p>
        </Banner>
      {:else if plan.refusals.length > 0}
        <Banner tone="danger" title={t('app.templates.refused')}>
          <ul class="changes">
            {#each plan.refusals as refused (refused.path + refused.code)}
              <li>{refusalText(refused)}</li>
            {/each}
          </ul>
        </Banner>
      {:else}
        <p class="quiet">{counts(plan.counts)}</p>
        <ul class="changes" data-plan-changes>
          {#each plan.changes as change (change.path)}
            <li>{changeText(change)}</li>
          {/each}
        </ul>
        <PlanReach reach={plan.reach} />
        {#if needsAcknowledge(plan.flags)}
          <Checkbox label={t('app.changes.acknowledge')} checked={acknowledged}
            onchange={(event: Event) => (acknowledged = (event.currentTarget as HTMLInputElement).checked)} />
        {/if}
      {/if}
      {#if dialogFailure}<Banner tone="danger">{dialogFailure}</Banner>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.changes.cancel')}</Button>
      {#if needed.length > 0 && maySwitch}
        <Button tone="primary" data-switch-modules isBusy={busy === 'modules'} onclick={switchOnAndUse}>
          {t('app.templates.switch_on_and_use', { modules: neededNames })}</Button>
      {/if}
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
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .questions { display: grid; gap: var(--sp-100); }
  .create-group { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .changes { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
</style>
