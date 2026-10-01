<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import CodeField from './CodeField.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'invalid' | 'eight' } = $props();

  let code = $state('482');
  let eight = $state('');
</script>

<Stack gap="300" class="codes">
  {#if mode === 'invalid'}
    <CodeField
      label="Code from your authenticator"
      bind:value={code}
      error="That code did not verify. Enter the current code from your authenticator."
    />
  {:else if mode === 'eight'}
    <CodeField
      label="Code from your authenticator"
      hint="Eight digits, the one showing now."
      bind:value={eight}
      length={8}
      groupOf={4}
    />
  {:else}
    <CodeField
      label="Code from your authenticator"
      hint="Six digits, the one showing now. You can paste it."
      bind:value={code}
    />
  {/if}
</Stack>

<style>
  :global(.codes) { max-width: 44ch; }
</style>
