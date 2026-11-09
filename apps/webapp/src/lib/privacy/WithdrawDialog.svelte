<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Withdraw a consent…* as a Dialog M (Art. 7(3)): the purpose, and whose consent - the reader's
  // own unless somebody is chosen.
  import { Button, Dialog, Input, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { privacy } from '../data/privacy.svelte.ts';
  import { isComplete, type Missing } from '../forms/missing.ts';
  import { marked } from '../forms/press.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import PersonPicker from '../people/PersonPicker.svelte';
  import { renderProblem } from '../problem.ts';

  let { isOpen = $bindable(false) }: { isOpen: boolean } = $props();

  let withdrawPurpose = $state('');
  let withdrawAccount = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let missing = $state<Missing>({});
  let form = $state<HTMLFormElement | undefined>(undefined);

  async function withdraw(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = marked(event, [{ kind: 'field', key: 'purpose', code: 'app.forms.required', isFilled: withdrawPurpose.trim() !== '' }]);
    if (!isComplete(missing)) return;
    failure = undefined;
    isWorking = true;
    try {
      await privacy.withdraw(withdrawPurpose.trim(), withdrawAccount.trim() || undefined);
      announcer.say(t('app.privacy.withdrawn_announced'));
      isOpen = false;
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.privacy.withdraw_title')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { withdrawPurpose = ''; withdrawAccount = ''; failure = undefined; missing = {}; }}
  primary={{ label: t('app.privacy.withdraw'), isBusy: isWorking, busyLabel: t('app.privacy.withdrawing'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={withdraw}>
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <p class="quiet">{t('app.privacy.withdraw_note')}</p>
      <Input
        label={t('app.privacy.purpose')}
        hint={t('app.privacy.purpose_hint')}
        bind:value={withdrawPurpose}
        error={missing.purpose && !withdrawPurpose.trim() ? t(missing.purpose) : undefined}
      />
      <PersonPicker label={t('app.privacy.withdraw_account')} hint={t('app.privacy.withdraw_account_hint')} noneLabel={t('app.person_picker.yourself')} bind:value={withdrawAccount} />
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
