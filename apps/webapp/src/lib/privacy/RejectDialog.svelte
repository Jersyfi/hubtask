<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Reject…* as a Dialog S: one field, the reason the person is given.
  import { Button, Dialog, Input, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { privacy, type Request } from '../data/privacy.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    request: Request | undefined;
    onClose: () => void;
  }

  const { request, onClose }: Props = $props();

  let rejectReason = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  async function reject(): Promise<void> {
    if (!request) return;
    failure = undefined;
    isWorking = true;
    try {
      await privacy.change(request.id, { status: 'REJECTED', rejection_reason: rejectReason.trim() });
      announcer.say(t('app.privacy.rejected_announced'));
      rejectReason = '';
      onClose();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="s"
  isOpen={request !== undefined}
  title={t('app.privacy.reject_title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={{ label: t('app.privacy.reject_confirm'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.privacy.rejecting'), onclick: () => void reject() }}
>
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <Input label={t('app.privacy.reject_reason')} hint={t('app.privacy.reject_reason_hint')} bind:value={rejectReason} isRequired />
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
