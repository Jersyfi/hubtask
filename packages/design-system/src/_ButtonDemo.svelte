<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Inline from './Inline.svelte';
  import Stack from './Stack.svelte';

  interface Props {
    mode?: 'tones' | 'busy' | 'unavailable';
    isLong?: boolean;
  }

  const { mode = 'tones', isLong = false }: Props = $props();

  const labels = $derived(
    isLong
      ? { create: 'Aufgabe erstellen', save: 'Änderungen speichern', more: 'Weitere Möglichkeiten', del: 'Endgültig löschen' }
      : { create: 'Create task', save: 'Save', more: 'More', del: 'Delete permanently' },
  );
</script>

{#if mode === 'busy'}
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
      <Button tone="danger" icon="trash-2">{labels.del}</Button>
    </Inline>
    <Inline gap="150" align="center">
      <Button tone="primary" size="sm" icon="plus">{labels.create}</Button>
      <Button tone="secondary" size="sm">{labels.save}</Button>
      <Button tone="subtle" size="sm">{labels.more}</Button>
    </Inline>
  </Stack>
{/if}
