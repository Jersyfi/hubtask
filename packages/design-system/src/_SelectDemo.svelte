<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import InContext from './_InContext.svelte';
  import Select from './Select.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'gated' | 'grouped' | 'row' | 'table' | 'option' } = $props();

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

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="The type is set by the template this entry came from.">
    {#snippet subject({ reason, onclick })}
      <Select label="Type" options={plain} value="task" disabledReason={reason} {onclick} />
    {/snippet}
  </InContext>
{:else if mode === 'option'}
  <InContext context="row" reason="Not available in this workspace.">
    {#snippet subject({ reason })}
      <Select
        label="Type"
        options={[...plain, { value: 'milestone', label: 'Milestone', disabledReason: reason }]}
        value="task"
        describeUnavailable={(label, why) => `${label} – ${why}`}
      />
    {/snippet}
  </InContext>
{:else}
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
{/if}

<style>
  :global(.form) { max-width: 40ch; }
</style>
