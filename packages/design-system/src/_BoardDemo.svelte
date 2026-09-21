<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import BucketColumn from './BucketColumn.svelte';
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';
  import LabelChip from './LabelChip.svelte';
  import ViewSwitcher, { type View } from './ViewSwitcher.svelte';
  import WorkItemCard from './WorkItemCard.svelte';

  const { mode = 'board' }: { mode?: 'board' | 'overLimit' | 'covers' | 'long' | 'phone' } = $props();

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

  const all = $derived(
    mode === 'overLimit' ? overLimit : mode === 'covers' ? covers : mode === 'long' ? long : columns,
  );

  // On a phone the board is one column at a time (ADR-0061 decision 4): the strip above says which
  // and how many, a tap switches, and each column keeps its own head and its own actions.
  let shownId = $state('todo');
  const strip = $derived<View[]>(all.map((column) => ({ id: column.id, label: `${column.name} · ${column.cards.length}` })));
  const shown = $derived(mode === 'phone' ? all.filter((column) => column.id === shownId) : all);
</script>

{#if mode === 'phone'}
  <div class="strip">
    <ViewSwitcher label="Which column is shown" views={strip} selected={shownId} onselect={(id) => (shownId = id)} />
  </div>
{/if}

<div class="board" data-one-column={mode === 'phone' ? '' : undefined}>
  {#each shown as column (column.id)}
    <div class="column-slot">
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
    </div>
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

  /* One column on a phone: it takes the width the strip leaves it. */
  .board[data-one-column] { flex-direction: column; align-items: stretch; overflow-x: visible; }

  .column-slot { display: flex; flex: none; }

  .board[data-one-column] .column-slot > :global(*) { box-sizing: border-box; inline-size: 100%; }

  .strip { overflow-x: auto; margin-block-end: var(--sp-150); padding-block-end: var(--sp-050); }
</style>
