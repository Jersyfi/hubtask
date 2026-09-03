<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Inline from './Inline.svelte';
  import Spinner from './Spinner.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'sizes' }: { mode?: 'sizes' | 'named' | 'inside' } = $props();
</script>

{#if mode === 'named'}
  <Inline gap="150" align="center">
    <Spinner label="Loading tasks" />
    <span>A region is waiting, and the only thing that knows it is this.</span>
  </Inline>
{:else if mode === 'inside'}
  <Inline gap="150" align="center">
    <Button tone="primary" isBusy busyLabel="Creating">Creating…</Button>
    <Button size="sm" isBusy busyLabel="Saving">Saving…</Button>
  </Inline>
{:else}
  <Stack gap="200">
    <Inline gap="200" align="center"><Spinner size="sm" /><Spinner size="md" /></Inline>
  </Stack>
{/if}
