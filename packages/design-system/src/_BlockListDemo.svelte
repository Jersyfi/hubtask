<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import BlockList, { type Block } from './BlockList.svelte';
  import Stack from './Stack.svelte';
  import { blockMenu, statuses, steps } from './_pageFixtures.ts';
  import { rankTarget } from './reorder.ts';

  let statusBlocks = $state<Block[]>([...statuses]);
  let stepBlocks = $state<Block[]>([...steps]);
  let opened = $state('');

  function moved(list: Block[], id: string, to: number): Block[] {
    const from = list.findIndex((block) => block.id === id);
    if (from < 0 || from === to) return list;
    const next = [...list];
    const [block] = next.splice(from, 1);
    next.splice(to, 0, block!);
    return next;
  }

  function menuMove(list: Block[], block: Block, id: string): Block[] {
    const index = list.indexOf(block);
    const to = id === 'up' ? rankTarget('up', index, list.length) : id === 'down' ? rankTarget('down', index, list.length) : null;
    if (to === null) {
      if (id === 'edit') opened = block.name;
      return list;
    }
    return moved(list, block.id, to);
  }

  // No props: the story's arguments, if any, are ignored.
  const {}: Record<string, unknown> = $props();
</script>

<Stack gap="300">
  <BlockList
    title="Statuses"
    count={String(statusBlocks.length)}
    blocks={statusBlocks}
    onopen={(id) => (opened = statusBlocks.find((block) => block.id === id)?.name ?? '')}
    menu={(block) => blockMenu(block.name)}
    onmenu={(block, id) => (statusBlocks = menuMove(statusBlocks, block, id))}
    onmove={(id, to) => (statusBlocks = moved(statusBlocks, id, to))}
    gripLabel={(block) => `Move ${block.name}`}
    add={{ label: 'Add status', onclick: () => {} }}
  />
  <BlockList
    title="Steps"
    count={String(stepBlocks.length)}
    blocks={stepBlocks}
    onopen={(id) => (opened = stepBlocks.find((block) => block.id === id)?.name ?? '')}
    menu={(block) => blockMenu(block.name)}
    onmenu={(block, id) => (stepBlocks = menuMove(stepBlocks, block, id))}
    onmove={(id, to) => (stepBlocks = moved(stepBlocks, id, to))}
    gripLabel={(block) => `Move ${block.name}`}
    add={{ label: 'Add step', onclick: () => {} }}
  />
  {#if opened}<p>Opens the dialog of “{opened}”.</p>{/if}
</Stack>
