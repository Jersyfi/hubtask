<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import BucketColumn from './BucketColumn.svelte';
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';
  import LabelChip from './LabelChip.svelte';
  import WorkItemCard from './WorkItemCard.svelte';

  const { mode = 'board' }: { mode?: 'board' | 'overLimit' | 'covers' | 'long' } = $props();

  interface Card {
    id: string;
    title: string;
    // §5: a boolean asks a question, in a fixture as much as in a prop — the check reads the whole
    // package, and a demo that named it `done` would be a demo teaching the wrong habit.
    isDone?: boolean;
    cover?: { kind: 'COLOR' | 'IMAGE'; token?: string };
    labels?: { name: string; token: string }[];
  }

  const columns: Column[] = [
    {
      id: 'todo',
      name: 'To do',
      cards: [
        { id: 'a', title: 'Order the tiles', labels: [{ name: 'Materials', token: 'teal' }] },
        { id: 'b', title: 'Book the electrician', labels: [{ name: 'Urgent', token: 'red' }] },
      ],
    },
    {
      id: 'doing',
      name: 'Doing',
      wip: 2,
      cards: [{ id: 'c', title: 'Move the socket by the window' }],
    },
    {
      id: 'done',
      name: 'Done',
      isDoneBucket: true,
      cards: [{ id: 'd', title: 'Measure the wall', isDone: true }],
    },
  ];

  const overLimit: Column[] = columns.map((column) =>
    column.id === 'doing'
      ? {
          ...column,
          wip: 1,
          cards: [
            ...column.cards,
            { id: 'e', title: 'Seal the joints' },
            { id: 'f', title: 'Fit the splashback' },
          ],
        }
      : column,
  );

  /** A column, typed once so every fixture below is the same shape. */
  interface Column {
    id: string;
    name: string;
    wip?: number | null;
    isDoneBucket?: boolean;
    cards: Card[];
  }

  const covers: Column[] = [
    {
      id: 'todo',
      name: 'To do',
      cards: [
        { id: 'a', title: 'Order the tiles', cover: { kind: 'COLOR', token: 'amber' } },
        { id: 'b', title: 'Book the electrician', cover: { kind: 'COLOR', token: 'violet' } },
        { id: 'g', title: 'No cover at all' },
      ],
    },
  ];

  const long: Column[] = [
    {
      id: 'todo',
      name: 'Zu erledigen',
      wip: 3,
      cards: [
        {
          id: 'a',
          title: 'Wandfliesen für Küche und Bad bestellen und liefern lassen',
          labels: [{ name: 'Materialbeschaffung', token: 'teal' }],
        },
      ],
    },
  ];

  const shown = $derived(
    mode === 'overLimit' ? overLimit : mode === 'covers' ? covers : mode === 'long' ? long : columns,
  );
</script>

<div class="board">
  {#each shown as column (column.id)}
    <BucketColumn
      name={column.name}
      count={column.cards.length}
      wipLimit={column.wip ?? null}
      overLimitLabel="More entries than this column is meant to hold."
      isDoneBucket={column.isDoneBucket ?? false}
      doneBucketLabel="Entries moved here are marked as done."
    >
      {#snippet actions()}
        <IconButton icon="ellipsis" label={`Actions for ${column.name}`} size="sm" />
      {/snippet}

      {#each column.cards as card (card.id)}
        <WorkItemCard
          title={card.title}
          href={`/items/${card.id}`}
          isCompleted={card.isDone ?? false}
          coverKind={card.cover?.kind ?? null}
          coverColorToken={card.cover?.token ?? null}
        >
          {#snippet footer()}
            {#if card.labels}
              <Inline gap="050">
                {#each card.labels as label (label.name)}
                  <LabelChip name={label.name} colorToken={label.token} />
                {/each}
              </Inline>
            {/if}
          {/snippet}
        </WorkItemCard>
      {/each}
    </BucketColumn>
  {/each}
</div>

<style>
  /* The board scrolls sideways, not the page: a wide board that widened the document would take
     every other region with it. */
  .board {
    display: flex;
    gap: var(--sp-200);
    align-items: start;
    overflow-x: auto;
    padding-block-end: var(--sp-100);
  }
</style>
