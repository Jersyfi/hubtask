<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Retiring an agent cannot be undone - a retired agent is never put live again - so it is asked
  // in a Dialog S (page-system §5.1), from a row's ⋯ and from the record's danger zone.

  import { Button, Dialog, Stack } from '@hubtask/design-system/components';
  import type { Agent } from '@hubtask/sync-engine';

  import { agents } from '../data/agents.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    agent: Agent | undefined;
    onclose: () => void;
  }

  const { agent, onclose }: Props = $props();

  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);

  async function retire() {
    if (!agent) return;
    isWorking = true;
    failure = undefined;
    try {
      await agents.press(agent.id, 'retire');
      toast.show({ text: t('app.agents.pressed.retire', { name: agent.name }), tone: 'success' });
      onclose();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog size="s" isOpen={agent !== undefined} title={t('app.agents.retire_title', { name: agent?.name ?? '' })} dismissLabel={t('app.agents.cancel')} onClose={onclose}>
  <Stack gap="150">
    <p class="cost">{t('app.agents.retire_cost')}</p>
    {#if failure}<FailureNote problem={failure} />{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={onclose}>{t('app.agents.cancel')}</Button>
    <Button tone="danger" isBusy={isWorking} busyLabel={t('app.agents.working')} onclick={() => void retire()}>{t('app.agents.press.retire')}</Button>
  {/snippet}
</Dialog>

<style>
  .cost { margin: 0; color: var(--text-secondary); }
</style>
