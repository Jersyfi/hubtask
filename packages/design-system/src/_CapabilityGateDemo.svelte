<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import InContext from './_InContext.svelte';
  import Button from './Button.svelte';
  import CapabilityGate from './CapabilityGate.svelte';
  import Select from './Select.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'permitted' }: {
    mode?: 'permitted' | 'capability' | 'role' | 'pending' | 'row' | 'table' | 'deciding-permitted' | 'deciding-refused';
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

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="Buckets apply to tasks directly in the collection.">
    {#snippet subject({ reason, onclick })}
      <CapabilityGate status={reason ? 'refused' : 'permitted'} {reason} isInline>
        <Button tone="secondary" icon="bucket" {onclick}>Move to a bucket</Button>
      </CapabilityGate>
    {/snippet}
  </InContext>
{:else if mode === 'deciding-permitted' || mode === 'deciding-refused'}
  <InContext context="row" reason="" driverLabel="Answer">
    {#snippet subject({ isOn })}
      <CapabilityGate
        status={isOn ? (mode === 'deciding-permitted' ? 'permitted' : 'refused') : 'undetermined'}
        reason="A work package is not on a board. Buckets apply to tasks directly in the collection."
        pendingLabel="Reading what this installation supports…"
      >
        <Select label="Bucket" options={buckets} value="doing" />
      </CapabilityGate>
    {/snippet}
  </InContext>
{:else}
<Stack gap="200">
  <CapabilityGate {status} {reason} pendingLabel="Reading what this installation supports…">
    <Select label="Bucket" options={buckets} bind:value={bucket} />
  </CapabilityGate>
</Stack>
{/if}
