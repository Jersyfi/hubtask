<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Input from './Input.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'invalid' | 'unavailable' } = $props();

  let title = $state('Review the quote');
  let search = $state('');
</script>

<Stack gap="300" class="form">
  {#if mode === 'invalid'}
    <Input label="Title" bind:value={title} error="A title needs at least one character." isRequired />
  {:else if mode === 'unavailable'}
    <Input
      label="Title"
      bind:value={title}
      disabledReason="This entry is archived. Restore it to change the title."
    />
  {:else}
    <Input label="Title" bind:value={title} hint="What the task is, in the words the team uses." isRequired />
    <Input label="Search" icon="search" bind:value={search} placeholder="Find a task" size="sm" />
  {/if}
</Stack>

<style>
  :global(.form) { max-width: 44ch; }
</style>
