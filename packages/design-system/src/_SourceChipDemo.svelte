<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import SourceChip from './SourceChip.svelte';
  import Stack from './Stack.svelte';
  import Switch from './Switch.svelte';

  const { mode = 'sources' }: { mode?: 'sources' | 'beside' } = $props();
</script>

{#if mode === 'beside'}
  <!-- Beside the name of the value it marks: a locked setting says who set it, where it is. -->
  <Stack gap="200">
    <div class="setting">
      <span class="name">Second factor for everybody</span>
      <SourceChip
        label="Installation, locked"
        isLocked
        detailsLabel="Where this comes from"
        value="On"
        source="Set by the installation for every workspace."
        whomToAsk="Ask your provider to change it."
      />
    </div>
    <Switch label="Second factor for everybody" checked disabledReason="Set by the installation for every workspace." />
  </Stack>
{:else}
  <Stack gap="150">
    <div class="setting"><span class="name">Password length</span><SourceChip label="Set here" detailsLabel="Where this comes from" value="12 characters" source="Set in this workspace." /></div>
    <div class="setting"><span class="name">Session length</span><SourceChip label="Default" detailsLabel="Where this comes from" value="14 days" source="Hubtask's default; nobody has changed it." /></div>
    <div class="setting"><span class="name">Storage per workspace</span><SourceChip label="Plan, locked" isLocked detailsLabel="Where this comes from" value="25 GB" source="Set by your plan, Team." whomToAsk="Ask the workspace owner about another plan." /></div>
    <div class="setting"><span class="name">Sign-in with a passkey</span><SourceChip label="Installation, locked" isLocked detailsLabel="Where this comes from" value="Allowed" source="Set by the installation for every workspace." whomToAsk="Ask your provider to change it." /></div>
  </Stack>
{/if}

<style>
  .setting { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .name { color: var(--text-primary); }
</style>
