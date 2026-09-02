<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Select from './Select.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'gated' } = $props();

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
</script>

<Stack gap="300" class="form">
  <Select
    label="Type"
    options={mode === 'gated' ? gated : plain}
    bind:value={kind}
    hint="What a type may carry is its capability profile, not a preference."
  />
</Stack>

<style>
  :global(.form) { max-width: 40ch; }
</style>
