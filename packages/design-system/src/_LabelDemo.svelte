<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Inline from './Inline.svelte';
  import LabelChip from './LabelChip.svelte';
  import LabelPicker, { type PickerLabel } from './LabelPicker.svelte';
  import Stack from './Stack.svelte';
  import { labelTokens } from '../dist/tokens.ts';

  const { mode = 'ten' }: { mode?: 'ten' | 'removable' | 'picker' | 'empty' | 'long' } = $props();

  const collectionLabels: PickerLabel[] = [
    { id: 'urgent', name: 'Urgent', colorToken: 'red', description: 'Blocks somebody else today.' },
    { id: 'electrics', name: 'Electrics', colorToken: 'amber' },
    { id: 'waiting', name: 'Waiting on a quote', colorToken: 'slate', description: 'Nothing to do until it arrives.' },
    { id: 'materials', name: 'Materials', colorToken: 'teal' },
    { id: 'cheap', name: 'Cheap win', colorToken: 'green', description: 'Under an hour, and visible.' },
  ];

  const long: PickerLabel[] = [
    { id: 'a', name: 'Elektroarbeiten im Fensterbereich', colorToken: 'amber' },
    { id: 'b', name: 'Wartet auf einen Kostenvoranschlag', colorToken: 'slate', description: 'Bis dahin gibt es nichts zu tun.' },
  ];

  let selected = $state(['urgent', 'materials']);
  let onEntry = $state(['urgent', 'electrics']);
</script>

{#if mode === 'ten'}
  <!-- All ten, which is the whole set: `domain-model.md` §3.5 allows no eleventh. -->
  <Inline gap="100">
    {#each labelTokens as token (token)}
      <LabelChip name={token} colorToken={token} />
    {/each}
  </Inline>
{:else if mode === 'removable'}
  <Inline gap="100">
    {#each collectionLabels.filter((label) => onEntry.includes(label.id)) as label (label.id)}
      <LabelChip
        name={label.name}
        colorToken={label.colorToken}
        description={label.description}
        removeLabel={`Take “${label.name}” off this entry`}
        onRemove={() => (onEntry = onEntry.filter((id) => id !== label.id))}
      />
    {/each}
  </Inline>
{:else if mode === 'empty'}
  <LabelPicker
    label="Labels in this collection"
    labels={[]}
    filterLabel="Filter the labels"
    emptyLabel="No labels in this collection yet."
    noMatchLabel="No label matches that."
  />
{:else}
  <Stack gap="200">
    <LabelPicker
      label="Labels in this collection"
      labels={mode === 'long' ? long : collectionLabels}
      {selected}
      filterLabel="Filter the labels"
      emptyLabel="No labels in this collection yet."
      noMatchLabel="No label matches that."
      onToggle={(id) =>
        (selected = selected.includes(id) ? selected.filter((each) => each !== id) : [...selected, id])}
    />
  </Stack>
{/if}
