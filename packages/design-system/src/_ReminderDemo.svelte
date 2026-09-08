<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The caller's half: the presets are this client's words for common offsets, the channels come
  // from the installation, and the specification the editor produces is shown so a reader of the
  // workbench can see the two paths agree on one grammar.

  import ReminderEditor, { type ReminderChannel } from './ReminderEditor.svelte';
  import AssigneeControl from './AssigneeControl.svelte';

  const { mode = 'preset' }: { mode?: 'preset' | 'relative' | 'absolute' | 'recipients' } = $props();

  const presets = [
    { value: 'REL:-PT10M', label: 'Ten minutes before' },
    { value: 'REL:-PT1H', label: 'An hour before' },
    { value: 'REL:-P1D', label: 'A day before' },
  ];

  // As an installation reports them: the one it sends on, one it offers, and one this client has
  // no phrase for.
  const channels: ReminderChannel[] = [
    { value: 'EMAIL', label: 'Email' },
    { value: 'PUSH', label: 'Push notification' },
    { value: 'MATRIX' },
  ];

  const people = [
    { id: 'p1', name: 'Amelie Fischer' },
    { id: 'p2', name: 'Jonas Brandt' },
  ];

  let value = $state('REL:-PT1H');
  let selectedChannels = $state<readonly string[]>(['EMAIL']);
  let named = $state<readonly string[]>([]);

  $effect(() => {
    value =
      mode === 'absolute' ? 'ABS:2026-09-16T08:00' : mode === 'relative' ? 'REL:-P2W' : 'REL:-PT1H';
  });
</script>

<div>
  <ReminderEditor
    label="Remind me"
    {value}
    {presets}
    customLabel="Something else"
    beforeLabel="Before the due date"
    atLabel="At a fixed time"
    amountLabel="How long"
    unitLabel="Unit"
    dateLabel="Date"
    timeLabel="Time"
    unitLabels={{ M: 'minutes', H: 'hours', D: 'days', W: 'weeks' }}
    channelsLabel="Send it by"
    {channels}
    {selectedChannels}
    unknownChannelLabel="This client has no name for this channel yet."
    recipientsLabel="Remind"
    defaultRecipientsLabel="The assignee and the members, resolved when the reminder fires."
    onChange={(next) => {
      value = next;
    }}
    onChannelsChange={(next) => {
      selectedChannels = next;
    }}
    recipients={mode === 'recipients' ? picker : undefined}
  />

  <p class="produced"><code>{value}</code></p>
</div>

{#snippet picker()}
  <AssigneeControl
    label="Recipients"
    candidates={people}
    selection="multiple"
    selected={named}
    filterLabel="Filter people"
    emptyLabel="Nobody here can be reminded yet."
    noMatchLabel="No one matches that. Try part of a name."
    chosenLabel="Reminding"
    unassignedLabel="Nobody named yet"
    onSelect={(ids) => {
      named = ids;
    }}
  />
{/snippet}

<style>
  .produced { margin-block-start: var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }
</style>
