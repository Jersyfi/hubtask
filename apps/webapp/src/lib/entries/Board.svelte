<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import AddRow from './AddRow.svelte';
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

  import { capture as captureField } from '../capture/capture.svelte.ts';

  import {
    BucketColumn,
    Button,
    Checkbox,
    Dialog,
    EmptyState,
    ErrorState,
    IconButton,
    Icon,
    LabelChip,
    Menu,
    Skeleton,
    ViewSwitcher,
    VisuallyHidden,
    WorkItemCard,
    rankIntent,
    rankTarget,
    type RankCommand,
    type View,
  } from '@hubtask/design-system/components';
  import type { Bucket, BulkResult, WorkflowShape, WorkItem } from '@hubtask/sync-engine';

  import type { ItemsQuery } from '../data/items.svelte.ts';

  import BucketDialog from './BucketDialog.svelte';
  import ReplicaMark from '../frame/ReplicaMark.svelte';
  import { createDrag } from './dragging.svelte.ts';
  import EntryActions from './EntryActions.svelte';
  import EntryMenu from './EntryMenu.svelte';
  import { attachEntryGestures } from './entryGestures.ts';
  import { entryMenu } from './entryMenu.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { archivalOfItem } from '../data/lifecycle.ts';
  import { isSwipe, swipeThreshold } from './swipe.ts';

  import { announcer } from '../announce.svelte.ts';
  import { celebration } from '../celebration.svelte.ts';
  import CelebrationSlot from './CelebrationSlot.svelte';
  import { buckets } from '../data/buckets.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { statusLabel, workflows } from '../data/workflows.svelte.ts';
  import { queue } from '../data/queue.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { selection } from '../data/selection.svelte.ts';
  import DueMark from './DueMark.svelte';
  import { outcomeOf } from '../data/bulk.ts';
  import { coverImageIdOf } from '../data/media.ts';
  import { anchorFor } from '../data/rank.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import PeopleMarks from '../people/PeopleMarks.svelte';
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
    /**
     * What the last bulk did, by entry.
     *
     * Handed in rather than read from a store, because the report belongs to the rows: a refusal
     * is about an entry and the place a reader looks for it is the row it happened to. An entry
     * that is not in the map was not part of that bulk.
     */
    lastResults?: ReadonlyMap<string, BulkResult>;
    /**
     * Asked to open a card beside the board rather than on its page (ADR-0061 decision 4), from
     * `large` up; without it a card is the link it is.
     */
    onopen?: (itemId: string) => void;
    /** The card open beside the board, so it is announced as current. */
    currentId?: string;
    /** Where a link the board cannot follow itself leads: the entry's page, a copy made from it. */
    onnavigate?: (path: string) => void;
    /** Whether the reader may share an entry here (`MANAGE_MEMBERS` along the path, D5). */
    mayShare?: boolean;
    /**
     * Why the reader's role may not change this card, or nothing where it may (UC-WRK-22 checks
     * 1, 2): such a card is not carried, ranked or moved between columns, and its menu offers only
     * a copy where the reader may create one.
     */
    changeRefusal: (item: WorkItem) => string | undefined;
    /** Whether the reader may create entries here - what a copy is. */
    mayCreate: boolean;
    /** Whether the reader may change the board's columns, which are the collection's shape (`STRUCTURE`). */
    mayShape: boolean;
    /**
     * The workflow the collection runs, where it runs one: the board is then one column per status
     * (UC-FLW-03 check 2), a card carried into another column takes the step to that status - the
     * server decides, as on every door - and the buckets stay out of it.
     */
    statuses?: WorkflowShape;
  }

  const {
    collectionId,
    isReadOnly = false,
    query,
    lastResults,
    onopen,
    currentId,
    onnavigate,
    mayShare = false,
    changeRefusal,
    mayCreate,
    mayShape,
    statuses,
  }: Props = $props();

  /** Whether a card may be carried, ranked or moved: not archived, and the reader's to change. */
  const isMovable = (card: WorkItem) => !isReadOnly && changeRefusal(card) === undefined;

  /** The sentence one card shows about the last bulk, or nothing where it was not in it. */
  function bulkNote(itemId: string): string | undefined {
    const result = lastResults?.get(itemId);
    if (!result) return undefined;
    if (outcomeOf(result) === 'applied') return undefined;
    // The server's own sentence wherever it sent one — including for a rollback, whose problem
    // names the operation that caused it. The client's own is the fallback for the shape the
    // schema describes, which carries no problem at all.
    return result.problem
      ? renderProblem(result.problem as never, messages).message
      : t('app.bulk.rolled_back');
  }

  $effect(() => {
    const wanted = collectionId;
    const byStatus = statuses !== undefined;
    const asked = byStatus ? { ...query, group: { field: 'status', limit_per_group: query?.group?.limit_per_group ?? 50 } } : query;
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
   * The card whose menu should hold focus again once the board has been read back. A card that
   * changed column is a card destroyed in one column and made anew in another, and the browser
   * drops focus to `body` in between, which leaves a keyboard reader at the top of the page
   * after every move. Set by `moveTo`, spent by the effect below the moment the board
   * that arrives holds the card - and only where nothing else took focus meanwhile.
   */
  let refocusId = $state<string | undefined>(undefined);

  $effect(() => {
    const wanted = refocusId;
    void groups;
    if (!wanted) return;
    queueMicrotask(() => {
      const trigger = document.querySelector<HTMLElement>(`[data-card-menu="${wanted}"] button`);
      if (!trigger) return;
      if (document.activeElement === document.body) trigger.focus();
      refocusId = undefined;
    });
  });

  /**
   * Whether this board is the bucket board.
   *
   * It is, unless the reader has grouped by something else — and then the columns are the values
   * the query grouped on rather than the collection's buckets, because a `wip_limit` and a done
   * column are properties of a bucket and mean nothing about a title or a type.
   */
  const isStatusBoard = $derived(statuses !== undefined);
  const isBucketBoard = $derived(!isStatusBoard && (query?.group?.field ?? 'bucket_id') === 'bucket_id');

  const columns = $derived(
    statuses
      ? statuses.statuses.map((status) => ({
          id: status.key,
          name: statusLabel(status, t),
          wip_limit: null,
          is_done_bucket: status.category === 'DONE',
          version: 0,
        }))
      : isBucketBoard
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
  /**
   * Below `medium` the board shows one column at a time (ADR-0061 decision 4), the columns as a
   * strip above it: a tap or a swipe switches. The strip is `ViewSwitcher` - one subject, several
   * renderings - and each column keeps its own head with its own actions. Moving a card between
   * columns is the card's menu, which it has (SC 2.5.7). The strip lists every column that would be
   * drawn, the entries with no column included where there are any.
   */
  /** The column the reader chose; `undefined` until they chose one, and then the first is shown. */
  let shownColumn = $state<string | null | undefined>(undefined);
  const strip = $derived<readonly { id: string | null; label: string; count: number }[]>(
    [null, ...columns]
      .filter((bucket) => bucket !== null || cardsOf(null).length > 0)
      .map((bucket) => ({ id: bucket?.id ?? null, label: bucket?.name ?? t('app.board.unbucketed'), count: countOf(bucket?.id ?? null) ?? cardsOf(bucket?.id ?? null).length })),
  );
  const stripViews = $derived<View[]>(strip.map((column) => ({ id: column.id ?? 'none', label: t('app.board.column_with_count', { name: column.label, count: String(column.count) }) })));
  const shownKey = $derived(
    shownColumn !== undefined && strip.some((column) => (column.id ?? 'none') === (shownColumn ?? 'none'))
      ? (shownColumn ?? 'none')
      : (strip[0]?.id ?? 'none'),
  );
  const isOneColumn = $derived(viewport.isCompact);

  function showColumn(key: string) {
    shownColumn = key === 'none' ? null : key;
  }

  /** A swipe across the board switches columns: a horizontal move that is more than a nudge. */
  let swipeStart: { x: number; y: number } | null = null;
  function onSwipeStart(event: TouchEvent) {
    const touch = event.touches[0];
    swipeStart = touch ? { x: touch.clientX, y: touch.clientY } : null;
  }
  function onSwipeEnd(event: TouchEvent) {
    const touch = event.changedTouches[0];
    if (!swipeStart || !touch || !isOneColumn) return;
    const dx = touch.clientX - swipeStart.x;
    const dy = touch.clientY - swipeStart.y;
    swipeStart = null;
    // Far enough sideways, and more sideways than down: a scroll is not a swipe (`swipe.ts`, the
    // rule the list's rows read too).
    if (!isSwipe(dx, dy, swipeThreshold(board))) return;
    const index = strip.findIndex((column) => (column.id ?? 'none') === shownKey);
    const dir = board !== null && getComputedStyle(board).direction === 'rtl' ? -1 : 1;
    const next = strip[index + (dx < 0 ? dir : -dir)];
    if (next) showColumn(next.id ?? 'none');
  }
  // Listened for in an effect rather than bound on the element: a `<div>` with a touch handler is
  // a static element with an interaction, and the a11y lint is right to say so. A swipe is a
  // pointer convenience over the strip, which is the control; nothing is reachable by it alone.
  $effect(() => {
    const element = board;
    if (!element) return;
    element.addEventListener('touchstart', onSwipeStart, { passive: true });
    element.addEventListener('touchend', onSwipeEnd, { passive: true });
    return () => {
      element.removeEventListener('touchstart', onSwipeStart);
      element.removeEventListener('touchend', onSwipeEnd);
    };
  });
  function cardsOf(bucketId: string | null): readonly WorkItem[] {
    return groups.find((group) => group.key === bucketId)?.data ?? [];
  }

  function countOf(bucketId: string | null): number | null {
    return groups.find((group) => group.key === bucketId)?.count ?? null;
  }

  /**
   * Every card on the board, column by column and in the order each column draws them.
   *
   * A range therefore runs *through the board as it is read* — down a column and on into the next
   * — rather than through one column alone, which is what somebody shift-clicking across two
   * columns means. Cards that have left the board leave the selection with them.
   */
  const visibleIds = $derived(
    columns.flatMap((column) => cardsOf(column.id).map((card) => card.id)).concat(
      cardsOf(null).map((card) => card.id),
    ),
  );

  $effect(() => {
    selection.keepVisible(visibleIds);
  });

  const available = $derived(labels.of(collectionId));

  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /** The moment standing: on the card that was completed, whichever tier. */
  const moment = $derived(celebration.current);
  $effect(() => () => celebration.dismiss());

  // Managing the columns themselves. `buckets` has `create`, `update`, `remove` and `reorder`,
  // and without this a collection's board could only be set up outside the application -
  // including `wip_limit` and `is_done_bucket`, both of which this board reads and
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
      announcer.say(t('app.buckets.removed_announced'));
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
    const target = columns.find((column) => column.id === bucketId);
    // A limit is advice: the move goes ahead, and is announced as going over it (UC-WRK-31 check 3).
    const isOverLimit =
      target?.wip_limit != null && item.bucket_id !== bucketId && (countOf(bucketId) ?? 0) + 1 > target.wip_limit;
    try {
      let moved = await items.setBucket(item.id, bucketId, item.version);

      if (target?.is_done_bucket && !moved.completion?.is_completed) {
        moved = await items.setCompleted(moved.id, true, crypto.randomUUID());
        // A card dropped into the done column is a completion, and a completion may be a moment.
        void celebration.celebrate(moved);
      }
      announcer.say(
        t(isOverLimit ? 'app.board.moved_over_limit' : 'app.board.moved', {
          title: item.title,
          name: target?.name ?? t('app.board.unbucketed'),
          limit: String(target?.wip_limit ?? ''),
        }),
      );
      refocusId = item.id;
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
   * Takes the step into a status column (UC-FLW-03 check 2): the same call the status control
   * makes, so the same rules hold. A refusal names what the step still needs, and the card stays
   * where the server has it.
   */
  async function stepTo(item: WorkItem, status: string): Promise<WorkItem | undefined> {
    writeFailure = undefined;
    const target = columns.find((column) => column.id === status);
    try {
      const moved = await workflows.take(item, { to_status: status });
      if (moved.completion?.is_completed && !item.completion?.is_completed) void celebration.celebrate(moved);
      announcer.say(t('app.board.moved', { title: item.title, name: target?.name ?? status, limit: '' }));
      refocusId = item.id;
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

  /**
   * A card's menu: the one every surface draws (UC-WRK-28 check 1), ranked within its column, and
   * after the moves the columns - but only where a column is a **bucket**. Grouping by something
   * else makes the columns values rather than places, and "move this entry into `WORK_PACKAGE`" is
   * not an offer: a type is a property of the entry, not a column it can be put in.
   */
  function menuItems(card: WorkItem, cards: readonly WorkItem[]) {
    const level = actions?.levelOf(card, cards);
    return entryMenu(
      {
        surface: 'board',
        item: {
          title: card.title,
          isCompleted: card.completion?.is_completed ?? false,
          hasDue: Boolean(card.due_at),
        },
        archival: archivalOfItem(card, false),
        isCollectionReadOnly: isReadOnly,
        mayChange: changeRefusal(card) === undefined,
        mayCreate,
        mayShare,
        carries: (capability) => supports(card.type, capability).status === 'permitted',
        level: level && { ...level, outReason: level.outReason },
        extras: isStatusBoard
          ? columns.filter((target) => target.id !== card.status).map((target) => ({
              id: `status:${target.id}`,
              label: t('app.board.move_to', { title: card.title, name: target.name }),
            }))
          : isBucketBoard
          ? [...columns, null].map((target) => ({
              id: `bucket:${target?.id ?? 'none'}`,
              label: t('app.board.move_to', { title: card.title, name: target?.name ?? t('app.board.unbucketed') }),
            }))
          : [],
      },
      t,
    );
  }

  /** The menus of the cards, so a right click, a key or a long press can open one. */
  const menus = $state<Record<string, EntryMenu | undefined>>({});
  let actions = $state<EntryActions | undefined>(undefined);

  function chose(card: WorkItem, cards: readonly WorkItem[], id: string) {
    if (id === 'select') {
      selection.pick(visibleIds, card.id);
      return;
    }
    if (id === 'open' && !onopen) {
      onnavigate?.(`/items/${card.id}`);
      return;
    }
    // Ranked within the column, which is a position in the level the reader can see.
    if (id === 'up' || id === 'down' || id === 'top' || id === 'bottom') {
      void rank(card, cards, id);
      return;
    }
    if (id.startsWith('status:')) {
      void stepTo(card, id.slice('status:'.length));
      return;
    }
    if (id.startsWith('bucket:')) {
      const bucketId = id.slice('bucket:'.length);
      void moveTo(card, bucketId === 'none' ? null : bucketId);
      return;
    }
    void actions?.run(card, id, cards);
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
    // The card is the handle (ADR-0063 decision 11). A board is the one surface where moving the
    // thing *is* what it is for, and the 14 × 22 px grip beside it was an affordance nobody found:
    // the gesture always worked, and nothing said so.
    handle: '[data-card]',
    start: (grip) => {
      const held = grip.closest('[data-card]');
      const cardId = held?.getAttribute('data-card');
      const key = held?.getAttribute('data-column') ?? 'none';
      const cards = cardsOf(key === 'none' ? null : key);
      const index = cards.findIndex((each) => each.id === cardId);
      const card = cards[index];
      if (!cardId || index < 0 || !card || !isMovable(card)) return null;
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
    // A finger held still on a card and lifted asks for its menu (ADR-0088); one that moves drags.
    onhold: (id) => menus[id]?.open(0),
  });

  /** The card elements of one column, in drawn order, for the measuring. */
  function elementsOf(key: string): HTMLElement[] {
    return [...(board?.querySelectorAll<HTMLElement>(`[data-column="${key}"]`) ?? [])];
  }

  async function drop(card: WorkItem, from: string, to: string, position: number) {
    // Across columns is a column change only where a column is a bucket. On any other grouping the
    // drag ranks within the column it was dropped in and changes nothing else, because the value
    // it was grouped by is not a place an entry can be put.
    if (isStatusBoard && from !== to && to !== 'none') {
      const cards = cardsOf(to);
      const moved = await stepTo(card, to);
      if (moved) await items.reorder(card.id, anchorFor(cards, card.id, position), moved.version, crypto.randomUUID());
      return;
    }
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

  /** Whether columns lie past the board's start and end edge, for the shadows that say so. */
  let edges = $state({ start: false, end: false });

  function measureEdges() {
    if (!board || isOneColumn) {
      edges = { start: false, end: false };
      return;
    }
    // `scrollLeft` runs negative from zero in a right-to-left board, so its size is what counts.
    const travelled = Math.abs(board.scrollLeft);
    const room = board.scrollWidth - board.clientWidth;
    edges = { start: travelled > 1, end: room - travelled > 1 };
  }

  $effect(() => {
    if (!board) return;
    const watched = board;
    const observer = new ResizeObserver(measureEdges);
    observer.observe(watched);
    for (const child of watched.children) observer.observe(child);
    measureEdges();
    return () => observer.disconnect();
  });

  /**
   * Where the carried card came from and how much room it takes, read once when the drag begins:
   * the cards it passes slide aside by that much, so the landing place is a real gap and nothing is
   * drawn over anything (design-system.md §11.9).
   */
  let dragOrigin = $state<{ key: string; index: number } | null>(null);
  let dragGap = $state<string | undefined>(undefined);
  /**
   * Where the carried card's copy starts, in the viewport: the copy is what follows the pointer,
   * above every column, because a column scrolls its cards and so clips anything inside it that
   * moves out of it (design-system.md §11.9). The card itself keeps its slot, unseen.
   */
  let ghost = $state<{ readonly card: WorkItem; readonly start: string; readonly top: string; readonly width: string } | null>(null);
  $effect(() => {
    const id = drag.id;
    if (id === null) {
      dragOrigin = null;
      dragGap = undefined;
      ghost = null;
      return;
    }
    untrack(() => {
      const element = board?.querySelector<HTMLElement>(`[data-card="${id}"]`);
      const card = [...columns.map((column) => column.id), null].flatMap((key) => cardsOf(key)).find((each) => each.id === id);
      if (element && card) {
        const box = element.getBoundingClientRect();
        const isRtl = getComputedStyle(element).direction === 'rtl';
        // Measured, not designed: the card's own place on the screen when it was picked up, from
        // the start of the line - the right edge in a right-to-left document.
        // design-system-lint-ignore: a measured box is physical; the start edge is worked out from it.
        ghost = { card, start: `${isRtl ? innerWidth - box.right : box.left}px`, top: `${box.top}px`, width: `${box.width}px` };
      }
      const key = element?.getAttribute('data-column') ?? 'none';
      dragOrigin = { key, index: cardsOf(key === 'none' ? null : key).findIndex((each) => each.id === id) };
      const parent = element?.parentElement;
      const rowGap = parent ? Number.parseFloat(getComputedStyle(parent).rowGap) || 0 : 0;
      // The card's own height and the column's gap: what a card measures, not a design value.
      dragGap = element ? `${element.offsetHeight + rowGap}px` : undefined;
    });
  });

  /** Which way a card slides while another is carried past it, if at all. */
  function shiftOf(card: WorkItem, key: string, cards: readonly WorkItem[]): 'up' | 'down' | undefined {
    const origin = dragOrigin;
    const target = drag.levelKey;
    const to = drag.position;
    if (drag.id === null || card.id === drag.id || origin === null || target === null || to === null) return undefined;
    const index = cards.indexOf(card);
    if (origin.key === key && target === key) {
      if (to > origin.index && index > origin.index && index <= to) return 'up';
      if (to < origin.index && index >= to && index < origin.index) return 'down';
      return undefined;
    }
    if (origin.key === key && index > origin.index) return 'up';
    if (target === key && index >= to) return 'down';
    return undefined;
  }

  const cardIdAt = (target: EventTarget | null) =>
    (target as Element | null)?.closest?.('[data-card]')?.getAttribute('data-card') ?? undefined;

  /**
   * A right click and the context-menu key open a card's menu; the long press is the drag
   * helper's, which opens it on a still release (`onhold`).
   */
  $effect(() => {
    const node = board;
    if (!node) return;
    return attachEntryGestures(node, { idAt: cardIdAt, open: (id) => menus[id]?.open(0), ignore: '[data-card]' });
  });

  /**
   * A click anywhere on a card opens it, not only on its title (concept round 3): a press that
   * moves is the drag, whose release the drag helper keeps from becoming a click, and a press held
   * still on a touch screen is the menu. The title stays the link the keyboard and a middle click
   * reach; the card's own controls keep their clicks.
   */
  $effect(() => {
    const node = board;
    if (!node) return;
    const onClick = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      if (selection.isOn) return;
      const target = event.target as Element | null;
      if (target?.closest('a, button, input, label, select, textarea, [role="menu"], [role="menuitem"]')) return;
      const id = cardIdAt(target);
      if (!id) return;
      if (onopen) onopen(id);
      else onnavigate?.(`/items/${id}`);
    };
    node.addEventListener('click', onClick);
    return () => node.removeEventListener('click', onClick);
  });

  /**
   * `Ctrl`/`⌘`-click picks a card (UC-WRK-28 check 4), captured before the link under it, as the
   * list does.
   */
  $effect(() => {
    const node = board;
    if (!node) return;
    const onClick = (event: MouseEvent) => {
      if (!(event.metaKey || event.ctrlKey)) return;
      const id = cardIdAt(event.target);
      if (!id) return;
      event.preventDefault();
      event.stopPropagation();
      selection.pick(visibleIds, id);
    };
    node.addEventListener('click', onClick, true);
    return () => node.removeEventListener('click', onClick, true);
  });

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
      if (!card || !isMovable(card)) return;

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
{:else if columns.length === 0 && cardsOf(null).length === 0}
  <ReplicaMark state={boardState} />
  <!-- §4.1: say what this place is for, and offer the one action. A board with no columns that
       explained what a column is and offered no way to make one would be a dead end. Only while
       there is no entry either: entries without a column keep their lane (ux-inventory.md E5), or switching to
       the board would make the work disappear. -->
  <EmptyState kind="unused" title={t('app.board.no_buckets')} icon="bucket">
    {#snippet action()}
      {#if isBucketBoard && !isReadOnly && mayShape}
        <Button icon="plus" onclick={() => editColumn(undefined)}>
          {t('app.board.create_column')}
        </Button>
      {/if}
    {/snippet}
  </EmptyState>
{:else}
  <ReplicaMark state={boardState} />
  {#if writeFailure}<p class="failure" role="alert">{writeFailure.message}</p>{/if}
  <EntryActions bind:this={actions} onopen={(id) => (onopen ? onopen(id) : onnavigate?.(`/items/${id}`))} {onnavigate} onselect={(id) => selection.pick(visibleIds, id)} />

  {#if isOneColumn && strip.length > 1}
    <div class="strip">
      <ViewSwitcher label={t('app.board.columns')} views={stripViews} selected={shownKey} onselect={showColumn} />
    </div>
  {/if}

  <!-- The edges say there are more columns past them: a shadow over the board, not behind it,
       so the columns cannot cover it. -->
  <div class="frame" data-more-start={edges.start ? '' : undefined} data-more-end={edges.end ? '' : undefined}>
  <!-- design-system-lint-ignore: the gap is the carried card's measured height, not a design value. -->
  <div class="board" data-one-column={isOneColumn ? '' : undefined} bind:this={board} onscroll={measureEdges} style:--drag-gap={dragGap}>
    {#each [null, ...columns] as bucket (bucket?.id ?? 'none')}
      {@const bucketId = bucket?.id ?? null}
      {@const cards = cardsOf(bucketId)}
      <!-- The entries with no bucket are their own lane, first (design-system.md §11.9): a card
           nobody has put in a column is a card somebody has to find, and it is found before the
           columns rather than after them. On a phone only the column the strip chose is drawn. -->
      {#if (bucket !== null || cards.length > 0) && (!isOneColumn || (bucket?.id ?? 'none') === shownKey)}
        <!-- The column as a drop zone. `elementFromPoint` reads this while a card is being carried
             across the board, which is why it is the whole column rather than the list of cards:
             an empty column is a destination too. -->
        <div class="zone" data-column-zone={bucket?.id ?? 'none'} data-over={drag.id !== null && drag.levelKey === (bucket?.id ?? 'none') ? '' : undefined}>
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
                   nothing to act on — so the control is absent rather than dead. Present and
                   dead, it would be the silent ignoring this project has a rule against. -->
              {#if bucket && isBucketBoard && !isReadOnly && mayShape}
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
                data-celebrating={moment && moment.item.id === card.id ? '' : undefined}
                data-dragging={drag.id === card.id ? '' : undefined}
                data-shift={shiftOf(card, bucket?.id ?? 'none', cards)}
              >
                <!-- The same pick the list has, and deliberately the same code behind it: one
                     selection across both layouts, drawn only while somebody is selecting, and
                     shift read from the event rather than from a mode this component would keep.
                     The mode is the screen's, so switching between the list and the board keeps
                     it (ADR-0063 decision 8). -->
                {#if selection.isOn}
                <span class="pick">
                  <Checkbox
                    label={t('app.bulk.select', { title: card.title })}
                    isLabelHidden={true}
                    checked={selection.has(card.id)}
                    onclick={(event: MouseEvent) =>
                      selection.pick(visibleIds, card.id, { range: event.shiftKey })}
                    onkeydown={(event: KeyboardEvent) => {
                      if (event.key !== ' ' && event.key !== 'Enter') return;
                      event.preventDefault();
                      selection.pick(visibleIds, card.id, { range: event.shiftKey });
                    }}
                  />
                </span>
                {/if}
                {#if moment && moment.item.id === card.id}
                  <CelebrationSlot current={moment} />
                {/if}
                <WorkItemCard
                  title={card.title}
                  href={`/items/${card.id}`}
                  onOpen={onopen ? () => onopen(card.id) : undefined}
                  isCurrent={currentId === card.id}
                  pendingLabel={queue.isPending(card.id) ? t('app.sync.pending_entry') : undefined}
                  isCompleted={card.completion?.is_completed ?? false}
                  coverKind={card.cover?.kind ?? null}
                  coverColorToken={card.cover?.color_token ?? null}
                  coverImageUrl={media.coverUrl(coverImageIdOf(card.cover), Date.now()) ?? null}
                  coverAlt=""
                >
                  <!-- The card's zones (concept round 2, §3): labels above the title, at most two and
                       "+n"; one footer line with the date and the counts, the people at its end. -->
                  {#snippet labels()}
                    {@const carried = (card.label_ids ?? []).map((labelId) => available.find((each) => each.id === labelId)).filter((label) => label !== undefined)}
                    {#each carried.slice(0, 2) as label (label.id)}
                      <LabelChip name={label.name} colorToken={label.color_token} />
                    {/each}
                    {#if carried.length > 2}
                      <span class="more-labels" title={carried.slice(2).map((label) => label.name).join(', ')}>
                        <span aria-hidden="true">+{carried.length - 2}</span><VisuallyHidden>{t('app.board.more_labels', { count: String(carried.length - 2) })}</VisuallyHidden>
                      </span>
                    {/if}
                  {/snippet}
                  {#snippet footer()}
                    <DueMark item={card} />
                    {#if card.counts && card.counts.parts > 0}
                      <span class="count"><Icon name="square-check" size="sm" /><span aria-hidden="true">{card.counts.parts_done}/{card.counts.parts}</span><VisuallyHidden>{t('app.board.parts_done', { done: String(card.counts.parts_done), count: String(card.counts.parts) })}</VisuallyHidden></span>
                    {/if}
                    {#if card.counts && card.counts.comments > 0}
                      <span class="count"><Icon name="message-square" size="sm" /><span aria-hidden="true">{card.counts.comments}</span><VisuallyHidden>{t('app.board.comments', { count: String(card.counts.comments) })}</VisuallyHidden></span>
                    {/if}
                    {#if card.counts && card.counts.attachments > 0}
                      <span class="count"><Icon name="paperclip" size="sm" /><span aria-hidden="true">{card.counts.attachments}</span><VisuallyHidden>{t('app.board.attachments', { count: String(card.counts.attachments) })}</VisuallyHidden></span>
                    {/if}
                  {/snippet}
                  {#snippet people()}
                    <PeopleMarks assigneeId={card.assignee_id} memberIds={card.member_ids ?? []} />
                  {/snippet}
                </WorkItemCard>
                <!-- In the card's corner rather than a line of its own, shown on hover and focus where
                     there is a hover. The keyboard path to moving a card - the drag is built against it, because
                     WCAG 2.2 SC 2.5.7 wants a single-pointer alternative and a rank change is
                     a command before it is a gesture. -->
                <span class="menu" data-card-menu={card.id}>
                  <EntryMenu
                    bind:this={menus[card.id]}
                    title={card.title}
                    items={menuItems(card, cards)}
                    onselect={(id) => chose(card, cards, id)}
                  />
                </span>
                {#if bulkNote(card.id)}
                  <p class="bulk-note">{bulkNote(card.id)}</p>
                {/if}
              </div>
            {/each}
            <!-- The column's own door to capture: pointed at this column, so an entry lands where it
                 is added (design-system.md §11.8). The same field as everywhere else. -->
            {#if mayCreate && !isReadOnly && isBucketBoard}
              <AddRow label={t('app.entries.add')} onclick={() => captureField.open({ collectionId, bucketId: bucket?.id })} />
            {/if}
          </BucketColumn>
        </div>
      {/if}
    {/each}

    <!-- After the columns rather than above them: a new column is added at the end, and the dashed
         column sits where it will appear, as the concept draws it. -->
    {#if isBucketBoard && !isReadOnly && mayShape}
      <button type="button" class="add-column" onclick={() => editColumn(undefined)}>
        <Icon name="plus" size="sm" />{t('app.board.create_column')}
      </button>
    {/if}

    {#if ghost}
      <!-- design-system-lint-ignore: where the card was picked up and how far it travelled are measured, not designed. -->
      <div
        class="ghost"
        aria-hidden="true"
        style:--ghost-start={ghost.start}
        style:--ghost-top={ghost.top}
        style:--ghost-width={ghost.width}
        style:--drag-offset={drag.offset}
        style:--drag-offset-inline={drag.offsetInline}
      >
        <WorkItemCard title={ghost.card.title} isCompleted={ghost.card.completion?.is_completed ?? false} />
      </div>
    {/if}
  </div>
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

  .notice { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  /* The card's wrapper carries what the two paths read — the identity a key press finds, and the
     state a drag draws. The card itself is the design system's. */
  .card { display: flex; align-items: start; gap: var(--sp-050); }

  /* A card is the slot a moment sits over; a column too, for the rare one. */
  .card[data-celebrating] { position: relative; }

  .card > :global(article) { flex: 1; min-width: 0; }

  .card { position: relative; }

  .menu { position: absolute; inset-block-start: var(--sp-050); inset-inline-end: var(--sp-050); }

  @media (hover: hover) {
    .card:not(:hover, :focus-within) .menu:not(:has([aria-expanded='true'])) { opacity: 0; }
  }

  .zone { display: flex; }

  /* A count on the card's footer: its mark and its number, the number said in words to a reader. */
  .count { display: inline-flex; align-items: center; gap: var(--sp-025); font-variant-numeric: tabular-nums; }
  .more-labels {
    display: inline-flex;
    align-items: center;
    padding: var(--sp-025) var(--sp-050);
    border-radius: var(--r-sm);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-050);
    font-weight: var(--fw-medium);
  }

  /* The lane of cards nobody has put in a column is not a column: dashed and open, as the
     concept draws it, so it reads as a place to sort out rather than a stage of the work. */
  .zone[data-column-zone='none'] > :global(.column) {
    background: transparent;
    outline: var(--bw-hairline) dashed var(--border-default);
    outline-offset: calc(-1 * var(--bw-hairline));
  }

  /* `touch-action: none` is what makes a drag possible on a touch screen: without it the browser
     claims the gesture for scrolling, and the board scrolls sideways, so it would claim it at
     once. On the grip alone, so the board still scrolls everywhere else. */
  .pick { display: flex; align-items: center; }

  .bulk-note { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }


  /* Rule 6: a translate and nothing else, and it is direct manipulation rather than decoration.
     Both axes, because a card carried towards another column that stayed where it was is a card
     the reader is told nothing about (ADR-0063 decision 11). */
  /* Lifted above every column - a later column must never paint over the card being carried -,
     tilted a little so it reads as held, and raised as an overlay is (rule 1). */
  /* The card keeps its slot, unseen, while its copy is carried: the gaps the others open are
     measured against the slots, and nothing the column clips is ever in view. */
  .card[data-dragging] {
    visibility: hidden;
    pointer-events: none;
  }

  /* The carried copy: above every column, tilted a little so it reads as held, and raised as an
     overlay is (rule 1). Fixed, so no column's scrolling can cut it. */
  .ghost {
    position: fixed;
    inset-block-start: var(--ghost-top);
    inset-inline-start: var(--ghost-start);
    z-index: var(--z-overlay);
    inline-size: var(--ghost-width);
    translate: var(--drag-offset-inline) var(--drag-offset);
    rotate: -1.5deg;
    border-radius: var(--r-md);
    box-shadow: var(--shadow-overlay);
    outline: var(--bw-thick) solid var(--accent-primary);
    pointer-events: none;
  }

  /* A card can be carried: the hand says so, and a press lifts it a little before it moves. */
  .card { cursor: pointer; }
  .card:active { cursor: grabbing; }
  .card:active { box-shadow: var(--shadow-raised); }

  /* The cards it passes slide aside by its height, so where it lands is a gap and nothing overlaps.
     A transform, so the layout the drag measures does not move (`dragging.svelte.ts`). */
  .card { transition: translate var(--motion-state-duration) var(--motion-state-easing); }
  .card[data-dragging] { transition: none; }
  .card[data-shift='down'] { translate: 0 var(--drag-gap); }
  .card[data-shift='up'] { translate: 0 calc(-1 * var(--drag-gap)); }

  /* Rule 6's floor: under a reduced-motion preference nothing travels or slides; the column's
     outline says where the card goes and the card's shadow which one is held. */
  @media (prefers-reduced-motion: reduce) {
    .card[data-dragging],
    .card[data-shift] { translate: none; rotate: none; transition: none; }
  }

  :global([data-motion='reduced']) .card[data-dragging],
  :global([data-motion='reduced']) .card[data-shift] { translate: none; rotate: none; transition: none; }

  @media (prefers-reduced-motion: reduce) { .ghost { rotate: none; } }
  :global([data-motion='reduced']) .ghost { rotate: none; }

  /* The column the pointer is over, so that "where would this land" is a place rather than an
     outline on one card. Rule 3: a border rather than a tint alone. */
  .zone[data-over] { border-radius: var(--r-lg); outline: var(--bw-hairline) dashed var(--border-default); outline-offset: var(--sp-050); }

  /* The gap the cards open is a translate, so the column grows by it too: the last card slides
     down into room rather than under the column's edge. */
  .zone[data-over] :global(.cards) { padding-block-end: var(--drag-gap); }

  /* A press on a card is a drag in waiting, so the browser's own text selection is out of the way
     and a touch does not scroll while one is being carried. */
  .card { touch-action: none; user-select: none; }

  /* The board scrolls sideways, not the page. Positioned, so that it is the containing block of
     what it scrolls: a card's checkbox hides its input the visually-hidden way, absolutely, and
     an absolute box inside an unpositioned scroller overflows the scroller's ancestor - which is
     how a board widens the page by four hundred pixels on a tablet. */
  .board {
    position: relative;
    display: flex;
    gap: var(--sp-200);
    align-items: start;
    overflow-x: auto;
    /* Room between the columns and the scrollbar under them, and round the columns for the dashed
       outline the column under a carried card draws outside itself - a scroller clips at its
       padding edge, so without it the outline was cut at the board's sides and top. The margin
       gives the room back, so the columns stay where the page's edge is. */
    padding-block: var(--sp-100) var(--sp-200);
    padding-inline: var(--sp-100);
    margin-block-start: calc(var(--sp-100) * -1);
    margin-inline: calc(var(--sp-100) * -1);
  }

  .frame { position: relative; }

  .frame::before,
  .frame::after {
    content: '';
    position: absolute;
    inset-block: 0 var(--sp-200);
    z-index: var(--z-raised);
    inline-size: var(--sp-300);
    opacity: 0;
    pointer-events: none;
    transition: opacity var(--motion-state-duration) var(--motion-state-easing);
  }

  .frame::before { inset-inline-start: 0; background: linear-gradient(to right, var(--bg-canvas), transparent); }
  .frame::after { inset-inline-end: 0; background: linear-gradient(to left, var(--bg-canvas), transparent); }

  :global([dir='rtl']) .frame::before { background: linear-gradient(to left, var(--bg-canvas), transparent); }
  :global([dir='rtl']) .frame::after { background: linear-gradient(to right, var(--bg-canvas), transparent); }

  .frame[data-more-start]::before,
  .frame[data-more-end]::after { opacity: 1; }

  /* The next column, dashed where it will stand. */
  .add-column {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: center;
    gap: var(--sp-100);
    box-sizing: border-box;
    inline-size: var(--layout-board-column);
    align-self: start;
    min-block-size: var(--sp-1000);
    border: var(--bw-thick) dashed var(--border-default);
    border-radius: var(--r-lg);
    background: transparent;
    color: var(--text-secondary);
    font: inherit;
    font-weight: var(--fw-medium);
    cursor: pointer;
  }
  .add-column:hover { border-color: var(--accent-primary); color: var(--text-brand); }
  .add-column:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  /* One column on a phone: it takes the width, and the strip above says which and how many. */
  .board[data-one-column] { flex-direction: column; align-items: stretch; overflow-x: visible; }

  .board[data-one-column] .zone,
  .board[data-one-column] .zone > :global(*) { box-sizing: border-box; inline-size: 100%; }

  .strip { overflow-x: auto; padding-block-end: var(--sp-050); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }
</style>
