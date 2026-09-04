<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Badge from './Badge.svelte';
  import IconButton from './IconButton.svelte';
  import Stack from './Stack.svelte';
  import TaskRow from './TaskRow.svelte';

  const { mode = 'levels' }: { mode?: 'levels' | 'collapsed' | 'unknown' | 'long' } = $props();

  // The three levels of domain-model.md §3.4, at the depths they sit at.
  const levels = [
    { id: 'a', type: 'TASK', title: 'The kitchen', depth: 0, expansion: 'expanded' as const },
    { id: 'b', type: 'WORK_PACKAGE', title: 'Electrics', depth: 1, expansion: 'expanded' as const },
    { id: 'c', type: 'ACTIVITY', title: 'Move the socket by the window', depth: 2, expansion: 'leaf' as const },
    { id: 'd', type: 'ACTIVITY', title: 'Book the electrician', depth: 2, expansion: 'leaf' as const, done: true },
  ];

  const collapsed = [
    { id: 'a', type: 'TASK', title: 'The kitchen', depth: 0, expansion: 'collapsed' as const },
    { id: 'e', type: 'TASK', title: 'The bathroom', depth: 0, expansion: 'collapsed' as const },
  ];

  // A type this client has never heard of, which an installation may report (domain-model.md §2's
  // extension example). It gets a row rather than being refused.
  const unknown = [
    { id: 'a', type: 'TASK', title: 'The kitchen', depth: 0, expansion: 'leaf' as const },
    { id: 'm', type: 'MILESTONE', title: 'Finished by the end of March', depth: 0, expansion: 'leaf' as const },
  ];

  const long = [
    { id: 'a', type: 'TASK', title: 'Küchenumbau und die vollständige Elektroinstallation', depth: 0, expansion: 'expanded' as const },
    { id: 'b', type: 'WORK_PACKAGE', title: 'Elektroarbeiten im Bereich der Fensterfront', depth: 1, expansion: 'leaf' as const },
  ];

  const rows = $derived(
    mode === 'collapsed' ? collapsed : mode === 'unknown' ? unknown : mode === 'long' ? long : levels,
  );

  let done = $state<string[]>(['d']);
</script>

<Stack gap="050">
  {#each rows as row (row.id)}
    <TaskRow
      type={row.type}
      title={row.title}
      depth={row.depth}
      expansion={row.expansion}
      isCompleted={done.includes(row.id)}
      href={`/items/${row.id}`}
      completeLabel={`Mark “${row.title}” as done`}
      expandLabel={row.expansion === 'expanded' ? 'Hide what is inside' : 'Show what is inside'}
      onToggleComplete={() =>
        (done = done.includes(row.id) ? done.filter((id) => id !== row.id) : [...done, row.id])}
    >
      {#snippet trailing()}
        <Badge>{row.type.replace('_', ' ').toLowerCase()}</Badge>
        <IconButton icon="ellipsis" label={`Actions for ${row.title}`} size="sm" />
      {/snippet}
    </TaskRow>
  {/each}
</Stack>
