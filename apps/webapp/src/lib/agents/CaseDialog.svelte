<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A case of an agent's evaluation set (UC-AGT-04 check 6): three fields in one group, a Dialog M
  // (page-system §5.1). A refusal is said inside, which keeps what was typed.

  import { Button, Dialog, Input, Stack, Textarea } from '@hubtask/design-system/components';

  import { agents, expectedFrom } from '../data/agents.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    agentId: string;
    isOpen: boolean;
    onclose: () => void;
  }

  const { agentId, isOpen, onclose }: Props = $props();

  let caseTitle = $state('');
  let caseNotes = $state('');
  let caseExpected = $state('{}');
  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);
  const parsed = $derived(expectedFrom(caseExpected));
  const reason = $derived(
    !caseTitle.trim() ? t('app.agents.case_needs_title')
    : 'problem' in parsed ? t(`app.agents.expected_${parsed.problem}`)
    : undefined,
  );

  async function add() {
    if (!('value' in parsed)) return;
    isWorking = true;
    failure = undefined;
    try {
      await agents.addCase(agentId, { title: caseTitle.trim(), notes: caseNotes, expected: parsed.value });
      toast.show({ text: t('app.agents.case_added'), tone: 'success' });
      caseTitle = '';
      caseNotes = '';
      caseExpected = '{}';
      onclose();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog size="m" {isOpen} title={t('app.agents.add_case')} dismissLabel={t('app.agents.cancel')} onClose={onclose}>
  <Stack gap="150">
    <Input label={t('app.agents.case_title')} bind:value={caseTitle} maxlength={500} />
    <Textarea label={t('app.agents.case_notes')} rows={3} bind:value={caseNotes} />
    <Textarea label={t('app.agents.case_expected')} hint={t('app.agents.case_expected_hint')} rows={4} bind:value={caseExpected} />
    {#if failure}<FailureNote problem={failure} />{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={onclose}>{t('app.agents.cancel')}</Button>
    <Button tone="primary" isBusy={isWorking} busyLabel={t('app.agents.working')} disabledReason={reason} onclick={() => void add()}>{t('app.agents.add_case')}</Button>
  {/snippet}
</Dialog>
