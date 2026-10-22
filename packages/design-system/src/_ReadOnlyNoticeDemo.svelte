<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import ReadOnlyNotice from './ReadOnlyNotice.svelte';
  import Stack from './Stack.svelte';
  import TaskRow from './TaskRow.svelte';
  import { provideReasonGroup } from './reason.ts';

  const { mode = 'archived' }: { mode?: 'archived' | 'role' | 'plan' | 'rows' } = $props();

  const archived = 'This collection is archived, so nothing in it can be changed. Ask whoever manages it to unarchive it.';
  // The rows' completion boxes are off for the area's reason: they point at the notice's sentence
  // and draw no bubble of their own, and are still refused when pressed.
  const instanceId = $props.id();
  const textId = `notice-${instanceId}`;
  provideReasonGroup(() => ({ id: textId, reason: mode === 'rows' ? archived : undefined }));

  let pressed = $state(0);
</script>

<Stack gap="200">
  {#if mode === 'archived'}
    <ReadOnlyNotice text="This collection is archived. Its entries can be read, not changed.">
      {#snippet action()}<Button tone="secondary" size="sm">Restore</Button>{/snippet}
    </ReadOnlyNotice>
  {:else if mode === 'role'}
    <ReadOnlyNotice text="You can read this hub. Ask its owner to change something." icon="eye" />
  {:else if mode === 'plan'}
    <ReadOnlyNotice text="Automation is not part of this workspace's plan. The rules below are kept as they were." />
  {:else}
    <ReadOnlyNotice text={archived} {textId} />
  {/if}
  {#if mode === 'rows'}
    <div class="list" data-pressed={pressed}>
      <TaskRow type="TASK" title="Order the tiles" completeLabel="Mark Order the tiles as done" completeDisabledReason={archived} onToggleComplete={() => pressed++} />
      <TaskRow type="TASK" title="Book the electrician" isCompleted completeLabel="Mark Book the electrician as not done" completeDisabledReason={archived} onToggleComplete={() => pressed++} />
    </div>
  {:else}
    <ul class="rows">
      <li>Order the tiles</li>
      <li>Book the electrician</li>
      <li>Measure the hallway</li>
    </ul>
  {/if}
</Stack>

<style>
  .rows { margin: 0; padding-inline-start: var(--sp-300); color: var(--text-primary); }
</style>
