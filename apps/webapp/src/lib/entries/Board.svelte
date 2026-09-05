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
    Button,
    Dialog,
    EmptyState,
    ErrorState,
    Icon,
    IconButton,
    Inline,
    LabelChip,
    Menu,
    Skeleton,
    WorkItemCard,
    rankIntent,
    rankTarget,
    type RankCommand,
  } from '@hubtask/design-system/components';
  import type { Bucket, WorkItem } from '@hubtask/sync-engine';

  import type { ItemsQuery } from '../data/items.svelte.ts';

  import BucketDialog from './BucketDialog.svelte';
  import { createDrag } from './dragging.svelte.ts';

  import { announcer } from '../announce.svelte.ts';
  import { buckets } from '../data/buckets.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { anchorFor } from '../data/rank.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    collectionId: string;
    isReadOnly?: boolean;
    /**
     * What the reader has asked of the board: a filter, an order, and the field it groups by.
     *
     * The grouping arrives from the caller because the **manifest** decides which fields may be
     * grouped on. A board grouped by the column an entry is in is what a board is, and that is the
     * caller's default — but the field travels rather than being written here, and a board of an
     * installation that groups by something else renders that instead.
     */
    query?: ItemsQuery;
  }

  const { collectionId, isReadOnly = false, query }: Props = $props();

  $effect(() => {
    const wanted = collectionId;
    const asked = query;
    return untrack(() => {
      const stops = [
        buckets.open(wanted),
        items.openBoard(wanted, asked),
        labels.open(wanted),
      ];
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  const groups = $derived(items.boardGroups(collectionId));

  /**
   * Whether this board is the bucket board.
   *
   * It is, unless the reader has grouped by something else — and then the columns are the values
   * the query grouped on rather than the collection's buckets, because a `wip_limit` and a done
   * column are properties of a bucket and mean nothing about a title or a type.
   */
  const isBucketBoard = $derived((query?.group?.field ?? 'bucket_id') === 'bucket_id');

  const columns = $derived(
    isBucketBoard
      ? buckets.of(collectionId)
      : // A column per value the result came back grouped by, named by the value itself: the
        // grouping field is the installation's and this client has no vocabulary for its values.
        groups
          .filter((group) => group.key !== null)
          .map((group) => ({
            id: group.key as string,
            name: group.key as string,
            wip_limit: null,
            is_done_bucket: false,
            version: 0,
          })),
  );
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

  // Managing the columns themselves. `buckets` has had `create`, `update`, `remove` and `reorder`
  // since F2-11 and no caller, so a collection's board could only be set up outside the
  // application - including `wip_limit` and `is_done_bucket`, both of which this board reads and
  // acts on.
  let editing = $state<Bucket | undefined>(undefined);
  let isEditingColumn = $state(false);
  let deleting = $state<Bucket | undefined>(undefined);
  let isDeletingColumn = $state(false);

  function editColumn(bucket: Bucket | undefined) {
    editing = bucket;
    isEditingColumn = true;
  }

  const COLUMN_ACTIONS = [
    { id: 'edit', label: 'app.board.edit_column' },
    { id: 'delete', label: 'app.board.delete_column', isDestructive: true },
  ];

  // By identifier rather than by the column object: a board grouped by anything but a bucket has
  // synthesised columns that are not buckets, and looking the real one up here is what makes it
  // impossible to act on one of those rather than merely unlikely.
  function choseForColumn(bucketId: string, id: string) {
    const column = buckets.of(collectionId).find((each) => each.id === bucketId);
    if (!column) return;
    if (id === 'edit') {
      editColumn(column);
      return;
    }
    deleting = column;
    isDeletingColumn = true;
  }

  async function deleteColumn() {
    if (!deleting) return;
    writeFailure = undefined;
    try {
      await buckets.remove(collectionId, deleting.id, deleting.version);
      isDeletingColumn = false;
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

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
  async function moveTo(item: WorkItem, bucketId: string | null): Promise<WorkItem | undefined> {
    writeFailure = undefined;
    try {
      let moved = await items.setBucket(item.id, bucketId, item.version);

      const target = columns.find((column) => column.id === bucketId);
      if (target?.is_done_bucket && !moved.completion?.is_completed) {
        moved = await items.setCompleted(moved.id, true, crypto.randomUUID());
      }
      announcer.say(
        t('app.board.moved', {
          title: item.title,
          name: target?.name ?? t('app.board.unbucketed'),
        }),
      );
      // The card as it now stands, version included. A drag ranks it straight afterwards, and a
      // version guessed rather than read is a precondition that fails for no reason the reader
      // can act on — the completion above may have bumped it a second time.
      return moved;
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
      return undefined;
    }
  }

  /**
   * Ranks a card within its column.
   *
   * **A column is not a level.** The level `:reorder` moves an entry within is the collection's
   * top level, and a column is that level seen through one bucket — so the neighbour named here is
   * the next card *in the column*, which is a position in the level and lands the card where the
   * reader watched it go. Naming the next entry of the level instead would put the card in front
   * of one it cannot see.
   *
   * The same call the list makes, from the same arithmetic, with an `Idempotency-Key` per intent.
   */
  async function rank(card: WorkItem, cards: readonly WorkItem[], command: RankCommand) {
    const index = cards.findIndex((each) => each.id === card.id);
    const target = rankTarget(command, index, cards.length);
    if (target === null) return;
    await rankTo(card, cards, target);
  }

  /** The call itself, once something — a command, a key or a drag — has named the position. */
  async function rankTo(card: WorkItem, cards: readonly WorkItem[], target: number) {
    writeFailure = undefined;
    try {
      await items.reorder(
        card.id,
        anchorFor(cards, card.id, target),
        card.version,
        crypto.randomUUID(),
      );
      announcer.say(
        t('app.rank.announced', {
          title: card.title,
          position: target + 1,
          count: cards.length,
        }),
      );
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /** Why a rank command is unavailable on this card, or nothing. */
  function rankReason(
    card: WorkItem,
    cards: readonly WorkItem[],
    command: RankCommand,
  ): string | undefined {
    if (isReadOnly) return t('app.entries.read_only');
    const index = cards.findIndex((each) => each.id === card.id);
    if (rankTarget(command, index, cards.length) !== null) return undefined;
    return command === 'up' || command === 'top'
      ? t('app.rank.already_first')
      : t('app.rank.already_last');
  }

  const RANK_COMMANDS: readonly { readonly id: RankCommand; readonly label: string }[] = [
    { id: 'up', label: 'app.rank.up' },
    { id: 'down', label: 'app.rank.down' },
    { id: 'top', label: 'app.rank.top' },
    { id: 'bottom', label: 'app.rank.bottom' },
  ];

  /**
   * The whole of what a card can be told to do: where in the column, and which column.
   *
   * Both in one menu, because the reader is answering one question. The separator is where the
   * column stops being the column — and where the call behind it stops being `:reorder`.
   */
  function menuItems(card: WorkItem, cards: readonly WorkItem[]) {
    return [
      ...RANK_COMMANDS.map((command) => ({
        id: command.id,
        label: t(command.label),
        disabledReason: rankReason(card, cards, command.id),
      })),
      // The columns, but only where a column is a **bucket**. Grouping by something else makes
      // the columns values rather than places, and "move this entry into `WORK_PACKAGE`" is not an
      // offer — a type is a property of the entry, not a column it can be put in.
      ...(isBucketBoard
        ? [...columns, null].map((target, index) => ({
            id: `bucket:${target?.id ?? 'none'}`,
            label: t('app.board.move_to', {
              title: card.title,
              name: target?.name ?? t('app.board.unbucketed'),
            }),
            hasSeparatorBefore: index === 0,
          }))
        : []),
    ];
  }

  function chose(card: WorkItem, cards: readonly WorkItem[], id: string) {
    if (!id.startsWith('bucket:')) {
      void rank(card, cards, id as RankCommand);
      return;
    }
    const bucketId = id.slice('bucket:'.length);
    void moveTo(card, bucketId === 'none' ? null : bucketId);
  }

  /**
   * The pointer path, and the one surface where a drag may leave the list it started in.
   *
   * A card carried into another column is a **column change and a rank change**, and the board
   * already had a call for the first: the same `PATCH bucket_id` the menu makes, with everything
   * that hangs off it — the done column still completes the card, because that reaction is this
   * client's rather than the server's. Then the rank, so the card lands where the reader dropped
   * it rather than at the end of the column they dropped it into.
   *
   * Two writes again, and not atomic, for the reason `moveTo` records: if the second fails the
   * card has still changed column, which is the larger half of what was asked for and is the
   * honest outcome rather than a rollback that hides it.
   */
  const drag = createDrag({
    start: (grip) => {
      const held = grip.closest('[data-card]');
      const cardId = held?.getAttribute('data-card');
      const key = held?.getAttribute('data-column') ?? 'none';
      const cards = cardsOf(key === 'none' ? null : key);
      const index = cards.findIndex((each) => each.id === cardId);
      if (!cardId || index < 0 || isReadOnly) return null;
      return { id: cardId, index, level: { key, elements: elementsOf(key) } };
    },
    // Which column the pointer is over. `elementFromPoint` rather than a rectangle kept from the
    // start of the gesture: a board scrolls sideways while a card is being carried across it.
    levelAt: ({ x, y }) => {
      const zone = document.elementFromPoint(x, y)?.closest('[data-column-zone]');
      const key = zone?.getAttribute('data-column-zone');
      return key === null || key === undefined ? null : { key, elements: elementsOf(key) };
    },
    ondrop: ({ id, to, levelKey }) => {
      const from = board?.querySelector(`[data-card="${id}"]`)?.getAttribute('data-column') ?? 'none';
      const card = cardsOf(from === 'none' ? null : from).find((each) => each.id === id);
      if (card) void drop(card, from, levelKey, to);
    },
  });

  /** The card elements of one column, in drawn order, for the measuring. */
  function elementsOf(key: string): HTMLElement[] {
    return [...(board?.querySelectorAll<HTMLElement>(`[data-column="${key}"]`) ?? [])];
  }

  async function drop(card: WorkItem, from: string, to: string, position: number) {
    // Across columns is a column change only where a column is a bucket. On any other grouping the
    // drag ranks within the column it was dropped in and changes nothing else, because the value
    // it was grouped by is not a place an entry can be put.
    if (!isBucketBoard) {
      await rankTo(card, cardsOf(to === 'none' ? null : to), position);
      return;
    }
    const bucketId = to === 'none' ? null : to;
    // The destination's cards **as the reader saw them**, taken before anything is written. The
    // board re-reads after a write, and reading it again afterwards would rank the card against a
    // list that arrived later than the gesture it is answering.
    const cards = cardsOf(bucketId);

    if (from === to) {
      await rankTo(card, cards, position);
      return;
    }

    const moved = await moveTo(card, bucketId);
    if (!moved) return;
    await items.reorder(
      card.id,
      anchorFor(cards, card.id, position),
      moved.version,
      crypto.randomUUID(),
    );
  }

  $effect(() => {
    const node = board;
    return node ? drag.attach(node) : undefined;
  });

  /**
   * The keyboard shortcuts, listened for on the board rather than bound in the markup — a `<div>`
   * with an `onkeydown` is a static element with an interaction, and Svelte is right to warn.
   *
   * `data-card` is how a press finds its way back from the focused control to the card, and
   * `data-column` says which column's cards it is ranked among.
   */
  let board = $state<HTMLElement | null>(null);

  $effect(() => {
    const node = board;
    if (!node || isReadOnly) return;

    const onKeydown = (event: KeyboardEvent) => {
      const held = (event.target as Element | null)?.closest?.('[data-card]');
      const cardId = held?.getAttribute('data-card');
      if (!cardId) return;
      const column = held?.getAttribute('data-column') ?? null;
      const cards = cardsOf(column === 'none' ? null : column);
      const index = cards.findIndex((each) => each.id === cardId);
      const card = cards[index];
      if (!card) return;

      if (rankIntent(event, index, cards.length) === null) return;
      event.preventDefault();
      void rank(
        card,
        cards,
        event.key === 'ArrowUp' ? (event.shiftKey ? 'top' : 'up') : event.shiftKey ? 'bottom' : 'down',
      );
    };

    node.addEventListener('keydown', onKeydown);
    return () => node.removeEventListener('keydown', onKeydown);
  });
</script>

{#if boardState === undefined || boardState.status === 'loading' || boardState.status === 'idle'}
  <div aria-busy="true"><Skeleton lines={3} shape="block" /></div>
{:else if failure}
  <ErrorState
    title={failure.message}
    reference={failure.reference}
    referenceLabel={t('app.reference')}
    retryLabel={t('app.retry')}
    onRetry={() => items.openBoard(collectionId, query)()}
  />
{:else if columns.length === 0}
  <!-- §4.1: say what this place is for, and offer the one action. A board with no columns used to
       explain what a column is and offer no way to make one. -->
  <EmptyState kind="unused" title={t('app.board.no_buckets')} icon="bucket">
    {#snippet action()}
      {#if isBucketBoard && !isReadOnly}
        <Button icon="plus" onclick={() => editColumn(undefined)}>
          {t('app.board.create_column')}
        </Button>
      {/if}
    {/snippet}
  </EmptyState>
{:else}
  {#if writeFailure}<p class="failure">{writeFailure.message}</p>{/if}

  <div class="board" bind:this={board}>
    {#each [...columns, null] as bucket (bucket?.id ?? 'none')}
      {@const bucketId = bucket?.id ?? null}
      {@const cards = cardsOf(bucketId)}
      <!-- The entries with no bucket are their own column, and the API puts that group last. It is
           shown rather than hidden: a card nobody has put in a column is a card somebody has to
           find. -->
      {#if bucket !== null || cards.length > 0}
        <!-- The column as a drop zone. `elementFromPoint` reads this while a card is being carried
             across the board, which is why it is the whole column rather than the list of cards:
             an empty column is a destination too. -->
        <div class="zone" data-column-zone={bucket?.id ?? 'none'}>
          <BucketColumn
            name={bucket?.name ?? t('app.board.unbucketed')}
            count={countOf(bucketId) ?? cards.length}
            wipLimit={bucket?.wip_limit ?? null}
            overLimitLabel={t('app.board.over_limit')}
            isDoneBucket={bucket?.is_done_bucket ?? false}
            doneBucketLabel={t('app.board.done_bucket')}
          >
            {#snippet actions()}
              <!-- A column's own actions are a **bucket's**: renaming it, its limit, deleting it.
                   Grouped by anything else the column is a value rather than a place, and there is
                   nothing to act on — so the control is absent rather than dead. It used to be
                   present and dead, which is the silent ignoring this project has a rule against. -->
              {#if bucket && isBucketBoard && !isReadOnly}
                {@const column = bucket}
                <Menu
                  label={t('app.board.column_actions', { name: column.name })}
                  items={COLUMN_ACTIONS.map((action) => ({ ...action, label: t(action.label) }))}
                  onselect={(id) => choseForColumn(column.id, id)}
                >
                  {#snippet trigger(props)}
                    <IconButton
                      icon="ellipsis"
                      label={t('app.board.column_actions', { name: column.name })}
                      size="sm"
                      {...props}
                    />
                  {/snippet}
                </Menu>
              {/if}
            {/snippet}

            {#each cards as card (card.id)}
              <!-- `data-card` is how a key press finds its way back from the focused control to the
                   card, and `data-column` says which column's cards it is ranked among. -->
              <div
                class="card"
                data-card={card.id}
                data-column={bucket?.id ?? 'none'}
                data-dragging={drag.id === card.id ? '' : undefined}
                data-drop={drag.id !== null &&
                drag.id !== card.id &&
                drag.levelKey === (bucket?.id ?? 'none') &&
                drag.position === cards.indexOf(card)
                  ? ''
                  : undefined}
                style:--drag-offset={drag.id === card.id ? drag.offset : undefined}
              >
                <!-- A picture, not a control: the menu on the card is SC 2.5.7's single-pointer
                     alternative, and a second focusable element that does nothing for the keyboard
                     would be noise in the tab order rather than access. -->
                <span class="grip" data-grip aria-hidden="true">
                  <Icon name="grip-vertical" size="sm" />
                </span>
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
                          label={t('app.board.card_actions', { title: card.title })}
                          items={menuItems(card, cards)}
                          onselect={(id) => chose(card, cards, id)}
                        >
                          {#snippet trigger(props)}
                            <IconButton icon="ellipsis" label={t('app.board.card_actions', { title: card.title })} size="sm" {...props} />
                          {/snippet}
                        </Menu>
                      {/if}
                    </Inline>
                  {/snippet}
                </WorkItemCard>
              </div>
            {/each}
          </BucketColumn>
        </div>
      {/if}
    {/each}

    <!-- After the columns rather than above them: a new column is added at the end, and the control
         sits where the column will appear. -->
    {#if isBucketBoard && !isReadOnly}
      <div class="add-column">
        <Button tone="secondary" size="sm" icon="plus" onclick={() => editColumn(undefined)}>
          {t('app.board.create_column')}
        </Button>
      </div>
    {/if}
  </div>
{/if}

<BucketDialog bind:isOpen={isEditingColumn} {collectionId} bucket={editing} />

{#if deleting}
  {@const column = deleting}
  <!-- Confirmed, and the confirmation says the one thing a reader would otherwise have to guess:
       the entries are not deleted with the column. -->
  <Dialog
    bind:isOpen={isDeletingColumn}
    title={t('app.board.delete_column')}
    dismissLabel={t('app.workspace.cancel')}
  >
    <p class="notice">{t('app.board.delete_confirm')}</p>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDeletingColumn = false)}>
        {t('app.workspace.cancel')}
      </Button>
      <Button tone="danger" onclick={deleteColumn}>
        {t('app.board.column_actions', { name: column.name })}
      </Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .add-column { align-self: start; padding: var(--sp-100); }
  .notice { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  /* The card's wrapper carries what the two paths read — the identity a key press finds, and the
     state a drag draws. The card itself is the design system's. */
  .card { display: flex; align-items: start; gap: var(--sp-050); }

  .card > :global(*:last-child) { flex: 1; min-width: 0; }

  .zone { display: flex; }

  /* `touch-action: none` is what makes a drag possible on a touch screen: without it the browser
     claims the gesture for scrolling, and the board scrolls sideways, so it would claim it at
     once. On the grip alone, so the board still scrolls everywhere else. */
  .grip {
    display: inline-flex;
    flex: none;
    align-items: center;
    padding-block-start: var(--sp-100);
    color: var(--text-subtle);
    cursor: grab;
    touch-action: none;
  }

  /* Rule 6: a translate and nothing else, and it is direct manipulation rather than decoration. */
  .card[data-dragging] {
    translate: 0 var(--drag-offset);
    border-radius: var(--r-md);
    box-shadow: var(--shadow-overlay);
    /* Out of the way of the measuring: what is under the pointer has to be the board. */
    pointer-events: none;
  }

  .card[data-dragging] .grip { cursor: grabbing; }

  /* Where it would land. Rule 3: an outline rather than a tint, so it reads in greyscale. */
  .card[data-drop] {
    outline: var(--bw-thick) dashed var(--accent-primary);
    outline-offset: var(--sp-025);
    border-radius: var(--r-md);
  }

  /* Rule 6's floor: under a reduced-motion preference the card does not travel, and the state and
     the landing slot are what say what is happening. */
  @media (prefers-reduced-motion: reduce) {
    .card[data-dragging] { translate: none; }
  }

  :global([data-motion='reduced']) .card[data-dragging] { translate: none; }

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
