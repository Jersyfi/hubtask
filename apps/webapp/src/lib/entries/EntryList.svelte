<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The entries of a collection, one level at a time.
  //
  // `LIST_COLLAPSED` and `LIST_EXPANDED` are not two components and not a prop: they are whether a
  // row's children are shown, one row at a time, and expanding is what fetches them. A layout that
  // read the whole subtree would be fetching levels nobody has opened.
  //
  // Which types may be created where comes from the **manifest** (`lib/data/capability.ts`), never
  // from the three names this product happens to have today: `domain-model.md` §2's extension
  // example — a new type is a profile entry and no code change — is only true if nothing here
  // spells them out.

  import { untrack } from 'svelte';

  import {
    Button,
    EmptyState,
    ErrorState,
    IconButton,
    Inline,
    Input,
    LabelChip,
    LabelPicker,
    Menu,
    Popover,
    Select,
    Icon,
    Skeleton,
    Stack,
    TaskRow,
    rankIntent,
    rankTarget,
    type RankCommand,
  } from '@hubtask/design-system/components';
  import type { DroppedReference, WorkItem } from '@hubtask/sync-engine';

  import type { ItemsQuery } from '../data/items.svelte.ts';

  import MoveDialog from './MoveDialog.svelte';
  import { createDrag } from './dragging.svelte.ts';

  import { announcer } from '../announce.svelte.ts';
  import { acceptsChild, childTypes, rootTypes, supports } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { anchorFor } from '../data/rank.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    collectionId: string;
    /** Whether the collection is archived — the entries in it are then read-only too (I-C3). */
    isReadOnly?: boolean;
    /** What the reader has asked of this level: a filter, an order. Built from the manifest. */
    query?: ItemsQuery;
    /**
     * `LIST_EXPANDED` rather than `LIST_COLLAPSED`: every row that takes children shows them.
     *
     * The layout is not a second component and not a prop on the row — F2-09's note — it is
     * whether the children are shown, and expanding is still what fetches them. So this opens the
     * rows as they arrive rather than reading a subtree the reader has not asked for.
     */
    isExpanded?: boolean;
  }

  const { collectionId, isReadOnly = false, query, isExpanded = false }: Props = $props();

  /** The entries whose children are shown. Expanding one is what reads its level. */
  let expanded = $state<string[]>([]);

  // `untrack` for the reason F2-08 records: the listener writes the store and writing it reads it,
  // so an effect that subscribes while tracking that read cancels itself before the answer lands.
  $effect(() => {
    const wanted = collectionId;
    const asked = query;
    return untrack(() => items.openCollection(wanted, asked));
  });

  // What `LIST_EXPANDED` means, one row at a time: a row that takes children is opened, and
  // opening it is what reads its level — so the rows that arrive are opened in turn rather than a
  // whole subtree being fetched at once.
  $effect(() => {
    if (!isExpanded) return;
    const opened = rows
      .filter((row) => row.takesChildren && !expanded.includes(row.item.id))
      .map((row) => row.item.id);
    if (opened.length > 0) expanded = [...expanded, ...opened];
  });

  $effect(() => {
    const open = [...expanded];
    return untrack(() => {
      const stops = open.map((id) => items.openChildren(id));
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  // The collection's labels, read once for the whole list: every row picks from the same set,
  // because a label belongs to a collection (I-W3).
  $effect(() => {
    const wanted = collectionId;
    return untrack(() => labels.open(wanted));
  });

  const available = $derived(
    labels.of(collectionId).map((entry) => ({
      id: entry.id,
      name: entry.name,
      colorToken: entry.color_token,
      description: entry.description,
    })),
  );

  async function toggleLabel(item: WorkItem, labelId: string) {
    writeFailure = undefined;
    try {
      const isOn = (item.label_ids ?? []).includes(labelId);
      await labels.setOnItem(item.id, labelId, !isOn, crypto.randomUUID());
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  interface Row {
    readonly item: WorkItem;
    readonly depth: number;
    readonly takesChildren: boolean;
    /**
     * The level this row is ranked within, and where it sits in it.
     *
     * Carried on the row rather than looked up when a command runs, because the flattened list is
     * not the level: a row at depth 2 is ranked among its parent's children, and the rows above it
     * on screen belong to other levels. `:reorder` moves an entry within its own level and nowhere
     * else, so the level is what a command has to be answered against.
     */
    readonly siblings: readonly WorkItem[];
    readonly index: number;
    /** The entry this one sits inside, or `null` for the top level of the collection. */
    readonly parentId: string | null;
  }

  /** The visible rows, in the order the eye walks them. */
  function flatten(parents: readonly WorkItem[], depth: number, parentId: string | null): Row[] {
    const rows: Row[] = [];
    parents.forEach((item, index) => {
      // Whether it *may* hold children is the manifest's answer; whether it *does* is not known
      // until it is opened, which is why the twist appears for any type that takes them.
      rows.push({
        item,
        depth,
        takesChildren: childTypes(item.type).length > 0,
        siblings: parents,
        index,
        parentId,
      });
      if (expanded.includes(item.id)) {
        rows.push(...flatten(items.childrenOf(item.id), depth + 1, item.id));
      }
    });
    return rows;
  }

  const rows = $derived(flatten(items.inCollection(collectionId), 0, null));
  // Not called `state`: a variable of that name collides with the `$state` rune in what the
  // compiler generates, and the error it produces names a line that looks unrelated.
  const levelState = $derived(items.stateOf(`container:${collectionId}`));
  const failure = $derived(
    levelState?.status === 'failed' ? renderProblem(levelState.error, messages) : undefined,
  );

  /** Which parent the add form is open under: an entry's id, `'root'`, or nothing. */
  let addingUnder = $state<string | null>(null);
  let draftTitle = $state('');
  let draftType = $state('');
  let isSaving = $state(false);
  let writeFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  const offeredTypes = $derived(
    addingUnder === null
      ? []
      : addingUnder === 'root'
        ? rootTypes()
        : childTypes(rows.find((row) => row.item.id === addingUnder)?.item.type ?? ''),
  );

  function startAdding(under: string) {
    addingUnder = under;
    draftTitle = '';
    writeFailure = undefined;
    // The first type the manifest offers, so the common case needs no choice at all.
    draftType =
      (under === 'root'
        ? rootTypes()
        : childTypes(rows.find((row) => row.item.id === under)?.item.type ?? ''))[0] ?? '';
  }

  async function create() {
    if (draftTitle.trim() === '' || draftType === '' || addingUnder === null) return;
    isSaving = true;
    writeFailure = undefined;
    try {
      await items.create(
        addingUnder === 'root'
          ? { type: draftType, collection_id: collectionId, title: draftTitle.trim() }
          : { type: draftType, parent_id: addingUnder, title: draftTitle.trim() },
        crypto.randomUUID(),
      );
      // The new entry appears because the write invalidated the level and the engine re-read it.
      // Nothing is inserted into the list by hand here.
      //
      // Expanding the parent is not decoration: a child created under a collapsed row is a row
      // nobody can see, so the reader would press Create and watch nothing happen. It opens the
      // level it was just added to, which is also what fetches it.
      if (addingUnder !== 'root' && !expanded.includes(addingUnder)) {
        expanded = [...expanded, addingUnder];
      }
      addingUnder = null;
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

  /**
   * Ranks a row within its level — the whole command path, in one function.
   *
   * **Built first, and the pointer path is measured against it.** WCAG 2.2 SC 2.5.7 wants a
   * single-pointer alternative to every dragging movement, and a rank change is a command before
   * it is a gesture: a menu item and a key press are this call, and so is a drag. Built the other
   * way round it would be a retrofit, and the retrofit is what an audit finds.
   *
   * The `Idempotency-Key` is minted **per intent**. Pressing "move down" twice is two intents and
   * moves the entry twice, which is what the reader asked for; one press delivered twice — a proxy
   * retry, a flaky connection — is one key and the server replays its own answer rather than
   * moving anything a second time.
   *
   * Nothing is inserted into the list by hand. The write invalidates `/items` and the level is
   * read again, so the order on screen is the server's order rather than this component's guess at
   * it — which is also what makes a second client's move appear rather than being overwritten.
   */
  async function rank(row: Row, command: RankCommand) {
    const target = rankTarget(command, row.index, row.siblings.length);
    if (target === null) return;
    await rankTo(row, target);
  }

  /** The call itself, once something — a command, a key or a drag — has named the position. */
  async function rankTo(row: Row, target: number) {
    writeFailure = undefined;
    try {
      await items.reorder(
        row.item.id,
        anchorFor(row.siblings, row.item.id, target),
        row.item.version,
        crypto.randomUUID(),
      );
      // Said out loud, because a rank change is invisible to a screen reader: the focused control
      // did not change and neither did its name, and the list quietly rearranged itself.
      announcer.say(
        t('app.rank.announced', {
          title: row.item.title,
          position: target + 1,
          count: row.siblings.length,
        }),
      );
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /** The reason a command is unavailable, or nothing. There is no `disabled` boolean anywhere. */
  function rankReason(row: Row, command: RankCommand): string | undefined {
    if (isReadOnly) return t('app.entries.read_only');
    if (rankTarget(command, row.index, row.siblings.length) !== null) return undefined;
    return command === 'up' || command === 'top'
      ? t('app.rank.already_first')
      : t('app.rank.already_last');
  }

  /**
   * The keyboard shortcuts, on the level rather than on a row.
   *
   * Listened for rather than bound in the markup: a `<div>` with an `onkeydown` is a static element
   * with an interaction, which Svelte warns about and is right to — the handler here belongs to the
   * *list*, and the events reach it from the row controls that are properly focusable. Attaching it
   * in an effect also means it is removed when the list is, with nothing for a caller to forget.
   */
  let level = $state<HTMLElement | null>(null);

  $effect(() => {
    const node = level;
    if (!node) return;

    const onKeydown = (event: KeyboardEvent) => {
      const id = (event.target as Element | null)?.closest?.('[data-row]')?.getAttribute('data-row');
      const row = rows.find((each) => each.item.id === id);
      if (!row || isReadOnly) return;

      const target = rankIntent(event, row.index, row.siblings.length);
      if (target === null) return;
      // Alt+Down scrolls in some engines. The command is answered, so the scroll is not.
      event.preventDefault();
      void rank(row, keyCommand(event));
    };

    node.addEventListener('keydown', onKeydown);
    return () => node.removeEventListener('keydown', onKeydown);
  });

  /** Which command a press was, once `rankIntent` has said it is one at all. */
  function keyCommand(event: KeyboardEvent): RankCommand {
    if (event.key === 'ArrowUp') return event.shiftKey ? 'top' : 'up';
    return event.shiftKey ? 'bottom' : 'down';
  }

  /**
   * The entry this row would move inside: the sibling directly above it.
   *
   * "Another parent" is a position the reader can see, so it is a command rather than a picker —
   * the sibling above is the one destination that needs no list of everything. Whether the move is
   * permitted is the **manifest's** answer and not this component's: a work package holds
   * activities on one installation and something else on the next, and `domain-model.md` §2's
   * extension example is only true if nothing here spells the three names out.
   */
  const previousSibling = (row: Row): WorkItem | undefined => row.siblings[row.index - 1];

  /** Why an entry cannot move inside the one above it, or nothing. */
  function insideReason(row: Row): string | undefined {
    if (isReadOnly) return t('app.entries.read_only');
    const target = previousSibling(row);
    if (!target) return t('app.move.nothing_above');
    const verdict = acceptsChild(target.type, row.item.type, row.depth);
    if (verdict.status === 'permitted') return undefined;
    // The server's own code where there is one, so a prediction and a refusal read the same way.
    return verdict.status === 'refused' ? t(verdict.code, verdict.params) : t('app.move.moving');
  }

  /**
   * Moves an entry to another parent, another collection, or both.
   *
   * `:move` rather than `:reorder` the moment the parent changes, and it answers a `MoveResult`
   * rather than an entry. **The second half of that answer is shown**: I-W6 says a reference the
   * destination cannot resolve is reported rather than dropped in silence, and swallowing it here
   * would turn a designed behaviour into data loss — the chips would simply be gone, which is
   * indistinguishable from a rendering fault.
   */
  async function move(
    row: Row,
    destination: { parentId: string | null; collectionId?: string },
  ) {
    writeFailure = undefined;
    try {
      const result = await items.move(
        row.item.id,
        destination,
        row.item.version,
        crypto.randomUUID(),
      );
      dropped = { title: row.item.title, references: result.dropped_references ?? [] };
      announcer.say(t('app.move.announced', { title: row.item.title }));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }

  /** What the last move left behind, until the reader has read it. */
  let dropped = $state<{ title: string; references: readonly DroppedReference[] } | undefined>(
    undefined,
  );

  /** Which row the destination picker is open for. */
  let movingRow = $state<Row | undefined>(undefined);
  let isMoving = $state(false);

  /**
   * Every collection the workspace holds, read when the picker opens.
   *
   * That is the one place it is right to read them. `containers.svelte.ts` argues against reading
   * every hub's level at boot, because that is a request per hub for rows nobody has asked to see;
   * here somebody has asked, and a picker offering only the hubs they happened to have expanded
   * would hide half the workspace.
   *
   * `untrack` around the subscribing, for the reason every store here records: the listener writes
   * the level and writing it reads it, so an effect tracking that read cancels its own
   * subscription before the answer arrives.
   */
  $effect(() => {
    if (!movingRow) return;
    const hubs = containers.hubs.map((hub) => hub.id);
    return untrack(() => {
      const stops = hubs.map((hubId) => containers.openLevel(hubId));
      return () => {
        for (const stop of stops) stop();
      };
    });
  });

  const destinations = $derived(
    containers.hubs.flatMap((hub) =>
      containers
        .collectionsOf(hub.id)
        // The collection it is already in is not a destination, and an archived one is read-only
        // (I-C3) — offering it would be offering a move the server refuses.
        .filter((collection) => collection.id !== collectionId && !collection.effective_archived)
        .map((collection) => ({
          value: collection.id,
          label: t('app.move.destination_option', { hub: hub.name, collection: collection.name }),
        })),
    ),
  );

  async function moveElsewhere(collectionId: string) {
    const row = movingRow;
    if (!row) return;
    isMoving = true;
    // The top level of the destination: an entry that had a parent here has none there, because
    // the parent is not in that collection. `target_collection_id` is what names where it lands.
    await move(row, { parentId: null, collectionId });
    isMoving = false;
    movingRow = undefined;
  }

  /**
   * Moves an entry out to the level above it.
   *
   * The grandparent, or the collection itself when the parent is already at the top level — and
   * that is why `target_parent_id` is sent as `null` *with* a collection rather than left out. An
   * entry's collection is the one its parent is in, so an entry with no parent has to name one.
   */
  function moveOut(row: Row) {
    if (!row.parentId) return;
    const parent = rows.find((each) => each.item.id === row.parentId);
    const grandParentId = parent?.parentId ?? null;
    void move(
      row,
      grandParentId === null ? { parentId: null, collectionId } : { parentId: grandParentId },
    );
  }

  const RANK_COMMANDS: readonly { readonly id: RankCommand; readonly label: string }[] = [
    { id: 'up', label: 'app.rank.up' },
    { id: 'down', label: 'app.rank.down' },
    { id: 'top', label: 'app.rank.top' },
    { id: 'bottom', label: 'app.rank.bottom' },
  ];

  /**
   * Everything a rank change or a move can do to this row, as one list.
   *
   * One menu rather than two, because they are one question to the reader — "where should this go"
   * — and the API's split between `:reorder` and `:move` is an answer about which operation, not
   * about which control. The separator is where the level stops being the level.
   */
  function menuItems(row: Row) {
    const above = previousSibling(row);
    return [
      ...RANK_COMMANDS.map((command) => ({
        id: command.id,
        label: t(command.label),
        disabledReason: rankReason(row, command.id),
      })),
      {
        id: 'inside',
        label: above ? t('app.move.inside', { name: above.title }) : t('app.move.inside_above'),
        disabledReason: insideReason(row),
        hasSeparatorBefore: true,
      },
      {
        id: 'out',
        label: t('app.move.out'),
        disabledReason: isReadOnly
          ? t('app.entries.read_only')
          : row.parentId
            ? undefined
            : t('app.move.already_top'),
      },
      {
        id: 'elsewhere',
        label: t('app.move.elsewhere'),
        disabledReason: isReadOnly ? t('app.entries.read_only') : undefined,
      },
    ];
  }

  /**
   * The pointer path, and it reaches the same call.
   *
   * A drag names a **position**, which is the number the menu items already name, so the two
   * cannot come to disagree about where the fourth row lands — and a drag that ends where it
   * began writes nothing, which is what makes thinking better of it free.
   *
   * The level is the row's own, and a drag stays in it: `:reorder` is a rank within a level, and a
   * gesture that reparented an entry by drifting over another level's rows would be performing
   * `:move` without the reader asking for it. Changing the parent is the menu's, where it is a
   * decision rather than a slip.
   */
  const drag = createDrag({
    start: (grip) => {
      const id = grip.closest('[data-row]')?.getAttribute('data-row');
      const row = rows.find((each) => each.item.id === id);
      if (!row || isReadOnly || !level) return null;
      return {
        id: row.item.id,
        index: row.index,
        level: {
          key: row.parentId ?? 'root',
          // The level's rows in drawn order — the siblings, not everything on screen. A row at
          // depth 2 is ranked among its parent's children, and the rows above it belong to others.
          elements: row.siblings
            .map((sibling) => level?.querySelector<HTMLElement>(`[data-row="${sibling.id}"]`))
            .filter((element): element is HTMLElement => element !== null),
        },
      };
    },
    ondrop: ({ id, to }) => {
      const row = rows.find((each) => each.item.id === id);
      if (!row) return;
      void rankTo(row, to);
    },
  });

  $effect(() => {
    const node = level;
    return node ? drag.attach(node) : undefined;
  });

  /** One press, whichever of the two operations it turns out to be. */
  function chose(row: Row, id: string) {
    if (id === 'inside') {
      const above = previousSibling(row);
      if (above) void move(row, { parentId: above.id });
    } else if (id === 'out') moveOut(row);
    else if (id === 'elsewhere') movingRow = row;
    else void rank(row, id as RankCommand);
  }

  async function toggleComplete(item: WorkItem) {
    writeFailure = undefined;
    try {
      // Re-read rather than predicted: with `completionPolicy = ROLLUP` a parent completes when its
      // children do (I-W5), and that is a change this client learns by asking.
      await items.setCompleted(item.id, !item.completion?.is_completed, crypto.randomUUID());
    } catch (error) {
      writeFailure = renderProblem(error as never, messages);
    }
  }
</script>

<Stack gap="200">
  {#if levelState === undefined || levelState.status === 'loading' || levelState.status === 'idle'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if failure}
    <ErrorState
      title={failure.message}
      reference={failure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => items.openCollection(collectionId)()}
    />
  {:else}
    {#if writeFailure}
      <p class="failure">{writeFailure.message}</p>
    {/if}

    <!-- I-W6, on the screen. A label belongs to a collection and a column belongs to a board, so
         an entry carried to another collection leaves both behind — and the operation reports what
         it left rather than dropping it in silence. Showing it is the part that makes the report
         worth having; a client that swallowed it would turn a designed behaviour into data loss. -->
    {#if dropped && dropped.references.length > 0}
      <div class="dropped">
        <p>{t('app.move.dropped', { title: dropped.title })}</p>
        <ul>
          {#each dropped.references as reference, index (`${reference.kind}-${reference.id}-${index}`)}
            <li>
              <strong>{t(`app.move.dropped_kind.${reference.kind}`)}</strong>
              <!-- The server's own message code says why. One fact, one sentence, whichever
                   channel reported it (ADR-0011). -->
              {t(reference.code)}
            </li>
          {/each}
        </ul>
        <div>
          <Button size="sm" tone="secondary" onclick={() => (dropped = undefined)}>
            {t('app.dismiss')}
          </Button>
        </div>
      </div>
    {/if}

    {#if rows.length === 0}
      <!-- The form belongs here as well as below. §4.1's empty state is the only one that carries a
           call to action, and a call to action that sets a flag nothing renders is a button that
           does nothing — which is what this was until it was pressed. -->
      {#if addingUnder === 'root'}
        {@render addForm()}
      {:else}
        <EmptyState kind="unused" title={t('app.entries.none')} icon="task">
          {#snippet action()}
            {#if !isReadOnly && rootTypes().length > 0}
              <Button onclick={() => startAdding('root')}>{t('app.entries.add')}</Button>
            {/if}
          {/snippet}
        </EmptyState>
      {/if}
    {:else}
      <!-- The level, and the element the shortcuts are listened for on. `data-row` is what a key
           press finds its way back from the focused control to the row by; the handler is added in
           an effect rather than bound here, because a `<div>` with an `onkeydown` is a static
           element with an interaction and Svelte is right to warn about one. -->
      <div class="level" bind:this={level}>
        {#each rows as row (row.item.id)}
          <div
            class="row"
            data-row={row.item.id}
            data-dragging={drag.id === row.item.id ? '' : undefined}
            data-drop={drag.id !== null &&
            drag.id !== row.item.id &&
            drag.levelKey === (row.parentId ?? 'root') &&
            drag.position === row.index
              ? ''
              : undefined}
            style:--drag-offset={drag.id === row.item.id ? drag.offset : undefined}
          >
            <!-- A picture, not a control. The single-pointer alternative SC 2.5.7 asks for is the
                 menu at the end of the row, and it is a real one — so a second focusable element
                 that does nothing for the keyboard would be noise in the tab order rather than
                 access. -->
            <span class="grip" data-grip aria-hidden="true">
              <Icon name="grip-vertical" size="sm" />
            </span>
            <TaskRow
              type={row.item.type}
              title={row.item.title}
              depth={row.depth}
              isCompleted={row.item.completion?.is_completed ?? false}
              expansion={!row.takesChildren
                ? 'leaf'
                : expanded.includes(row.item.id)
                  ? 'expanded'
                  : 'collapsed'}
              completeLabel={t(
                row.item.completion?.is_completed ? 'app.entries.reopen' : 'app.entries.complete',
                { title: row.item.title },
              )}
              completeDisabledReason={isReadOnly ? t('app.entries.read_only') : undefined}
              expandLabel={t(
                expanded.includes(row.item.id) ? 'app.entries.collapse' : 'app.entries.expand',
                { title: row.item.title },
              )}
              onToggleComplete={() => toggleComplete(row.item)}
              onToggleExpand={() =>
                (expanded = expanded.includes(row.item.id)
                  ? expanded.filter((id) => id !== row.item.id)
                  : [...expanded, row.item.id])}
            >
              {#snippet trailing()}
                <!-- The labels this entry carries, and a way to change them — but only for a type
                     whose profile has LABELS. The manifest answers that; an ACTIVITY has none, and
                     §2 says a field whose capability is off is refused rather than ignored, so it is
                     not offered here at all. -->
                {#if supports(row.item.type, 'LABELS').status === 'permitted'}
                  {#each (row.item.label_ids ?? []) as labelId (labelId)}
                    {@const entry = available.find((each) => each.id === labelId)}
                    {#if entry}
                      <LabelChip
                        name={entry.name}
                        colorToken={entry.colorToken}
                        description={entry.description}
                        removeLabel={isReadOnly
                          ? undefined
                          : t('app.labels.remove_from', { name: entry.name, title: row.item.title })}
                        onRemove={() => toggleLabel(row.item, labelId)}
                      />
                    {/if}
                  {/each}

                  {#if !isReadOnly}
                    <Popover label={t('app.labels.on_entry', { title: row.item.title })}>
                      {#snippet trigger(props)}
                        <Button size="sm" tone="subtle" icon="tag" {...props}>
                          {t('app.labels.choose')}
                        </Button>
                      {/snippet}
                      <LabelPicker
                        label={t('app.labels.on_entry', { title: row.item.title })}
                        labels={available}
                        selected={row.item.label_ids ?? []}
                        filterLabel={t('app.labels.filter')}
                        emptyLabel={t('app.labels.none_yet')}
                        noMatchLabel={t('app.labels.no_match')}
                        onToggle={(labelId) => toggleLabel(row.item, labelId)}
                      />
                    </Popover>
                  {/if}
                {/if}

                {#if !isReadOnly && row.takesChildren}
                  <Button
                    size="sm"
                    tone="subtle"
                    icon="plus"
                    onclick={() => startAdding(row.item.id)}
                  >
                    {t('app.entries.add')}
                  </Button>
                {/if}

                <!-- The single-pointer alternative SC 2.5.7 asks for, and the path that was built
                     first: every position a drag can reach is an item here, each with the reason it
                     cannot be used rather than a control that goes grey for no stated cause. -->
                <Menu
                  label={t('app.rank.actions', { title: row.item.title })}
                  items={menuItems(row)}
                  onselect={(id) => chose(row, id)}
                >
                  {#snippet trigger(props)}
                    <IconButton
                      icon="ellipsis"
                      label={t('app.rank.actions', { title: row.item.title })}
                      size="sm"
                      {...props}
                    />
                  {/snippet}
                </Menu>
              {/snippet}
            </TaskRow>

          </div>

          {#if addingUnder === row.item.id}
            {@render addForm()}
          {/if}
        {/each}
      </div>

      {#if !isReadOnly && rootTypes().length > 0}
        {#if addingUnder === 'root'}
          {@render addForm()}
        {:else}
          <div>
            <Button tone="secondary" icon="plus" onclick={() => startAdding('root')}>
              {t('app.entries.add')}
            </Button>
          </div>
        {/if}
      {/if}
    {/if}
  {/if}
</Stack>

{#if movingRow}
  <MoveDialog
    isOpen={true}
    title={t('app.rank.actions', { title: movingRow.item.title })}
    label={t('app.move.destination')}
    placeholder={t('app.move.choose')}
    options={destinations}
    emptyLabel={t('app.move.no_destination')}
    warning={t('app.move.leaves_behind')}
    confirmLabel={t('app.move.confirm')}
    busyLabel={t('app.move.moving')}
    cancelLabel={t('app.entries.cancel')}
    chooseFirstLabel={t('app.move.choose_first')}
    isBusy={isMoving}
    onmove={(destination) => moveElsewhere(destination)}
  />
{/if}

{#snippet addForm()}
  <Stack gap="150">
    <!-- The chooser appears only when there is a choice: at most installations a work package takes
         activities and nothing else, and a select with one option is a decision nobody has. -->
    {#if offeredTypes.length > 1}
      <Select
        label={t('app.entries.type_label')}
        size="sm"
        bind:value={draftType}
        options={offeredTypes.map((type) => ({ value: type, label: type }))}
      />
    {/if}
    <Input label={t('app.entries.new_title')} bind:value={draftTitle} />
    <Inline gap="100">
      <!-- Off with a reason rather than silently doing nothing. `create()` returns early on an
           empty title, and a button that answers a press with nothing is the silent ignoring this
           system has a rule against — there is no `disabled` boolean anywhere for exactly this. -->
      <Button
        size="sm"
        isBusy={isSaving}
        busyLabel={t('app.entries.creating')}
        disabledReason={draftTitle.trim() === '' ? t('app.entries.title_required') : undefined}
        onclick={create}
      >
        {t('app.entries.create')}
      </Button>
      <Button size="sm" tone="secondary" onclick={() => (addingUnder = null)}>
        {t('app.entries.cancel')}
      </Button>
    </Inline>
  </Stack>
{/snippet}

<style>
  /* What `Stack gap="050"` was, written here because the level now owns a state of its own: a row
     being dragged is drawn differently, and a primitive that decorated would stop being one. */
  .level { display: flex; flex-direction: column; gap: var(--sp-050); }

  .row { display: flex; align-items: center; gap: var(--sp-050); }

  .row > :global(*:last-child) { flex: 1; min-width: 0; }

  /* `touch-action: none` is what makes a drag possible on a touch screen at all: without it the
     browser claims the gesture for scrolling and the pointer events stop arriving after the first
     few. It is on the grip alone, so the page still scrolls everywhere else. */
  .grip {
    display: inline-flex;
    flex: none;
    align-items: center;
    color: var(--text-subtle);
    cursor: grab;
    touch-action: none;
  }

  /* Rule 6: the movement is a `translate` and nothing else, and it is direct manipulation rather
     than decoration — the row is under the reader's finger. */
  .row[data-dragging] {
    translate: 0 var(--drag-offset);
    /* Rule 1: it is off the surface while it is being carried, so it is raised. */
    box-shadow: var(--shadow-overlay);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    /* Out of the way of the measuring: the element under the pointer must be the list, not the
       row being carried over it. */
    pointer-events: none;
  }

  .row[data-dragging] .grip { cursor: grabbing; }

  /* Where it would land. Rule 3: an outline and not a tint, so it reads in greyscale and to a
     reader who does not perceive the accent. */
  .row[data-drop] {
    outline: var(--bw-thick) dashed var(--accent-primary);
    outline-offset: var(--sp-025);
    border-radius: var(--r-md);
  }

  /* Rule 6's floor. Under a reduced-motion preference the row does not travel; the state and the
     landing slot still say what is happening, which is the colour change the rule fixes as the
     least a movement may reduce to. */
  @media (prefers-reduced-motion: reduce) {
    .row[data-dragging] { translate: none; }
  }

  :global([data-motion='reduced']) .row[data-dragging] { translate: none; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); max-width: 64ch; }

  /* Rule 1: a standalone notice, so it is raised rather than tinted. Rule 3: it is a list of what
     was lost and why, so nothing about it rests on the colour. */
  .dropped {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    max-width: 64ch;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    font-size: var(--fs-075);
  }

  .dropped p { margin: 0; }

  .dropped ul {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    margin: 0;
    padding-inline-start: var(--sp-200);
  }
</style>
