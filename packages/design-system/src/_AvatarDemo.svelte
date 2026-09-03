<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Avatar from './Avatar.svelte';
  import AvatarGroup from './AvatarGroup.svelte';
  import Inline from './Inline.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'people' }: { mode?: 'people' | 'group' } = $props();

  // Names in four scripts, because "the initials" is where an avatar quietly becomes a Latin-only
  // component. Nothing here is a real person.
  const people = [
    { name: 'Ada Lovelace' },
    { name: '张 伟' },
    { name: 'أمينة بن يوسف' },
    { name: 'Björn Sørensen' },
    { name: 'Mira' },
    { name: 'Tomás de la Vega' },
  ];
</script>

{#if mode === 'group'}
  <Stack gap="250">
    <AvatarGroup people={people.slice(0, 3)} />
    <AvatarGroup {people} max={4} overflowLabel="2 more people" />
    <AvatarGroup {people} max={2} size="sm" overflowLabel="4 more people" />
  </Stack>
{:else}
  <Stack gap="250">
    <Inline gap="150" align="center">
      {#each people as person (person.name)}
        <Avatar name={person.name} />
      {/each}
    </Inline>
    <Inline gap="150" align="center">
      {#each people.slice(0, 4) as person (person.name)}
        <Avatar name={person.name} size="sm" />
      {/each}
    </Inline>
  </Stack>
{/if}
