<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Checkbox from './Checkbox.svelte';
  import Stack from './Stack.svelte';

  interface Props {
    mode?: 'states' | 'unavailable';
    isLong?: boolean;
  }

  const { mode = 'states', isLong = false }: Props = $props();

  let on = $state(true);
  let off = $state(false);
  let some = $state(false);

  const label = $derived(
    isLong
      ? 'Benachrichtigungen für dieses Arbeitspaket und alle darunter liegenden Aktivitäten'
      : 'Notify me about this work package',
  );
</script>

<Stack gap="200">
  {#if mode === 'unavailable'}
    <Checkbox
      label="Notify me about this work package"
      disabledReason="Reminders need an email address. Add one in your account."
    />
  {:else}
    <Checkbox bind:checked={on} label={label} />
    <Checkbox bind:checked={off} label="Include archived entries" hint="Archived entries stay searchable either way." />
    <Checkbox bind:checked={some} isIndeterminate label="All activities in this work package" />
  {/if}
</Stack>
