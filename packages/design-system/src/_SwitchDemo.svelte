<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Switch from './Switch.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'unavailable' } = $props();

  let celebrations = $state(true);
  let digest = $state(false);
</script>

<Stack gap="250">
  {#if mode === 'unavailable'}
    <Switch
      label="Celebrate a completed task"
      disabledReason="This workspace has switched celebrations off for everybody."
    />
  {:else}
    <Switch bind:checked={celebrations} label="Celebrate a completed task" hint="Takes effect at once." />
    <Switch bind:checked={digest} label="Send a daily digest instead of single messages" />
  {/if}
</Stack>
