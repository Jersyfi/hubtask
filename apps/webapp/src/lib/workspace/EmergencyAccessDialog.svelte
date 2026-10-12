<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace owner's emergency access to a private hub (ADR-0073 §4): what it does is said
  // before it is done - one hour, every member told at once with the reason, in the trail - and the
  // reason is asked for in the words the members will read. The proof is the step-up's own prompt,
  // asked only when the server demands it.

  import { Button, Dialog, Stack, Textarea } from '@hubtask/design-system/components';

  import { privateHubs } from '../data/privatehubs.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { TransportError } from '@hubtask/sync-engine';

  interface Props {
    isOpen: boolean;
    hubId: string;
    onopened: (hubId: string) => void;
  }

  let { isOpen = $bindable(false), hubId, onopened }: Props = $props();

  let reason = $state('');
  let isWorking = $state(false);
  let refusal = $state<string | undefined>(undefined);
  let reasonError = $state<string | undefined>(undefined);

  async function open() {
    isWorking = true;
    refusal = undefined;
    reasonError = undefined;
    try {
      await privateHubs.openHub(hubId, reason);
      isOpen = false;
      reason = '';
      onopened(hubId);
    } catch (cause) {
      if (cause instanceof TransportError) {
        const problem = renderProblem(cause, messages);
        reasonError = problem.fields.get('/reason');
        refusal = reasonError ? undefined : problem.message;
      } else {
        refusal = t('errors.internal');
      }
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog bind:isOpen title={t('app.private_hubs.emergency_title')} dismissLabel={t('app.private_hubs.emergency_cancel')}>
  <Stack gap="200">
    <p class="explain">{t('app.private_hubs.emergency_explain')}</p>
    <Textarea
      label={t('app.private_hubs.emergency_reason')}
      hint={t('app.private_hubs.emergency_reason_hint')}
      error={reasonError}
      isRequired
      maxlength={500}
      bind:value={reason}
    />
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  </Stack>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.private_hubs.emergency_cancel')}</Button>
    <Button tone="danger" onclick={open} isBusy={isWorking} disabledReason={reason.trim() === '' ? t('app.private_hubs.emergency_reason_hint') : undefined}>
      {t('app.private_hubs.emergency_confirm')}
    </Button>
  {/snippet}
</Dialog>

<style>
  .explain { margin: 0; }

  .refusal { margin: 0; color: var(--text-danger); }
</style>
