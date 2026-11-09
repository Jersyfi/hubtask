<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Record a request* as a Dialog M (page-system §9.3): the right asked for, who asked - by address
  // or by account -, and where an archive goes for a kind that writes one; the day it arrived, its
  // own deadline and the notes under *Advanced*. **`INSTALLATION` scope is not offered**: it
  // crosses the tenant boundary and is the operator's path.
  import { Button, Dialog, Disclosure, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { backup } from '../data/backup.svelte.ts';
  import { KINDS, deadlineOfDay, earliestDeadline, privacy, producesArchive, receiptOf, type Kind } from '../data/privacy.svelte.ts';
  import { isComplete, type Missing } from '../forms/missing.ts';
  import { marked } from '../forms/press.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import PersonPicker from '../people/PersonPicker.svelte';
  import { renderProblem } from '../problem.ts';
  import { kindWord, today, zoneOf } from './words.svelte.ts';

  let { isOpen = $bindable(false) }: { isOpen: boolean } = $props();

  let draftKind = $state<string>('ACCESS');
  let draftEmail = $state('');
  let draftAccount = $state('');
  let draftTarget = $state('');
  let draftNotes = $state('');
  let draftDueOn = $state('');
  let draftReceivedOn = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let missing = $state<Missing>({});
  let form = $state<HTMLFormElement | undefined>(undefined);

  function reset(): void {
    draftKind = 'ACCESS';
    draftEmail = '';
    draftAccount = '';
    draftTarget = '';
    draftNotes = '';
    draftDueOn = '';
    draftReceivedOn = '';
    failure = undefined;
    missing = {};
  }

  function close(): void {
    isOpen = false;
  }

  const guard = new DiscardGuard(() => draftEmail.trim() !== '' || draftAccount !== '' || draftNotes.trim() !== '' || draftDueOn !== '' || draftReceivedOn !== '', close);

  async function record(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = marked(event, [
      { kind: 'field', key: 'subject', code: 'app.privacy.subject_needed', isFilled: draftEmail.trim() !== '' || draftAccount.trim() !== '' },
    ]);
    if (!isComplete(missing)) return;
    failure = undefined;
    isWorking = true;
    try {
      const zone = zoneOf();
      await privacy.record({
        kind: draftKind as Kind,
        ...(draftAccount.trim() ? { subject_account_id: draftAccount.trim() } : {}),
        ...(draftEmail.trim() ? { subject_email: draftEmail.trim() } : {}),
        ...(draftTarget ? { target_id: draftTarget } : {}),
        ...(draftNotes.trim() ? { notes: draftNotes.trim() } : {}),
        ...receiptOf(draftReceivedOn, today()),
        ...(deadlineOfDay(draftDueOn, zone) ? { due_at: deadlineOfDay(draftDueOn, zone) } : {}),
      });
      announcer.say(t('app.privacy.recorded_announced'));
      reset();
      close();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.privacy.record_title')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={guard.dismiss}
  onClose={reset}
  primary={{ label: t('app.privacy.record'), isBusy: isWorking, busyLabel: t('app.privacy.recording'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={record}>
    <Stack gap="150">
      {#if failure}
        <FailureNote problem={failure}>
          {#if failure.fields.size > 0}
            <Stack gap="050">{#each [...failure.fields] as [path, message] (path)}<span>{message}</span>{/each}</Stack>
          {/if}
        </FailureNote>
      {/if}
      <Select
        label={t('app.privacy.kind')}
        hint={t(`app.privacy.kind_${draftKind.toLowerCase()}_note`)}
        bind:value={draftKind}
        options={KINDS.map((kind) => ({ value: kind, label: kindWord(kind) }))}
      />
      <Input
        label={t('app.privacy.subject_email')}
        hint={t('app.privacy.subject_email_hint')}
        type="email"
        bind:value={draftEmail}
        error={missing.subject && !draftEmail.trim() && !draftAccount.trim() ? t(missing.subject) : undefined}
      />
      <PersonPicker label={t('app.privacy.subject_account')} hint={t('app.privacy.subject_account_hint')} noneLabel={t('app.person_picker.choose')} bind:value={draftAccount} />
      {#if producesArchive(draftKind as Kind)}
        <!-- Required before such a case can start: a copy of somebody's data has to be put
             somewhere, and this system writes archives to configured targets. -->
        <Select
          label={t('app.privacy.target')}
          hint={t('app.privacy.target_hint')}
          bind:value={draftTarget}
          placeholder={t('app.privacy.choose_target')}
          options={backup.all.map((target) => ({ value: target.id, label: target.name }))}
        />
      {/if}
      <Disclosure tone="quiet" label={t('app.privacy.record_more')} description={t('app.privacy.record_more_says')}>
        <Stack gap="150">
          <!-- The day the request arrived: the month counts from it, in the workspace's zone. -->
          <Input label={t('app.privacy.received_on')} hint={t('app.privacy.received_on_hint')} type="date" max={today()} bind:value={draftReceivedOn} />
          <!-- Its own deadline, where the controller owes a sooner answer than the month. -->
          <Input label={t('app.privacy.deadline')} hint={t('app.privacy.deadline_hint')} type="date" min={earliestDeadline(draftReceivedOn, today())} bind:value={draftDueOn} />
          <Textarea label={t('app.privacy.notes')} bind:value={draftNotes} rows={2} />
        </Stack>
      </Disclosure>
      <p class="quiet small">{t('app.privacy.record_note')}</p>
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
</style>
