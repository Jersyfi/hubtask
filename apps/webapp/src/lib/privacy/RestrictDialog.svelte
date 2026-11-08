<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Restrict processing…* as a Dialog M (Art. 18): what it means for the person's sessions and for
  // the workspace's automations, the person, and why.
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

  let restrictAccount = $state('');
  let restrictReason = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let missing = $state<Missing>({});
  let form = $state<HTMLFormElement | undefined>(undefined);

  async function restrict(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = marked(event, [{ kind: 'field', key: 'restrict', code: 'app.forms.required', isFilled: restrictAccount.trim() !== '' }]);
    if (!isComplete(missing)) return;
    failure = undefined;
    isWorking = true;
    try {
      await privacy.restrict(restrictAccount.trim(), true, restrictReason.trim() || undefined);
      announcer.say(t('app.privacy.restricted_announced'));
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
  title={t('app.privacy.restrict_title')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { restrictAccount = ''; restrictReason = ''; failure = undefined; missing = {}; }}
  primary={{ label: t('app.privacy.restrict'), isBusy: isWorking, busyLabel: t('app.privacy.restricting'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={restrict}>
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <p class="quiet">{t('app.privacy.restrict_note')}</p>
      <PersonPicker
        label={t('app.privacy.restrict_account')}
        noneLabel={t('app.person_picker.choose')}
        bind:value={restrictAccount}
        error={missing.restrict && !restrictAccount.trim() ? t(missing.restrict) : undefined}
      />
      <Input label={t('app.privacy.restrict_reason')} bind:value={restrictReason} />
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
