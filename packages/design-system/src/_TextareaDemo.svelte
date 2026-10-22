<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import InContext from './_InContext.svelte';
  import Textarea from './Textarea.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'invalid' | 'row' | 'table' } = $props();

  let note = $state(
    'The quote arrives on Thursday. Nothing here is a finished sentence in a component — this is a value a caller passed in.',
  );
</script>

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="This entry is archived. Restore it to change the note.">
    {#snippet subject({ reason, onclick })}
      <Textarea label="Note" value="The quote arrives on Thursday." rows={2} disabledReason={reason} {onclick} />
    {/snippet}
  </InContext>
{:else}
<Stack gap="300" class="form">
  {#if mode === 'invalid'}
    <Textarea label="Note" bind:value={note} error="A note may be at most 4000 characters. Shorten it by 40." />
  {:else}
    <Textarea label="Note" bind:value={note} hint="Anything the next person needs and the title does not carry." />
  {/if}
</Stack>
{/if}

<style>
  :global(.form) { max-width: 52ch; }
</style>
