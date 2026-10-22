<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import CheckboxGroup from './CheckboxGroup.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'missing' } = $props();

  const scopes = [
    { value: 'items:read', label: 'Read entries' },
    { value: 'items:write', label: 'Change entries' },
    { value: 'comments:write', label: 'Write comments' },
  ];

  let selected = $state<readonly string[]>([]);
  // Pressed with nothing ticked, the form marks the group - which is what this story starts in.
  let isMarked = $state(false);
  $effect(() => {
    selected = mode === 'missing' ? [] : ['items:read'];
    isMarked = mode === 'missing';
  });
</script>

<Stack gap="200">
  <CheckboxGroup
    label="What the token may do"
    hint="Tick only what the script needs."
    options={scopes}
    {selected}
    error={isMarked && selected.length === 0 ? 'Tick at least one. A token that may do nothing is no use to a script.' : undefined}
    onChange={(next) => (selected = next)}
  />
  <div>
    <Button onclick={() => (isMarked = selected.length === 0)}>Create token</Button>
  </div>
</Stack>
