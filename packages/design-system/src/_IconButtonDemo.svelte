<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import IconButton from './IconButton.svelte';
  import InContext from './_InContext.svelte';
  import Inline from './Inline.svelte';

  const { mode = 'toolbar' }: { mode?: 'toolbar' | 'unavailable' | 'row' | 'table' | 'busy' } = $props();
</script>

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="This entry is already first.">
    {#snippet subject({ reason, onclick })}
      <IconButton icon="chevron-up" label="Move up" disabledReason={reason} {onclick} />
    {/snippet}
  </InContext>
{:else if mode === 'busy'}
  <InContext context="row" reason="This entry is already first." driverLabel="Switch busy">
    {#snippet subject({ onclick, isOn })}
      <IconButton icon="check" label="Save" isBusy={isOn} {onclick} />
    {/snippet}
  </InContext>
{:else}

{#if mode === 'unavailable'}
  <Inline gap="100" align="center">
    <IconButton icon="pencil" label="Rename this task" disabledReason="You may read this collection but not change it." />
    <IconButton icon="trash" tone="danger" label="Delete this task" disabledReason="Deleting needs the workspace owner's permission." />
  </Inline>
{:else}
  <Inline gap="100" align="center">
    <IconButton icon="pencil" label="Rename this task" />
    <IconButton icon="copy" label="Duplicate this task" />
    <IconButton icon="archive" label="Archive this task" />
    <IconButton icon="ellipsis" label="More actions" />
    <IconButton icon="plus" tone="primary" label="Add a task" />
    <IconButton icon="trash" tone="danger" label="Delete this task" />
    <IconButton icon="check" label="Saving" isBusy />
  </Inline>
{/if}
{/if}
