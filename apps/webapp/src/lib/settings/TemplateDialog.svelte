<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A template opened from a gallery (page-system §5.1, Dialog L): what it creates beside the
  // questions taking it asks, and below them the plan - taking or updating is a plan first, an
  // update listing each change the template made, a change the workspace made itself unticked
  // until the person ticks it - applied exactly as shown, recording `template@version`
  // (configuration.md §6). A template already in use, or one the reader may not take,
  // opens as its preview alone.

  import { untrack } from 'svelte';
  import { Banner, Button, Checkbox, Dialog, Input, Select, Spinner, Stack, type DialogAction } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplatePlan, TransportError } from '@hubtask/sync-engine';

  import FailureNote from '../frame/FailureNote.svelte';
  import TemplatePreview from './TemplatePreview.svelte';
  import PlanReach from './PlanReach.svelte';
  import { config, needsAcknowledge } from '../data/config.svelte.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { answersOf, asksWorkspaceFirst, changeWords, modulesToSwitchOn, offerOf, offersSamples, refusalWords, startsWithWorkspace, takenFirst } from '../data/configtemplates.ts';
  import { modules } from '../data/modules.svelte.ts';
  import { groups as workspaceGroups } from '../data/groups.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { afterTaking } from '../product/templates.ts';
  import { createsOf } from '../workspace/hubGallery.ts';

  interface Props {
    /** The template opened; none is closed. */
    template?: ConfigTemplate;
    /** A hub taking a template's hub part; omitted is the workspace. */
    hubId?: string;
    /** A collection taking a template's collection part; its hub follows from it. */
    collectionId?: string;
    /** Opened as its preview alone: for a reader who may not take it. */
    isPreviewOnly?: boolean;
    /** Where a taken template's main product home is opened (§3.2); without it the page stays. */
    onnavigate?: (path: string) => void;
    /** Ran when it closes, however it closed. */
    onClose: () => void;
    /** The template whose plan is being asked, before the dialog opens. */
    pending?: string;
  }

  let { template, hubId, collectionId, isPreviewOnly = false, onnavigate, onClose, pending = $bindable() }: Props = $props();

  /** Hubtask's templates are named by their key, in the reader's language. */
  const nameOf = (each: ConfigTemplate) =>
    each.origin === 'HUBTASK' ? t(`app.structure.preset.${each.key.toUpperCase()}`) : each.name;
  const summaryOf = (each: ConfigTemplate) =>
    each.origin === 'HUBTASK' && messages.has(`app.templates.summary.${each.key.toUpperCase()}`) ? t(`app.templates.summary.${each.key.toUpperCase()}`) : undefined;

  let chosen = $state<ConfigTemplate>();
  let isOpen = $state(false);
  /** The answers to the chosen template's questions, by key. */
  let answers = $state<Record<string, string>>({});
  /** Whether the hub takes the template's sample entries too, offered on its first take only. */
  let withSamples = $state(false);
  let plan = $state<ConfigTemplatePlan>();
  /** The plan shown is the workspace's part, which this place's waits on; its own follows it. */
  let isWorkspaceFirst = $state(false);
  let take = $state<string[]>([]);
  let acknowledged = $state(false);
  let busy = $state<string>();
  // Everything after the plan is asked happens with the dialog open; a refusal is said inside it,
  // which keeps the answers given (user-experience.md §5.10).
  let dialogFailure = $state<string>();

  const samplesOffered = $derived(offersSamples(chosen, hubId));
  const asksGroup = $derived(chosen?.questions?.some((question) => question.kind === 'GROUP') ?? false);
  $effect(() => {
    if (!asksGroup) return;
    return untrack(() => workspaceGroups.open());
  });
  const creates = $derived(chosen ? createsOf(chosen) : []);
  const hasAnswers = $derived(plan !== undefined && (((chosen?.questions?.length ?? 0) > 0) || (samplesOffered && !isWorkspaceFirst)));

  $effect(() => {
    const next = template;
    untrack(() => {
      if (next) void start(next);
      else isOpen = false;
    });
  });

  async function start(next: ConfigTemplate) {
    chosen = next;
    answers = {};
    withSamples = false;
    acknowledged = false;
    dialogFailure = undefined;
    plan = undefined;
    take = [];
    isWorkspaceFirst = false;
    if (isPreviewOnly || offerOf(next) === 'IN_USE') {
      isOpen = true;
      return;
    }
    // It opens at once with what the template creates; the plan follows below it.
    pending = next.key;
    isOpen = true;
    // A hub or a collection takes its part once the workspace took the template's own. Whoever
    // may change the workspace is shown that part's plan first, in the same dialog, rather than
    // a refusal (user-experience.md §5.2); anybody else reads why.
    const mayChangeWorkspace = holdsHere('STRUCTURE') === true;
    isWorkspaceFirst = startsWithWorkspace(next, hubId ?? collectionId, mayChangeWorkspace);
    try {
      plan = await config.planTemplate(next.key, undefined, isWorkspaceFirst ? undefined : hubId, answersOf(next.questions, answers), false, isWorkspaceFirst ? undefined : collectionId);
      take = takenFirst(plan.upgrade);
    } catch (error) {
      // The list was read before the workspace's part moved: the refusal says so instead.
      if (!isWorkspaceFirst && (hubId || collectionId) && asksWorkspaceFirst(error as TransportError) && mayChangeWorkspace) {
        try {
          plan = await config.planTemplate(next.key, undefined, undefined, answersOf(next.questions, answers), false);
          take = takenFirst(plan.upgrade);
          isWorkspaceFirst = true;
        } catch (inner) {
          dialogFailure = renderProblem(inner as never, messages).message;
        }
      } else {
        dialogFailure = renderProblem(error as never, messages).message;
      }
    } finally {
      pending = undefined;
    }
  }

  /** The plan asked again with what the person chose so far. */
  async function replan() {
    if (!chosen) return;
    dialogFailure = undefined;
    try {
      plan = await config.planTemplate(chosen.key, take, isWorkspaceFirst ? undefined : hubId, answersOf(chosen.questions, answers), !isWorkspaceFirst && withSamples, isWorkspaceFirst ? undefined : collectionId);
    } catch (error) {
      dialogFailure = renderProblem(error as never, messages).message;
    }
  }

  function choose(path: string, isTaken: boolean) {
    take = isTaken ? [...take, path] : take.filter((each) => each !== path);
    void replan();
  }

  /** An answer changes what the template brings, so the plan is asked again. */
  async function answer(key: string, value: string) {
    answers = { ...answers, [key]: value };
    await replan();
  }

  /** Taking the samples or not changes the plan, so it is asked again. */
  function sampled(on: boolean) {
    withSamples = on;
    void replan();
  }

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
    busy = 'modules';
    try {
      for (const key of needed) await modules.switch(key as Parameters<typeof modules.switch>[0], 'ON');
      await replan();
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

  // The one primary, named with what it does: switching the modules a take needs, or the take.
  const primary = $derived.by<DialogAction | undefined>(() => {
    if (!plan || !chosen) return undefined;
    if (needed.length > 0 && maySwitch) {
      return { label: t('app.templates.switch_on_and_use', { modules: neededNames }), isBusy: busy === 'modules', onclick: () => void switchOnAndUse() };
    }
    if (!plan.plan_hash) return undefined;
    return {
      label: t(plan.from_version ? 'app.templates.update_template' : 'app.templates.take_template'),
      isBusy: busy === 'apply',
      disabledReason: needsAcknowledge(plan.flags) && !acknowledged ? t('app.changes.acknowledge_first') : undefined,
      onclick: () => void apply(),
    };
  });
</script>

{#if chosen}
  <Dialog size="l" bind:isOpen title={nameOf(chosen)} dismissLabel={t(plan ? 'app.changes.cancel' : 'app.page.close')} {onClose} {primary}>
    <div data-template-dialog={chosen.key}>
      <Stack gap="300">
        {#if isWorkspaceFirst}
          <Banner tone="info" title={t('app.templates.workspace_first_title')}>
            <p class="quiet" data-template-workspace-first>{t(collectionId ? 'app.templates.workspace_first_collection' : 'app.templates.workspace_first_hub')}</p>
          </Banner>
        {/if}
        <div class="beside" data-columns={hasAnswers ? 'two' : 'one'}>
          <section class="group" aria-label={t('app.templates.creates_title')}>
            <h3>{t('app.templates.creates_title')}</h3>
            {#if summaryOf(chosen)}<p class="quiet">{summaryOf(chosen)}</p>{/if}
            {#if creates.length > 0}<TemplatePreview template={chosen} isUnfolded />{/if}
            {#if offerOf(chosen) === 'IN_USE'}
              <p class="quiet" data-template-note>{t('app.templates.in_use', { version: chosen.version })}</p>
            {/if}
          </section>
          {#if plan && hasAnswers}
            <section class="group" aria-label={t('app.templates.answers_title')}>
              <h3>{t('app.templates.answers_title')}</h3>
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
            </section>
          {/if}
        </div>

        {#if !plan && pending === chosen.key}
          <p class="quiet busy"><Spinner label={t('app.templates.planning')} /> <span>{t('app.templates.planning')}</span></p>
        {/if}
        {#if plan}
          <section class="group" aria-label={t('app.templates.changes_title')} data-template-plan>
            <h3>{t('app.templates.changes_title')}</h3>
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
              <FailureNote problem={t('app.templates.refused')}>
                <ul class="changes">
                  {#each plan.refusals as refused (refused.path + refused.code)}
                    <li>{refusalText(refused)}</li>
                  {/each}
                </ul>
              </FailureNote>
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
          </section>
        {/if}
        {#if dialogFailure}<FailureNote problem={dialogFailure} />{/if}
      </Stack>
    </div>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => (isOpen = false)}>{t(plan ? 'app.changes.cancel' : 'app.page.close')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .beside { display: grid; gap: var(--sp-300); align-items: start; }
  .beside[data-columns='two'] { grid-template-columns: repeat(auto-fit, minmax(min(100%, 28ch), 1fr)); }
  .group { display: grid; gap: var(--sp-100); min-inline-size: 0; }
  .group h3 { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
  .busy { display: flex; gap: var(--sp-100); align-items: center; }
  .questions { display: grid; gap: var(--sp-100); }
  .create-group { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }
  .changes { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  /* A long plan reads in columns, so the dialog seldom scrolls (page-system §5.1). */
  .changes[data-plan-changes] { display: block; columns: 28ch auto; column-gap: var(--sp-300); }
  .changes[data-plan-changes] li { break-inside: avoid; margin-block-end: var(--sp-050); }
</style>
