<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The collection as a board: one grouped query, one column per bucket.
  //
  // It is not a list fetched and then sorted in the browser. `group_by` is what turns the read into
  // columns — "one group per distinct value, each with its own rows and its own cursor" — and that
  // is also why a column with two hundred cards pages **on its own**: a grouped result has no
  // single walk, so continuing a column is a different question rather than a later page of the
  // same one.
  //
  // **`BUCKET` is a `TASK` capability only.** Buckets apply to the entries directly in the
  // collection, so a board shows tasks; what a work package or an activity does on a board is
  // nothing, and this says so rather than filtering them out silently.

  import { untrack } from 'svelte';

  import {
    BucketColumn,
    EmptyState,
    ErrorState,
    IconButton,
    Inline,
    LabelChip,
    Menu,
    Skeleton,
    WorkItemCard,
  } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { buckets } from '../data/buckets.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    collectionId: string;
    isReadOnly?: boolean;
  }

  const { collectionId, isReadOnly = false }: Props = $props();

  $effect(() => {
    const wanted = collectionId;
    return untrack(() => {
      const stops = [
        buckets.open(wanted),
        items.openBoard(wanted),
        labels.open(wanted),
      ];
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  const columns = $derived(buckets.of(collectionId));
  const groups = $derived(items.boardGroups(collectionId));
  const boardState = $derived(items.stateOf(`board:${collectionId}`));
  const failure = $derived(
    boardState?.status === 'failed' ? renderProblem(boardState.error, messages) : undefined,
  );

  /** The cards of one column, from the group whose key is that bucket. */
  function cardsOf(bucketId: string | null): readonly WorkItem[] {
    return groups.find((group) => group.key === bucketId)?.data ?? [];
  }

  function countOf(bucketId: string | null): number | null {
    return groups.find((group) => group.key === bucketId)?.count ?? null;
  }

  const available = $derived(labels.of(collectionId));

  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /**
   * Moves a card, and completes it where the column says so.
   *
   * **The completion is this client's, not the server's**, and that is the domain's own division
   * rather than a choice made here: `Bucket.IsDoneBucket` is "stored and reported; what reacts to
   * it is the client that renders the board … the server completes nothing on its own account".
   * So the board is the thing that reacts, and a card dropped in the done column comes back ticked
   * because this asked for it.
   *
   * Two writes, and they are not atomic. If the completion fails the card has still moved, which
   * is the honest outcome — the move is what the reader asked for and it happened — and the failure
   * says so rather than the move being rolled back to hide it.
   *
   * Moving **out** of a done column does not reopen. Nothing says it should, and inventing the
   * reverse of a rule is how a client acquires behaviour nobody can find in the model.
   */
  async function moveTo(item: WorkItem, bucketId: string | null) {
    writeFailure = undefined;
    try {
      const moved = await items.setBucket(item.id, bucketId, item.version);

      const target = columns.find((column) => column.id === bucketId);
      if (target?.is_done_bucket && !moved.completion?.is_completed) {
        await items.setCompleted(moved.id, true, crypto.randomUUID());
      }
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }
</script>

{#if boardState === undefined || boardState.status === 'loading' || boardState.status === 'idle'}
  <div aria-busy="true"><Skeleton lines={3} shape="block" /></div>
{:else if failure}
  <ErrorState
    title={failure.message}
    reference={failure.reference}
    referenceLabel={t('app.reference')}
    retryLabel={t('app.retry')}
    onRetry={() => items.openBoard(collectionId)()}
  />
{:else if columns.length === 0}
  <EmptyState kind="unused" title={t('app.board.no_buckets')} icon="bucket" />
{:else}
  {#if writeFailure}<p class="failure">{writeFailure.message}</p>{/if}

  <div class="board">
    {#each [...columns, null] as bucket (bucket?.id ?? 'none')}
      {@const bucketId = bucket?.id ?? null}
      {@const cards = cardsOf(bucketId)}
      <!-- The entries with no bucket are their own column, and the API puts that group last. It is
           shown rather than hidden: a card nobody has put in a column is a card somebody has to
           find. -->
      {#if bucket !== null || cards.length > 0}
        <BucketColumn
          name={bucket?.name ?? t('app.board.unbucketed')}
          count={countOf(bucketId) ?? cards.length}
          wipLimit={bucket?.wip_limit ?? null}
          overLimitLabel={t('app.board.over_limit')}
          isDoneBucket={bucket?.is_done_bucket ?? false}
          doneBucketLabel={t('app.board.done_bucket')}
        >
          {#snippet actions()}
            {#if bucket}
              <IconButton
                icon="ellipsis"
                label={t('app.board.column_actions', { name: bucket.name })}
                size="sm"
              />
            {/if}
          {/snippet}

          {#each cards as card (card.id)}
            <WorkItemCard
              title={card.title}
              href={`/items/${card.id}`}
              isCompleted={card.completion?.is_completed ?? false}
              coverKind={card.cover?.kind ?? null}
              coverColorToken={card.cover?.color_token ?? null}
            >
              {#snippet footer()}
                <Inline gap="050">
                  {#each (card.label_ids ?? []) as labelId (labelId)}
                    {@const label = available.find((each) => each.id === labelId)}
                    {#if label}
                      <LabelChip name={label.name} colorToken={label.color_token} />
                    {/if}
                  {/each}
                  {#if !isReadOnly}
                    <!-- The keyboard path to moving a card, and for now the only one — F2-12 builds
                         the drag against it, because WCAG 2.2 SC 2.5.7 wants a single-pointer
                         alternative and a rank change is a command before it is a gesture. -->
                    <Menu
                      label={t('app.board.column_actions', { name: card.title })}
                      items={[...columns, null].map((target) => ({
                        id: target?.id ?? 'none',
                        label: t('app.board.move_to', {
                          title: card.title,
                          name: target?.name ?? t('app.board.unbucketed'),
                        }),
                      }))}
                      onselect={(id) => moveTo(card, id === 'none' ? null : id)}
                    >
                      {#snippet trigger(props)}
                        <IconButton icon="ellipsis" label={t('app.board.column_actions', { name: card.title })} size="sm" {...props} />
                      {/snippet}
                    </Menu>
                  {/if}
                </Inline>
              {/snippet}
            </WorkItemCard>
          {/each}
        </BucketColumn>
      {/if}
    {/each}
  </div>
{/if}

<style>
  /* The board scrolls sideways, not the page. */
  .board {
    display: flex;
    gap: var(--sp-200);
    align-items: start;
    overflow-x: auto;
    padding-block-end: var(--sp-100);
  }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
