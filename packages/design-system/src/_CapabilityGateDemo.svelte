<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import CapabilityGate from './CapabilityGate.svelte';
  import Select from './Select.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'permitted' }: {
    mode?: 'permitted' | 'capability' | 'role' | 'pending';
  } = $props();

  let bucket = $state('doing');

  const buckets = [
    { value: 'todo', label: 'To do' },
    { value: 'doing', label: 'Doing' },
    { value: 'done', label: 'Done' },
  ];

  // The sentences are the application's — resolved message codes — and are written out here only
  // because a workbench has no catalogue. A component never writes one.
  const status = $derived(
    mode === 'permitted' ? 'permitted' : mode === 'pending' ? 'undetermined' : 'refused',
  );
  const reason = $derived(
    mode === 'capability'
      ? 'A work package is not on a board. Buckets apply to tasks directly in the collection.'
      : mode === 'role'
        ? 'Changing the board needs the workspace owner’s permission.'
        : undefined,
  );
</script>

<Stack gap="200">
  <CapabilityGate {status} {reason} pendingLabel="Reading what this installation supports…">
    <Select label="Bucket" options={buckets} bind:value={bucket} />
  </CapabilityGate>
</Stack>
