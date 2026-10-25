<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Avatar from './Avatar.svelte';
  import BucketColumn from './BucketColumn.svelte';
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';
  import LabelChip from './LabelChip.svelte';
  import ViewSwitcher, { type View } from './ViewSwitcher.svelte';
  import WorkItemCard from './WorkItemCard.svelte';

  const { mode = 'board' }: { mode?: 'board' | 'overLimit' | 'covers' | 'long' | 'phone' | 'beside' | 'zones' } = $props();

  // The card open beside the board: a plain press on a title opens it here rather than following
  // the link; a press with a modifier still opens the link in a new tab.
  let current = $state<string | undefined>('c');

  interface Card {
    id: string;
    title: string;
    // §5: a boolean asks a question, in a fixture as much as in a prop — the check reads the whole
    // package, and a demo that named it `done` would be a demo teaching the wrong habit.
    isDone?: boolean;
    cover?: { kind: 'COLOR' | 'IMAGE'; token?: string };
    labels?: { name: string; token: string }[];
    /** What the footer counts, and who is on it. */
    due?: string;
    counts?: string;
    people?: string[];
  }

  // The card's zones, sparse and full side by side: a title alone, a date and a person, everything.
  const zones: Column[] = [
    {
      id: 'zones',
      name: 'Doing',
      cards: [
        { id: 'z1', title: 'Check the logo' },
        { id: 'z2', title: 'Call Mr Müller', due: 'today 9:00', people: ['BO'] },
        {
          id: 'z3',
          title: 'Write the home page and agree the tone for every sub-page with the client before the translations are ordered',
          cover: { kind: 'COLOR', token: 'violet' },
          labels: [{ name: 'Design', token: 'blue' }, { name: 'Urgent', token: 'red' }, { name: 'Text', token: 'green' }, { name: 'Web', token: 'teal' }],
          due: 'tomorrow 10:00',
          counts: '☑ 1/3 · 💬 4 · 📎 2',
          people: ['JW', 'AB'],
        },
      ],
    },
  ];

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
    mode === 'overLimit' ? overLimit : mode === 'covers' ? covers : mode === 'long' ? long : mode === 'zones' ? zones : columns,
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
          onOpen={mode === 'beside' ? () => (current = card.id) : undefined}
          isCurrent={mode === 'beside' && current === card.id}
          isCompleted={card.isDone ?? false}
          coverKind={card.cover?.kind ?? null}
          coverColorToken={card.cover?.token ?? null}
        >
          {#snippet labels()}
            {#each (card.labels ?? []).slice(0, 2) as label (label.name)}
              <LabelChip name={label.name} colorToken={label.token} />
            {/each}
            {#if (card.labels ?? []).length > 2}<span class="more">+{(card.labels ?? []).length - 2}</span>{/if}
          {/snippet}
          {#snippet footer()}
            {#if card.due}<span>{card.due}</span>{/if}
            {#if card.counts}<span>{card.counts}</span>{/if}
          {/snippet}
          {#snippet people()}
            {#each card.people ?? [] as initials (initials)}<Avatar name={initials} size="sm" />{/each}
          {/snippet}
        </WorkItemCard>
      {/each}
    </BucketColumn>
    </div>
  {/each}
</div>

<style>
  .more { color: var(--text-secondary); font-size: var(--fs-050); }

  /* The board scrolls sideways, not the page: a wide board that widened the document would take
     every other region with it. */
  .board {
    /* Positioned, as the product's board is: the containing block of what it scrolls. */
    position: relative;
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
