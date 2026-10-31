<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Draft and publish for a block many people work with (ui.md A4): a type's form, an SLA policy,
  // a role. Saving keeps a draft beside the block in force, which everybody keeps working with;
  // publishing first shows the configuration's own plan of the block - what it changes and
  // reaches, what it refuses - and then applies that plan, which removes the draft.

  import { untrack } from 'svelte';
  import { Banner, Button, Checkbox, Inline, Stack } from '@hubtask/design-system/components';
  import type { ConfigDraft, ConfigDraftList, ConfigPlan, ResourceState } from '@hubtask/sync-engine';

  import { config, type DraftSection } from '../data/config.svelte.ts';
  import { draftOf, publishStep } from '../data/configdrafts.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import PlanReach from './PlanReach.svelte';

  interface Props {
    section: DraftSection;
    key: string;
    /** The block as the editor holds it now, as the configuration document writes it. */
    block: () => Record<string, unknown>;
    /** Called once with a waiting draft's block, so the editor opens on it. */
    onLoad: (block: Record<string, unknown>) => void;
    onPublished: () => void;
    /** The fields a refused publish names, for the editor to show beside its controls. */
    onProblem?: (fields: ReturnType<typeof renderProblem>['fields']) => void;
  }

  let { section, key, block, onLoad, onPublished, onProblem }: Props = $props();

  let drafts = $state<ResourceState<ConfigDraftList> | undefined>(undefined);
  let loaded = false;
  $effect(() => untrack(() => config.drafts((next) => {
    drafts = next;
    const waiting = next.status === 'ready' ? draftOf(next.data, section, key) : undefined;
    if (waiting && !loaded) onLoad(waiting.block as Record<string, unknown>);
    loaded = loaded || next.status === 'ready';
  })));

  const waiting = $derived<ConfigDraft | undefined>(drafts?.status === 'ready' ? draftOf(drafts.data, section, key) : undefined);
  let plan = $state<ConfigPlan | undefined>(undefined);
  let planned = $state<Record<string, unknown> | undefined>(undefined);
  let acknowledged = $state(false);
  let isBusy = $state(false);
  let problem = $state<string | undefined>(undefined);
  const step = $derived(plan ? publishStep(plan) : undefined);

  async function run(work: () => Promise<void>) {
    isBusy = true;
    problem = undefined;
    try {
      await work();
    } catch (error) {
      const rendered = renderProblem(error as never, messages);
      problem = rendered.message;
      onProblem?.(rendered.fields);
    } finally {
      isBusy = false;
    }
  }

  const saveDraft = () => run(async () => {
    await config.saveDraft(section, key, block(), waiting?.version);
    plan = undefined;
    toast.show({ text: t('app.config_drafts.saved'), tone: 'success' });
  });

  const discard = () => run(async () => {
    await config.discardDraft(section, key);
    plan = undefined;
    toast.show({ text: t('app.config_drafts.discarded'), tone: 'success' });
  });

  const review = () => run(async () => {
    planned = block();
    acknowledged = false;
    plan = await config.planDraft(section, key, planned);
  });

  const publish = () => run(async () => {
    if (!plan || !planned) return;
    await config.publishDraft(section, key, planned, plan, acknowledged);
    toast.show({ text: t('app.config_drafts.published'), tone: 'success' });
    plan = undefined;
    onPublished();
  });
</script>

<Stack gap="150">
  {#if waiting}
    <Banner tone="info" title={t('app.config_drafts.waiting', { when: formatDateTime(waiting.saved_at, messages.locale) })} />
  {/if}
  {#if plan && step === 'refused'}
    <Banner tone="danger" title={t('app.config_drafts.refused')} />
    <ul class="lines" data-draft-refusals>
      {#each plan.refusals as refusal, index (index)}<li>{t(refusal.code)}</li>{/each}
    </ul>
  {:else if plan && step === 'unchanged'}
    <p class="quiet">{t('app.config_drafts.unchanged')}</p>
  {:else if plan}
    <p class="quiet" data-draft-plan>{t('app.config_drafts.plan', { count: plan.changes.length })}</p>
    <PlanReach reach={plan.reach} />
    {#if step === 'acknowledge'}
      <Checkbox label={t('app.config_drafts.acknowledge')} checked={acknowledged}
        onchange={(event: Event) => (acknowledged = (event.currentTarget as HTMLInputElement).checked)} />
    {/if}
  {/if}
  {#if problem}<p class="refusal" role="alert">{problem}</p>{/if}
  <Inline gap="150">
    {#if waiting}
      <Button tone="subtle" isBusy={isBusy} onclick={discard}>{t('app.config_drafts.discard')}</Button>
    {/if}
    <Button tone="secondary" data-save-draft isBusy={isBusy} onclick={saveDraft}>{t('app.config_drafts.save')}</Button>
    {#if plan && (step === 'ready' || step === 'acknowledge')}
      <Button tone="primary" data-publish isBusy={isBusy}
        disabledReason={step === 'acknowledge' && !acknowledged ? t('app.config_drafts.acknowledge_first') : undefined}
        onclick={publish}>{t('app.config_drafts.publish')}</Button>
    {:else}
      <Button tone="primary" data-review isBusy={isBusy} onclick={review}>{t('app.config_drafts.review')}</Button>
    {/if}
  </Inline>
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .lines { margin: 0; padding-inline-start: var(--sp-300); display: grid; gap: var(--sp-100); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
