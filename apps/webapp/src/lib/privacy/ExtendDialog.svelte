<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Extend the deadline…* as a Dialog M. Once, by law, and only where the server says it can be
  // (`extendable_until`, P-05): the form starts on that day. Recorded rather than sent - the
  // controller tells the person, and the case keeps the reason and the day they were told.
  import { Button, Dialog, Input, Select, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { EXTENSION_REASONS, extensionPayload, privacy, type Request } from '../data/privacy.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { day, today } from './words.svelte.ts';

  interface Props {
    request: Request | undefined;
    onClose: () => void;
  }

  const { request, onClose }: Props = $props();

  let extendDueOn = $state('');
  let extendReason = $state<string>('COMPLEXITY');
  let extendInformedOn = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => {
    if (!request) return;
    extendDueOn = request.extendable_until ?? '';
    extendReason = 'COMPLEXITY';
    extendInformedOn = today();
    failure = undefined;
  });

  async function extend(): Promise<void> {
    if (!request) return;
    const payload = extensionPayload({ dueOn: extendDueOn, reason: extendReason, informedOn: extendInformedOn });
    if (!payload) return;
    failure = undefined;
    isWorking = true;
    try {
      await privacy.extend(request.id, payload);
      announcer.say(t('app.privacy.extended_announced'));
      onClose();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  isOpen={request !== undefined}
  title={t('app.privacy.extend')}
  dismissLabel={t('app.workspace.cancel')}
  {onClose}
  primary={{ label: t('app.privacy.extend_confirm'), isBusy: isWorking, busyLabel: t('app.privacy.extending'), onclick: () => void extend() }}
>
  {#if request}
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <p class="quiet">{t('app.privacy.extend_note')}</p>
      <Input
        label={t('app.privacy.extend_until')}
        hint={t('app.privacy.extend_until_hint', { latest: day(request.extendable_until ?? '') })}
        type="date"
        max={request.extendable_until}
        bind:value={extendDueOn}
        isRequired
      />
      <Select
        label={t('app.privacy.extend_reason')}
        bind:value={extendReason}
        options={EXTENSION_REASONS.map((reason) => ({ value: reason, label: t(`app.privacy.extend_reason_${reason.toLowerCase()}`) }))}
      />
      <Input label={t('app.privacy.extend_informed')} hint={t('app.privacy.extend_informed_hint')} type="date" max={today()} bind:value={extendInformedOn} isRequired />
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
