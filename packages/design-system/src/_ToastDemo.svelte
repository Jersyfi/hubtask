<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Input from './Input.svelte';
  import Stack from './Stack.svelte';
  import Toast from './Toast.svelte';

  const { mode = 'tones' }: { mode?: 'tones' | 'focus' } = $props();

  let shown = $state<number[]>([]);
  let next = $state(1);
</script>

{#if mode === 'focus'}
  <Stack gap="200">
    <!-- The rule, made checkable: type in the field, raise a toast, and the caret does not move.
         A toast that took focus would throw a keyboard user out of the field they were in - which
         is exactly the moment a save confirmation arrives. -->
    <Input label="Task title" value="Write the release notes" />
    <Button tone="primary" onclick={() => (shown = [...shown, next++])}>Save</Button>
    <Stack gap="100">
      {#each shown as id (id)}
        <Toast tone="success" dismissLabel="Dismiss" onDismiss={() => (shown = shown.filter((s) => s !== id))}>
          {#snippet action()}
            <Button size="sm" tone="subtle">Undo</Button>
          {/snippet}
          Task saved.
        </Toast>
      {/each}
    </Stack>
  </Stack>
{:else}
  <Stack gap="150">
    <Toast>Two people are editing this task.</Toast>
    <Toast tone="success" dismissLabel="Dismiss">
      {#snippet action()}
        <Button size="sm" tone="subtle">Undo</Button>
      {/snippet}
      Task moved to Done.
    </Toast>
    <Toast tone="warning">The reminder was set for a time that has passed.</Toast>
    <Toast tone="danger" dismissLabel="Dismiss">Could not reach the server. Retrying.</Toast>
  </Stack>
{/if}
