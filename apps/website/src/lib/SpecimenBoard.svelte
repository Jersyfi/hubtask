<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The same collection as a board, drawn with the real `BucketColumn` and `WorkItemCard`.
  // A picture, on the same terms as SpecimenList: inert, out of the accessibility tree, and
  // explained by its caption.

  import { BucketColumn, Icon, LabelChip, WorkItemCard } from '@hubtask/design-system/components';

  type Card = {
    title: string;
    cover: string | null;
    done?: boolean;
    label: { name: string; token: string } | null;
  };
  type Column = { name: string; count: number; wipLimit?: number; isDone?: boolean; cards: Card[] };

  const columns: Column[] = [
    {
      name: 'To do',
      count: 4,
      cards: [
        { title: 'Renew the domain', cover: 'violet', label: { name: 'admin', token: 'violet' } },
        { title: 'Draft the release note', cover: null, label: { name: 'release', token: 'blue' } },
      ],
    },
    {
      name: 'In progress',
      count: 2,
      wipLimit: 3,
      cards: [{ title: 'Move the nameservers', cover: 'amber', label: { name: 'infra', token: 'amber' } }],
    },
    {
      name: 'Done',
      count: 9,
      isDone: true,
      cards: [{ title: 'Export the DNS zone', cover: null, done: true, label: null }],
    },
  ];
</script>

<figure class="specimen">
  <div class="specimen-frame" inert aria-hidden="true">
    <div class="specimen-bar">
      <span class="specimen-where">
        <Icon name="collection" size="sm" /> Errands
        <span class="specimen-sep">/</span>
        <Icon name="bucket" size="sm" /> Board
      </span>
      <span class="specimen-tools">
        <Icon name="search" size="sm" />
        <Icon name="funnel" size="sm" />
      </span>
    </div>

    <div class="specimen-board">
      {#each columns as column (column.name)}
        <BucketColumn
          name={column.name}
          count={column.count}
          wipLimit={column.wipLimit ?? null}
          isDoneBucket={column.isDone ?? false}
          doneBucketLabel="Dropping here completes an entry"
          overLimitLabel="Over the limit this column announces"
        >
          {#each column.cards as card (card.title)}
            <WorkItemCard
              title={card.title}
              isCompleted={card.done ?? false}
              coverKind={card.cover ? 'COLOR' : null}
              coverColorToken={card.cover}
            >
              {#snippet footer()}
                {#if card.label}
                  <LabelChip name={card.label.name} colorToken={card.label.token} />
                {/if}
              {/snippet}
            </WorkItemCard>
          {/each}
        </BucketColumn>
      {/each}
    </div>
  </div>

  <figcaption>
    The same collection as a board. The work-in-progress limit and the done column are
    <em>announced</em>, never enforced — the server accepts a card that crosses a limit somebody set
    as a reminder to themselves, and it is the board that decides what to do about it.
  </figcaption>
</figure>
