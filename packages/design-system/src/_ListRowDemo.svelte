<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import IconButton from './IconButton.svelte';
  import ListRow from './ListRow.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'link' }: { mode?: 'link' | 'select' | 'plain' | 'long' } = $props();

  let chosen = $state('b');

  const entries = [
    { id: 'a', title: 'Move the socket by the window', badge: 'Electrics' },
    { id: 'b', title: 'Order the tiles', badge: 'Materials' },
    { id: 'c', title: 'Book the electrician', badge: 'Electrics' },
  ];

  const long = [
    { id: 'a', title: 'Steckdose am Küchenfenster versetzen und neu verkabeln lassen', badge: 'Elektroarbeiten' },
    { id: 'b', title: 'Wandfliesen für Küche und Bad bestellen', badge: 'Materialbeschaffung' },
  ];
</script>

<Stack gap="050">
  {#each mode === 'long' ? long : entries as entry (entry.id)}
    <ListRow
      href={mode === 'link' || mode === 'long' ? `/items/${entry.id}` : undefined}
      onactivate={mode === 'select' ? () => (chosen = entry.id) : undefined}
      isSelected={mode === 'select' ? chosen === entry.id : mode !== 'plain' && entry.id === 'b'}
    >
      {#snippet leading()}
        <!-- A control in the leading slot, to show that it is outside the row's activation: this
             handle can be reached and used without opening the entry. A checkbox is what a real
             list puts here, and it wants an accessible name that is not drawn — which `Checkbox`
             has no way to give it today. Named in the pull request rather than worked around. -->
        <IconButton icon="grip-vertical" label={`Reorder ${entry.title}`} size="sm" />
      {/snippet}
      {entry.title}
      {#snippet trailing()}
        <Badge>{entry.badge}</Badge>
        <IconButton icon="ellipsis" label={`Actions for ${entry.title}`} size="sm" />
      {/snippet}
    </ListRow>
  {/each}
</Stack>
