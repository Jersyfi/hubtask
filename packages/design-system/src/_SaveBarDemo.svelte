<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A form a caller holds: its saved values and the edited ones, and the count of what differs -
  // resolved here with its plural form, because a component writes no sentence.

  import Button from './Button.svelte';
  import Input from './Input.svelte';
  import SaveBar from './SaveBar.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'form' }: { mode?: 'form' | 'saving' } = $props();

  const saved = { name: 'Kitchen', owner: 'Amelie Fischer', budget: '12000', supplier: 'Nordhaus GmbH', room: 'Ground floor' };
  let edited = $state({ ...saved });
  let isPressed = $state(false);
  const isSaving = $derived(mode === 'saving' || isPressed);

  const changes = $derived(Object.keys(saved).filter((key) => edited[key as keyof typeof saved] !== saved[key as keyof typeof saved]).length);
  const changesLabel = $derived(changes === 0 ? undefined : changes === 1 ? '1 change' : `${changes} changes`);

  /** What the layout test presses: one change, without typing. */
  function change() {
    edited = { ...edited, name: edited.name === saved.name ? 'Kitchen and hall' : saved.name };
  }
</script>

<div class="area" data-scroll-area="">
  <Stack gap="200">
    <div><Button tone="subtle" size="sm" data-driver="" onclick={change}>Change the name</Button></div>
    <Input label="Name" bind:value={edited.name} />
    <Input label="Owner" bind:value={edited.owner} />
    <Input label="Budget" bind:value={edited.budget} />
    <Input label="Supplier" bind:value={edited.supplier} />
    <Input label="Room" bind:value={edited.room} data-last="" />
  </Stack>
  <SaveBar
    label="Unsaved changes"
    changesLabel={mode === 'saving' ? '2 changes' : changesLabel}
    discardLabel="Discard"
    saveLabel="Save"
    savingLabel="Saving"
    {isSaving}
    onDiscard={() => (edited = { ...saved })}
    onSave={() => (isPressed = true)}
  />
</div>

<style>
  /* A scroll area shorter than the form, so the bar has something to stand over. */
  .area {
    max-block-size: calc(var(--layout-bottombar-height) * 5);
    overflow-y: auto;
    padding: var(--sp-100);
  }
</style>
