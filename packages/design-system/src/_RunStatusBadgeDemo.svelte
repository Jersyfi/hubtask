<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper: the wording of each status is resolved text, so the demo is where it
  // is written.

  import Inline from './Inline.svelte';
  import RunStatusBadge from './RunStatusBadge.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'all' }: { mode?: 'all' | 'dry' | 'unknown' } = $props();

  const wording: Record<string, string> = {
    RUNNING: 'Running',
    WAITING: 'Waiting',
    SUCCEEDED: 'Succeeded',
    SKIPPED: 'Skipped',
    FAILED: 'Failed',
    ABORTED_LOOP: 'Stopped: it triggered itself',
    THROTTLED: 'Held back',
  };

  const statuses = Object.keys(wording);
</script>

{#if mode === 'dry'}
  <Stack gap="100">
    {#each ['SUCCEEDED', 'SKIPPED', 'FAILED'] as status (status)}
      <Inline gap="100">
        <RunStatusBadge {status} label={wording[status]} />
        <RunStatusBadge {status} label={wording[status]} isDryRun dryRunLabel="Dry run" />
      </Inline>
    {/each}
  </Stack>
{:else if mode === 'unknown'}
  <Inline gap="100">
    <!-- A status this build has never seen. No wording for it either, so the badge shows the
         server's own token. -->
    <RunStatusBadge status="QUARANTINED" />
    <RunStatusBadge status="QUARANTINED" isDryRun dryRunLabel="Dry run" />
  </Inline>
{:else}
  <Stack gap="100">
    {#each statuses as status (status)}
      <div><RunStatusBadge {status} label={wording[status]} /></div>
    {/each}
  </Stack>
{/if}
