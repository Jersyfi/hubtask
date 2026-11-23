<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A settings block with its advanced controls folded, and a refusal with its details for
  // support: the two places a person meets a fold.
  import Banner from './Banner.svelte';
  import Disclosure from './Disclosure.svelte';
  import Icon from './Icon.svelte';
  import Stack from './Stack.svelte';
  import Switch from './Switch.svelte';

  const { open = false, isRow = false, hasValue = false }: { open?: boolean; isRow?: boolean; hasValue?: boolean } = $props();
  let calendar = $state(false);
</script>

{#snippet chosen()}<Icon name="heart" size="sm" /><span>Heart</span>{/snippet}

{#if isRow}
  <Stack gap="200">
    <Disclosure label="Colour and symbol" tone="row" isOpen={open} value={hasValue ? chosen : undefined}>
      <Switch label="A colour of its own" />
      <Switch label="A symbol of its own" />
    </Disclosure>
  </Stack>
{:else}
<Stack gap="300">
  <Disclosure label="Advanced" description="Calendar time, escalation" isOpen={open}>
    <Switch label="Measure by calendar time instead of business hours" bind:checked={calendar} />
  </Disclosure>
  <Banner tone="danger" title="You cannot move entries in “Incidents and requests”.">
    <Disclosure label="Details for support" tone="quiet" isOpen={open}>
      <span>Reference <code>1ed9ffdb</code></span>
    </Disclosure>
  </Banner>
</Stack>
{/if}
