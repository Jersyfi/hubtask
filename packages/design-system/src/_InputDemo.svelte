<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import InContext from './_InContext.svelte';
  import Input from './Input.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'invalid' | 'unavailable' | 'row' | 'table' | 'range' | 'code' } = $props();

  let title = $state('Review the quote');
  let search = $state('');
  let host = $state('tasks-hr.corp.example');
</script>

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="This entry is archived. Restore it to change the title.">
    {#snippet subject({ reason, onclick })}
      <Input label="Title" value="Review the quote" disabledReason={reason} {onclick} />
    {/snippet}
  </InContext>
{:else if mode === 'range'}
  <InContext context="row" reason="The plan sets the volume; ask the workspace owner.">
    {#snippet subject({ reason, onclick })}
      <Input label="Volume" type="range" min="0" max="10" value="4" disabledReason={reason} {onclick} />
    {/snippet}
  </InContext>
{:else}
<Stack gap="300" class="form">
  {#if mode === 'invalid'}
    <Input label="Title" bind:value={title} error="A title needs at least one character." isRequired />
  {:else if mode === 'code'}
    <Input label="Host" bind:value={host} face="code" hint="The full name this workspace answers at." isRequired />
    <Input label="Short name" face="code" placeholder="tasks-hr" />
    <Input label="New address" face="code" value="familie-meier" suffix=".tasks.example.net" hint="Lower case letters, digits and hyphens." />
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
{/if}

<style>
  :global(.form) { max-width: 44ch; }
</style>
