<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Select from './Select.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'gated' | 'grouped' } = $props();

  let kind = $state('task');

  const plain = [
    { value: 'task', label: 'Task' },
    { value: 'work-package', label: 'Work package' },
    { value: 'activity', label: 'Activity' },
  ];

  const gated = [
    ...plain,
    { value: 'milestone', label: 'Milestone', disabledReason: 'Not available in this workspace.' },
  ];

  // A list long enough to scan by heading: the events a rule can start on, by what they are about.
  let eventType = $state('item.created');
  const groups = [
    {
      label: 'Entries',
      options: [
        { value: 'item.created', label: 'An entry is created' },
        { value: 'item.completed', label: 'An entry is completed' },
        { value: 'item.overdue', label: 'An entry becomes overdue' },
      ],
    },
    {
      label: 'Hubs and collections',
      options: [
        { value: 'container.created', label: 'A hub or collection is created' },
        { value: 'container.archived', label: 'A hub or collection is archived' },
      ],
    },
    { label: 'Comments', options: [{ value: 'comment.created', label: 'A comment is written' }] },
  ];
</script>

<Stack gap="300" class="form">
  {#if mode === 'grouped'}
    <Select label="Which event" options={[]} {groups} bind:value={eventType} hint="Grouped by what the event is about; the wire name of the chosen one stands under the field." />
  {:else}
    <Select
      label="Type"
      options={mode === 'gated' ? gated : plain}
      bind:value={kind}
      hint="What a type may carry is its capability profile, not a preference."
    />
  {/if}
</Stack>

<style>
  :global(.form) { max-width: 40ch; }
</style>
