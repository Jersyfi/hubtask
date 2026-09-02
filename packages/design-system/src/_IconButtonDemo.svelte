<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';

  const { mode = 'toolbar' }: { mode?: 'toolbar' | 'unavailable' } = $props();
</script>

{#if mode === 'unavailable'}
  <Inline gap="100" align="center">
    <IconButton icon="pencil" label="Rename this task" disabledReason="You may read this collection but not change it." />
    <IconButton icon="trash-2" tone="danger" label="Delete this task" disabledReason="Deleting needs the workspace owner's permission." />
  </Inline>
{:else}
  <Inline gap="100" align="center">
    <IconButton icon="pencil" label="Rename this task" />
    <IconButton icon="copy" label="Duplicate this task" />
    <IconButton icon="archive" label="Archive this task" />
    <IconButton icon="ellipsis" label="More actions" />
    <IconButton icon="plus" tone="primary" label="Add a task" />
    <IconButton icon="trash-2" tone="danger" label="Delete this task" />
    <IconButton icon="check" label="Saving" isBusy />
  </Inline>
{/if}
