<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Run on…* (UC-AGT-07 check 7): the one thing the job works on - an entry or a hub - so a
  // Dialog M (page-system §5.1), its picker the one list in it.

  import { Button, Dialog, Select, Stack, type SelectOption } from '@hubtask/design-system/components';
  import type { Agent, WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from '../entries/EntryPicker.svelte';
  import { agents, jobTarget } from '../data/agents.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    agent: Agent;
    isOpen: boolean;
    onclose: () => void;
  }

  const { agent, isOpen, onclose }: Props = $props();

  const target = $derived(jobTarget(agent.kind));
  let chosenEntry = $state<WorkItem | undefined>(undefined);
  let chosenHub = $state('');
  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);
  const hubOptions = $derived<SelectOption[]>(containers.hubs.map((each) => ({ value: each.id, label: each.name })));
  const runTarget = $derived(target === 'HUB' ? chosenHub : chosenEntry?.id);

  async function run() {
    if (!runTarget) return;
    isWorking = true;
    failure = undefined;
    try {
      await agents.startJob(agent.id, runTarget);
      toast.show({ text: t('app.agents.job_started'), tone: 'success' });
      chosenEntry = undefined;
      chosenHub = '';
      onclose();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog size="m" {isOpen} title={t('app.agents.run_on_named', { name: agent.name })} dismissLabel={t('app.agents.cancel')} onClose={onclose}>
  <Stack gap="150">
    {#if target === 'HUB'}
      <Select label={t('app.agents.target_hub')} options={hubOptions} bind:value={chosenHub} />
    {:else}
      <p class="meta">{t('app.agents.target_entry')}</p>
      <EntryPicker chosen={chosenEntry} onchoose={(item) => (chosenEntry = item)} />
    {/if}
    {#if failure}<FailureNote problem={failure} />{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={onclose}>{t('app.agents.cancel')}</Button>
    <Button tone="primary" isBusy={isWorking} busyLabel={t('app.agents.working')} disabledReason={runTarget ? undefined : t('app.agents.target_needed')} onclick={() => void run()}>{t('app.agents.run')}</Button>
  {/snippet}
</Dialog>

<style>
  .meta { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
