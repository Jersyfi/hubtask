<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import InContext from './_InContext.svelte';
  import Button from './Button.svelte';
  import Toast from './Toast.svelte';
  import Switch from './Switch.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'resting' }: { mode?: 'resting' | 'unavailable' | 'row' | 'table' | 'busy' | 'saving' } = $props();

  let celebrations = $state(true);
  let digest = $state(false);

  // The save a caller would send, stood in for by a pause: busy while it runs, then confirmed.
  let isSaving = $state(false);
  let isSaved = $state(false);
  function save() {
    isSaving = true;
    isSaved = false;
    setTimeout(() => {
      isSaving = false;
      isSaved = true;
    }, 900);
  }
  function undo() {
    digest = !digest;
    isSaved = false;
  }
</script>

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="Your workspace switches this on for everybody.">
    {#snippet subject({ reason, onclick })}
      <Switch label="Celebrate a completed task" disabledReason={reason} {onclick} />
    {/snippet}
  </InContext>
{:else if mode === 'busy'}
  <InContext context="row" reason="" driverLabel="Switch busy">
    {#snippet subject({ onclick, isOn })}
      <Switch label="Celebrate a completed task" isBusy={isOn} busyLabel="Saving" {onclick} />
    {/snippet}
  </InContext>
{:else if mode === 'saving'}
  <Stack gap="250">
    <Switch
      label="Send a daily digest"
      bind:checked={digest}
      isBusy={isSaving}
      busyLabel="Saving"
      onchange={save}
    />
    {#if isSaved}
      <Toast tone="success" dismissLabel="Dismiss" onDismiss={() => (isSaved = false)}>
        Saved.
        {#snippet action()}<Button tone="subtle" size="sm" onclick={undo}>Undo</Button>{/snippet}
      </Toast>
    {/if}
  </Stack>
{:else}
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
{/if}
