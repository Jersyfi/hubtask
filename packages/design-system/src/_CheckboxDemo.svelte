<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import InContext from './_InContext.svelte';
  import Checkbox from './Checkbox.svelte';
  import Stack from './Stack.svelte';

  interface Props {
    mode?: 'states' | 'unavailable' | 'row' | 'table' | 'busy';
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

{#if mode === 'row'}
  <InContext context="row" reason="This entry is archived. Restore it to tick it off.">
    {#snippet subject({ reason, onclick })}
      <Checkbox label="Completed" disabledReason={reason} {onclick} />
    {/snippet}
  </InContext>
{:else if mode === 'table'}
  <InContext context="table" reason="This entry is archived. Restore it to tick it off.">
    {#snippet subject({ reason, onclick })}
      <Checkbox label="Completed" isLabelHidden disabledReason={reason} {onclick} />
    {/snippet}
  </InContext>
{:else if mode === 'busy'}
  <InContext context="row" reason="" driverLabel="Switch busy">
    {#snippet subject({ onclick, isOn })}
      <Checkbox label="Completed" isBusy={isOn} busyLabel="Saving" {onclick} />
    {/snippet}
  </InContext>
{:else}
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
{/if}
