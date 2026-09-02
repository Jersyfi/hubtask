<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Radio from './Radio.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'gated' } = $props();

  let when = $state('completion');

  const plain = [
    { value: 'schedule', label: 'On the schedule', hint: 'The next entry appears whether or not this one is done.' },
    { value: 'completion', label: 'When this one is completed' },
  ];

  const gated = [
    ...plain,
    { value: 'manual', label: 'Only when asked', disabledReason: 'Not available for an activity.' },
  ];
</script>

<Stack gap="300">
  <Radio label="Repeat" options={mode === 'gated' ? gated : plain} bind:value={when} />
</Stack>
