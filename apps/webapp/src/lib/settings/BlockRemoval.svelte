<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Removing a block of the configuration through its plan (user-experience.md §5.9): the REMOVE
  // is planned first and shown with its reach and its refusals, acknowledged where it changes what
  // people may do, applied by the plan's hash - versioned like every other change - and *Undo*
  // plans the removed block back.

  import { untrack } from 'svelte';
  import { Banner, Button, Checkbox, Inline, Spinner, Stack } from '@hubtask/design-system/components';
  import type { ConfigPlan } from '@hubtask/sync-engine';

  import { config } from '../data/config.svelte.ts';
  import { publishStep, removalDocument, removedBlock } from '../data/configdrafts.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import PlanReach from './PlanReach.svelte';

  interface Props {
    section: string;
    key: string;
    /** The sentence the removal's toast says, and the button's words. */
    removed: string;
    confirm: string;
    keep: string;
    onDone: () => void;
  }

  let { section, key, removed, confirm, keep, onDone }: Props = $props();

  let plan = $state<ConfigPlan | undefined>(undefined);
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
      problem = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }

  $effect(() => untrack(() => void run(async () => {
    plan = await config.planDocument(removalDocument(section, key));
  })));

  async function undo(block: Record<string, unknown>) {
    const document = { [section]: { [key]: block } };
    const back = await config.planDocument(document);
    // Undo is the remover's own act, putting back exactly what they took a moment ago.
    await config.applyDocument(document, back, true);
  }

  const remove = () => run(async () => {
    if (!plan) return;
    const block = removedBlock(plan, section, key);
    await config.applyDocument(removalDocument(section, key), plan, acknowledged);
    toast.show({ text: removed, tone: 'success', undo: block ? () => undo(block) : undefined });
    onDone();
  });
</script>

<Stack gap="150">
  {#if !plan && !problem}
    <p class="quiet"><Spinner label={t('app.block_removal.planning')} /> <span>{t('app.block_removal.planning')}</span></p>
  {:else if plan && step === 'refused'}
    <Banner tone="warning" title={t('app.block_removal.refused')} />
    <ul class="lines" data-removal-refusals>
      {#each plan.refusals as refusal, index (index)}<li>{t(refusal.code, refusal.params ?? {})}</li>{/each}
    </ul>
  {:else if plan}
    <Banner tone="warning" title={t('app.block_removal.planned')} />
    <PlanReach reach={plan.reach} />
    {#if step === 'acknowledge'}
      <Checkbox label={t('app.config_drafts.acknowledge')} checked={acknowledged}
        onchange={(event: Event) => (acknowledged = (event.currentTarget as HTMLInputElement).checked)} />
    {/if}
  {/if}
  {#if problem}<p class="refusal" role="alert">{problem}</p>{/if}
  <Inline gap="150">
    {#if plan && (step === 'ready' || step === 'acknowledge')}
      <Button tone="danger" data-remove isBusy={isBusy}
        disabledReason={step === 'acknowledge' && !acknowledged ? t('app.config_drafts.acknowledge_first') : undefined}
        onclick={remove}>{confirm}</Button>
    {/if}
    <Button tone="subtle" onclick={onDone}>{keep}</Button>
  </Inline>
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); display: flex; gap: var(--sp-100); align-items: center; }
  .lines { margin: 0; padding-inline-start: var(--sp-300); display: grid; gap: var(--sp-100); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
