<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import InContext from './_InContext.svelte';
  import Inline from './Inline.svelte';
  import Stack from './Stack.svelte';

  interface Props {
    mode?: 'tones' | 'busy' | 'unavailable' | 'row' | 'table' | 'busy-in-place';
    isLong?: boolean;
  }

  const { mode = 'tones', isLong = false }: Props = $props();

  const labels = $derived(
    isLong
      ? { create: 'Aufgabe erstellen', save: 'Änderungen speichern', more: 'Weitere Möglichkeiten', del: 'Endgültig löschen' }
      : { create: 'Create task', save: 'Save', more: 'More', del: 'Delete permanently' },
  );
</script>

{#if mode === 'row' || mode === 'table'}
  <InContext context={mode} reason="Only this collection's templates are changed here.">
    {#snippet subject({ reason, onclick })}
      <Button tone="secondary" icon="pencil" disabledReason={reason} {onclick}>Edit</Button>
    {/snippet}
  </InContext>
{:else if mode === 'busy-in-place'}
  <InContext context="row" reason="" driverLabel="Switch busy">
    {#snippet subject({ onclick, isOn })}
      <Button tone="secondary" isBusy={isOn} busyLabel="Saving" {onclick}>Save changes</Button>
      <Button tone="secondary" icon="check" isBusy={isOn} busyLabel="Saving" {onclick}>Save changes</Button>
    {/snippet}
  </InContext>
{:else if mode === 'busy'}
  <Inline gap="150" align="center">
    <Button tone="primary" isBusy busyLabel="Creating">Creating…</Button>
    <Button tone="secondary" isBusy busyLabel="Saving">Saving…</Button>
  </Inline>
{:else if mode === 'unavailable'}
  <Stack gap="200">
    <Button tone="primary" disabledReason="This collection does not accept new tasks.">
      {labels.create}
    </Button>
    <Button tone="danger" disabledReason="Deleting needs the workspace owner's permission.">
      {labels.del}
    </Button>
  </Stack>
{:else}
  <Stack gap="250">
    <Inline gap="150" align="center">
      <Button tone="primary" icon="plus">{labels.create}</Button>
      <Button tone="secondary">{labels.save}</Button>
      <Button tone="subtle" icon="ellipsis">{labels.more}</Button>
      <Button tone="danger" icon="trash">{labels.del}</Button>
    </Inline>
    <Inline gap="150" align="center">
      <Button tone="primary" size="sm" icon="plus">{labels.create}</Button>
      <Button tone="secondary" size="sm">{labels.save}</Button>
      <Button tone="subtle" size="sm">{labels.more}</Button>
    </Inline>
  </Stack>
{/if}
