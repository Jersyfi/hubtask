<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What an agent's apply does (D-16) as a Dialog M: read from the document in force, set by a person.
  import { Button, Dialog, Select, Stack } from '@hubtask/design-system/components';

  import { config } from '../data/config.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  type AgentApply = 'ALLOWED' | 'APPROVAL' | 'NEVER';

  interface Props {
    isOpen: boolean;
    agentApply: string | undefined;
    mayChange: boolean;
  }

  let { isOpen = $bindable(false), agentApply, mayChange }: Props = $props();

  let draft = $state('');
  let busy = $state(false);
  $effect(() => {
    if (isOpen) draft = agentApply ?? '';
  });
  const options = $derived((['APPROVAL', 'ALLOWED', 'NEVER'] as const).map((value) => ({ value, label: t('app.changes.agent_apply_option', { value }) })));

  async function save(): Promise<void> {
    if (draft === '' || draft === agentApply) {
      isOpen = false;
      return;
    }
    busy = true;
    try {
      await config.setAgentApply(draft as AgentApply);
      toast.show({ text: t('app.changes.agent_apply_saved'), tone: 'success' });
      isOpen = false;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      busy = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.changes.agent_apply')}
  bind:isOpen
  dismissLabel={t('app.changes.cancel')}
  primary={mayChange ? { label: t('app.changes.agent_apply_save'), isBusy: busy, onclick: () => void save() } : undefined}
>
  <Stack gap="150">
    <p class="quiet" data-agent-apply>{t('app.changes.agent_apply_explain')}</p>
    {#if mayChange}
      <Select label={t('app.changes.agent_apply')} bind:value={draft} {options} />
    {:else if agentApply}
      <p>{t('app.changes.agent_apply_option', { value: agentApply })}</p>
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.changes.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet, p { margin: 0; }
  .quiet { color: var(--text-secondary); }
</style>
