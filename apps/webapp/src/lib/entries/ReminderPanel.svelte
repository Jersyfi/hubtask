<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // When somebody is told about this entry.
  //
  // **A relative reminder needs a due date**, and the control says so with the server's own
  // sentence rather than being missing. `fire_at` would have nothing to count from, which is the
  // contract's reasoning and the reader's too.
  //
  // **Who is reminded is worked out when it fires**, not when it is written. An empty recipient
  // list means the assignee and the members *at that moment* — so somebody added tomorrow is
  // reached tomorrow, and the panel says that rather than showing an empty list.
  //
  // **The state is read as a string.** The contract declares four — `LAPSED`, a reminder whose
  // moment passed while the data sat in an archive, is one of them — and a fifth from a newer
  // server still reads as words, because `t` humanises a code it has never met.
  //
  // **The bound is the manifest's.** `max_reminders_per_item` turns the add control off with the
  // reason, rather than letting the server refuse the twenty-sixth.

  import { untrack } from 'svelte';

  import {
    AssigneeControl,
    Badge,
    Button,
    CapabilityGate,
    ReminderEditor,
    Stack,
  } from '@hubtask/design-system/components';
  import type { Reminder, WorkItem } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { reminders } from '../data/reminders.svelte.ts';
  import {
    isPending,
    isRelative,
    mayAddAnother,
    relativeRefusal,
    reminderLimitOf,
  } from '../data/reminders.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { item, path }: { item: WorkItem; path: Path } = $props();

  const capability = $derived(supports(item.type, 'REMINDER'));
  const limit = $derived(reminderLimitOf(manifest.value?.limits as Record<string, unknown> | undefined));

  $effect(() => {
    const wanted = item.id;
    return untrack(() => reminders.open(wanted));
  });

  const held = $derived(reminders.of(item.id));
  const mayAdd = $derived(mayAddAnother(held.length, limit));

  /** The offsets this client has words for. A preset is a vocabulary; the offset is what travels. */
  const PRESETS = [
    { value: 'REL:PT0S', label: t('app.reminders.preset_at_time') },
    { value: 'REL:-PT10M', label: t('app.reminders.preset_10m') },
    { value: 'REL:-PT1H', label: t('app.reminders.preset_1h') },
    { value: 'REL:-P1D', label: t('app.reminders.preset_1d') },
    { value: 'REL:-P1W', label: t('app.reminders.preset_1w') },
  ];

  const channels = $derived(
    ((manifest.value?.notification_channels ?? ['EMAIL']) as readonly string[]).map((id) => id),
  );

  const candidateIds = $derived(people.candidates(path));
  const candidateList = $derived(
    candidateIds.map((id) => ({ id, name: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  let editing = $state<Reminder | undefined>(undefined);
  let spec = $state('REL:-PT1H');
  let chosenChannels = $state<readonly string[]>(['EMAIL']);
  let chosenRecipients = $state<readonly string[]>([]);
  let isSaving = $state(false);
  let isComposing = $state(false);
  let failure = $state<string | undefined>(undefined);

  /** Why this offset cannot be saved on this entry, or nothing. The server's own code. */
  const refusal = $derived(isRelative(spec) ? relativeRefusal(item) : undefined);

  $effect(() => {
    if (!isComposing) return;
    accounts.resolve(candidateIds);
  });

  function startNew() {
    editing = undefined;
    spec = 'REL:-PT1H';
    chosenChannels = ['EMAIL'];
    chosenRecipients = [];
    failure = undefined;
    isComposing = true;
  }

  function startEdit(reminder: Reminder) {
    editing = reminder;
    spec = reminder.offset_spec;
    chosenChannels = (reminder.channels ?? ['EMAIL']) as readonly string[];
    chosenRecipients = (reminder.recipients ?? []) as readonly string[];
    failure = undefined;
    isComposing = true;
  }

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    isSaving = true;
    failure = undefined;
    try {
      await work();
      isComposing = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  function save() {
    if (refusal) return;
    const current = editing;
    void attempt(async () => {
      const body = {
        offset_spec: spec,
        channels: chosenChannels as never,
        recipients: chosenRecipients as never,
      };
      if (current) await reminders.edit(item.id, current, body);
      else await reminders.add(item.id, body, crypto.randomUUID());
    });
  }

  function remove(reminder: Reminder) {
    void attempt(() => reminders.remove(item.id, reminder));
  }

  /** When it will fire, or the sentence for a relative one whose entry has no date. */
  function firesAt(reminder: Reminder): string {
    return reminder.fire_at
      ? t('app.reminders.fires_at', { when: formatDateTime(reminder.fire_at, messages.locale) })
      : t('app.reminders.fires_never');
  }
</script>

<CapabilityGate
  status={capability.status}
  reason={capability.status === 'refused' ? t(capability.code, capability.params) : undefined}
  pendingLabel={t('app.reminders.deciding')}
>
  <Stack gap="150">
    {#if held.length === 0}
      <p class="quiet">{t('app.reminders.none')}</p>
    {:else}
      <ul class="list">
        {#each held as reminder (reminder.id)}
          <li>
            <div class="row">
              <div>
                <span class="spec">{reminder.offset_spec}</span>
                <span class="detail">{firesAt(reminder)}</span>
              </div>
              <div class="actions">
                <!-- The state is the server's, read as a string: a code this client has never met
                     still reads as words rather than as a key. -->
                <Badge>{t(`app.reminders.state_${reminder.state}`)}</Badge>
                <Button
                  size="sm"
                  tone="secondary"
                  disabledReason={isPending(reminder)
                    ? undefined
                    : t('reminders.not_pending', { state: String(reminder.state) })}
                  onclick={() => startEdit(reminder)}
                >
                  {t('app.entries.edit')}
                </Button>
                <Button size="sm" tone="secondary" onclick={() => remove(reminder)}>
                  {t('app.reminders.delete')}
                </Button>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    {#if isComposing}
      <ReminderEditor
        label={t('app.reminders.editor')}
        value={spec}
        presets={PRESETS}
        customLabel={t('app.reminders.custom')}
        beforeLabel={t('app.reminders.before')}
        atLabel={t('app.reminders.at')}
        amountLabel={t('app.reminders.amount')}
        unitLabel={t('app.reminders.unit')}
        dateLabel={t('app.reminders.date')}
        timeLabel={t('app.reminders.time')}
        unitLabels={{
          M: t('app.reminders.unit_M'),
          H: t('app.reminders.unit_H'),
          D: t('app.reminders.unit_D'),
          W: t('app.reminders.unit_W'),
        }}
        channelsLabel={t('app.reminders.channels')}
        channels={channels as never}
        selectedChannels={chosenChannels}
        unknownChannelLabel={t('app.reminders.channel_unknown')}
        recipientsLabel={t('app.reminders.recipients')}
        defaultRecipientsLabel={t('app.reminders.default_recipients')}
        disabledReason={refusal ? t(refusal) : undefined}
        onChange={(next) => (spec = next)}
        onChannelsChange={(next) => (chosenChannels = next)}
      >
        {#snippet recipients()}
          <AssigneeControl
            label={t('app.reminders.recipients')}
            candidates={candidateList}
            selected={chosenRecipients}
            selection="multiple"
            filterLabel={t('app.bulk.assignee_filter')}
            emptyLabel={t('app.bulk.assignee_empty')}
            noMatchLabel={t('app.bulk.assignee_no_match')}
            chosenLabel={t('app.bulk.assignee_chosen')}
            unassignedLabel={t('app.reminders.default_recipients')}
            onSelect={(ids) => (chosenRecipients = ids)}
          />
        {/snippet}
      </ReminderEditor>

      {#if failure}<p class="failure">{failure}</p>{/if}

      <div class="actions">
        <Button
          isBusy={isSaving}
          busyLabel={t('app.workspace.saving')}
          disabledReason={refusal ? t(refusal) : undefined}
          onclick={save}
        >
          {t('app.reminders.save')}
        </Button>
        <Button tone="secondary" onclick={() => (isComposing = false)}>
          {t('app.workspace.cancel')}
        </Button>
      </div>
    {:else}
      <div>
        <Button
          size="sm"
          tone="secondary"
          disabledReason={mayAdd ? undefined : t('app.reminders.at_limit')}
          onclick={startNew}
        >
          {t('app.reminders.add')}
        </Button>
      </div>
      {#if failure}<p class="failure">{failure}</p>{/if}
    {/if}
  </Stack>
</CapabilityGate>

<style>
  .list { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-100); }

  .row { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .spec { display: block; font-family: var(--font-mono); }

  .detail { display: block; color: var(--text-secondary); font-size: var(--fs-075); }

  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
